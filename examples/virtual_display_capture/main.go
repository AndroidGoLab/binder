// This example captures one raw RGBA frame through an Android 14 virtual display.
// It uses the Pixel gralloc4 descriptor and requires ACCESS_SURFACE_FLINGER.
// Build and run on a Pixel 6a with Android 14, using a privileged account:
//
// GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o capture ./examples/virtual_display_capture
// adb push capture /data/local/tmp/capture
// adb shell su 0 /data/local/tmp/capture -source-width 1080 -source-height 2400 -output /data/local/tmp/frame.rgba
// adb exec-out su 0 cat /data/local/tmp/frame.rgba > frame.rgba
//
// The output is one tightly packed RGBA8888 frame, 360 by 800 pixels.
// Its allocator descriptor is specific to Pixel gralloc4; other devices and
// Android versions need their own allocator and SurfaceFlinger protocols.
// The command exits after one attempt; malformed Binder replies can leave
// unread descriptors open until process exit because transport offsets are
// not exposed to this example.
// The virtual-display approach follows minicap's Android 12 implementation:
// https://github.com/DeviceFarmer/minicap/blob/f3d40d65da0cc7168846c4ae7466ada970441d4e/jni/minicap-shared/aosp/src/minicap_31.cpp#L207
package main

import (
	"context"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/AndroidGoLab/binder/android/hardware/common"
	"github.com/AndroidGoLab/binder/android/hardware/graphics/allocator"
	gfxCommon "github.com/AndroidGoLab/binder/android/hardware/graphics/common"
	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/binder/versionaware"
	"github.com/AndroidGoLab/binder/gralloc"
	"github.com/AndroidGoLab/binder/igbp"
	"github.com/AndroidGoLab/binder/kernelbinder"
	"github.com/AndroidGoLab/binder/parcel"
	"github.com/AndroidGoLab/binder/servicemanager"
	"golang.org/x/sys/unix"
)

const (
	// width and height set the fixed output geometry.
	//
	// Example: output contains 360 * 800 pixels.
	width  = 360
	height = 800
	// rgba8888 and pixelUsage identify the Pixel buffer layout and consumers.
	//
	// Example: the GPU renders pixels that the CPU reads.
	rgba8888   = int32(gfxCommon.PixelFormatRgba8888)
	pixelUsage = uint64(gfxCommon.BufferUsageCpuReadOften | gfxCommon.BufferUsageGpuTexture | gfxCommon.BufferUsageGpuRenderTarget | gfxCommon.BufferUsageComposerOverlay)
	// fenceTimeoutMS bounds the wait for a queued frame.
	//
	// Example: an unsignaled fence fails after five seconds.
	fenceTimeoutMS = 5000
	// DMA-BUF sync ioctl and flags follow Linux include/uapi/linux/dma-buf.h.
	// https://android.googlesource.com/kernel/common/+/refs/heads/android14-6.1/include/uapi/linux/dma-buf.h
	//
	// Example: READ starts CPU read access and END completes it.
	dmaBufSync     = 0x40086200
	dmaBufSyncRead = 1
	dmaBufSyncEnd  = 4
	// QueueBufferInput layout follows Android 14 IGraphicBufferProducerFlattenables.cpp.
	// https://android.googlesource.com/platform/frameworks/native/+/refs/tags/android-14.0.0_r1/libs/gui/IGraphicBufferProducerFlattenables.cpp
	//
	// Example: a minimal input includes an empty region and HDR metadata.
	queueFenceOffset = 45
	queueMinimumSize = 61
	// These API 34 transaction codes and display-state flags follow ISurfaceComposer.
	// https://android.googlesource.com/platform/frameworks/native/+/refs/tags/android-14.0.0_r1/libs/gui/aidl/android/gui/ISurfaceComposer.aidl
	// https://android.googlesource.com/platform/frameworks/native/+/refs/tags/android-14.0.0_r1/libs/gui/include/gui/LayerState.h
	//
	// Example: configureDisplayCode applies surface, projection, and size.
	allocateCode         = 1
	createDisplayCode    = 4
	destroyDisplayCode   = 5
	configureDisplayCode = 8
	displayStateChanges  = 7
	// Pixel 6a's tested gralloc4 descriptor uses version 40 in a 37-byte blob.
	// https://android.googlesource.com/platform/hardware/google/gchips/+/refs/heads/android14-release/gralloc4/src/hidl_common/BufferDescriptor.h
	//
	// Example: the final byte is the empty descriptor name.
	pixelDescriptorSize    = 37
	pixelDescriptorVersion = 40
)

// frameResult holds the copied frame or its capture error.
//
// Example: a fence timeout is returned without exposing the buffer.
type frameResult struct {
	Pixels []byte
	Err    error
}

// producer holds one slot until its only frame has been copied.
//
// Example: additional dequeue requests receive WOULD_BLOCK.
type producer struct {
	*igbp.ProducerStub
	Buffer   *gralloc.Buffer
	Dequeued atomic.Bool
	Result   chan frameResult
	Mu       sync.Mutex
	Closed   bool
}

// closeFDs closes descriptors acquired from the allocator.
//
// Example: a partially decoded allocation still releases its known FDs.
func closeFDs(fds []int32) error {
	var errs []error
	for _, fd := range fds {
		if err := unix.Close(int(fd)); err != nil {
			errs = append(errs, fmt.Errorf("close allocation fd: %w", err))
		}
	}
	return errors.Join(errs...)
}

// readAllocation tracks descriptors as they are decoded, including on error.
//
// Example: owned contains FDs read before a truncated handle.
func readAllocation(
	p *parcel.Parcel,
	owned *[]int32,
) (allocator.AllocationResult, error) {
	var result allocator.AllocationResult
	end, err := parcel.ReadParcelableHeader(p)
	if err != nil {
		return result, err
	}
	defer parcel.SkipToParcelableEnd(p, end)
	if result.Stride, err = p.ReadInt32(); err != nil {
		return result, err
	}
	count, err := p.ReadInt32()
	if err != nil {
		return result, err
	}
	if count < 0 || int64(count) > int64(p.Len()/4) {
		return result, errors.New("invalid allocation buffer count")
	}
	for i := int32(0); i < count; i++ {
		present, err := p.ReadInt32()
		if err != nil {
			return result, err
		}
		if present == 0 {
			return result, errors.New("null allocation buffer")
		}
		handle, err := readNativeHandle(p, owned)
		if err != nil {
			return result, err
		}
		result.Buffers = append(result.Buffers, handle)
	}
	return result, nil
}

// readNativeHandle decodes an allocator handle and records received FDs.
//
// Example: descriptors remain tracked if a later field is truncated.
func readNativeHandle(
	p *parcel.Parcel,
	owned *[]int32,
) (common.NativeHandle, error) {
	var handle common.NativeHandle
	end, err := parcel.ReadParcelableHeader(p)
	if err != nil {
		return handle, err
	}
	defer parcel.SkipToParcelableEnd(p, end)
	n, err := p.ReadInt32()
	if err != nil {
		return handle, err
	}
	if n < 0 || int64(n) > int64(p.Len()/4) {
		return handle, errors.New("invalid allocation fd count")
	}
	for j := int32(0); j < n; j++ {
		present, err := p.ReadInt32()
		if err != nil {
			return handle, err
		}
		if present == 0 {
			return handle, errors.New("null allocation fd")
		}
		comm, err := p.ReadInt32()
		if err != nil {
			return handle, err
		}
		fd, err := p.ReadFileDescriptor()
		if err != nil {
			return handle, err
		}
		*owned = append(*owned, fd)
		handle.Fds = append(handle.Fds, fd)
		if comm != 0 {
			fd, err = p.ReadFileDescriptor()
			if err != nil {
				return handle, err
			}
			*owned = append(*owned, fd)
			return handle, errors.New("unexpected allocation communication fd")
		}
	}
	n, err = p.ReadInt32()
	if err != nil {
		return handle, err
	}
	if n < 0 || int64(n) > int64(p.Len()/4) {
		return handle, errors.New("invalid allocation int count")
	}
	for j := int32(0); j < n; j++ {
		v, err := p.ReadInt32()
		if err != nil {
			return handle, err
		}
		handle.Ints = append(handle.Ints, v)
	}
	return handle, nil
}

// readQueueFence reads the slot and optional acquire fence from a queue request.
//
// Example: an absent fence is returned as -1.
func readQueueFence(p *parcel.Parcel) (int32, int32, error) {
	slot, err := p.ReadInt32()
	if err != nil {
		return 0, -1, err
	}
	size, err := p.ReadInt32()
	if err != nil {
		return 0, -1, err
	}
	count, err := p.ReadInt32()
	if err != nil {
		return 0, -1, err
	}
	if size < 0 || count < 0 || count > 1 {
		return 0, -1, fmt.Errorf("invalid queue payload size=%d fds=%d", size, count)
	}
	payload, err := p.Read(int(size))
	if err != nil {
		return 0, -1, err
	}
	fd := int32(-1)
	if count == 1 {
		fd, err = p.ReadFileDescriptor()
		if err != nil {
			return 0, -1, err
		}
	}
	// Android 14 puts the Fence count after 45 bytes; Region, HDR metadata,
	// and slot add at least another twelve bytes after that count.
	if len(payload) < queueMinimumSize || binary.LittleEndian.Uint32(payload[queueFenceOffset:queueFenceOffset+4]) != uint32(count) {
		var closeErr error
		if fd >= 0 {
			closeErr = unix.Close(int(fd))
		}
		return 0, -1, errors.Join(errors.New("invalid queue input fence"), closeErr)
	}
	return slot, fd, nil
}

// reportCancel publishes a terminal capture error unless shutdown has started.
//
// Example: a canceled slot unblocks the waiting capture.
func (p *producer) reportCancel(err error) {
	p.Mu.Lock()
	defer p.Mu.Unlock()
	if p.Closed {
		return
	}
	select {
	case p.Result <- frameResult{Err: err}:
	default:
	}
}

// packedRGBA removes row padding from an RGBA8888 buffer.
//
// Example: a 360-pixel row with stride 368 loses eight padding pixels.
func packedRGBA(
	data []byte,
	w int,
	h int,
	stride int,
) ([]byte, error) {
	if w <= 0 || h <= 0 || stride < w || stride > len(data)/4/h {
		return nil, errors.New("invalid frame dimensions or buffer length")
	}
	out := make([]byte, w*h*4)
	for y := 0; y < h; y++ {
		copy(out[y*w*4:(y+1)*w*4], data[y*stride*4:y*stride*4+w*4])
	}
	return out, nil
}

// copyFrame waits for the producer fence and copies CPU-synchronized pixels.
//
// Example: no output is returned when the fence does not signal.
func copyFrame(
	buf *gralloc.Buffer,
	fd int32,
) (_ []byte, resultErr error) {
	if fd >= 0 {
		defer // Close the acquire fence and retain any close failure.
		//
		// Example: an invalid fence cannot leak a descriptor.
		func() {
			if err := unix.Close(int(fd)); err != nil {
				resultErr = errors.Join(resultErr, fmt.Errorf("close fence: %w", err))
			}
		}()
		if err := waitFence(fd, fenceTimeoutMS*time.Millisecond, unix.Poll); err != nil {
			return nil, err
		}
	}
	if buf == nil || len(buf.Handle.Fds) == 0 || buf.Handle.Fds[0] < 0 || buf.Stride < width || int64(buf.Stride)*height*4 > int64(math.MaxInt) {
		return nil, errors.New("invalid vendor buffer")
	}
	bufferFD := int(buf.Handle.Fds[0])
	flags := uint64(dmaBufSyncRead)
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(bufferFD), dmaBufSync, uintptr(unsafe.Pointer(&flags))); errno != 0 {
		return nil, fmt.Errorf("begin CPU access: %w", errno)
	}
	defer // End synchronized CPU read access.
	//
	// Example: report an END ioctl failure along with a copy failure.
	func() {
		flags = uint64(dmaBufSyncRead | dmaBufSyncEnd)
		if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(bufferFD), dmaBufSync, uintptr(unsafe.Pointer(&flags))); errno != 0 {
			resultErr = errors.Join(resultErr, fmt.Errorf("end CPU access: %w", errno))
		}
	}()
	data, err := unix.Mmap(bufferFD, 0, int(buf.Stride)*height*4, unix.PROT_READ, unix.MAP_SHARED)
	if err != nil {
		return nil, err
	}
	defer // Release the mapped frame.
	//
	// Example: retain a munmap failure in the result.
	func() {
		if err := unix.Munmap(data); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("unmap frame: %w", err))
		}
	}()
	return packedRGBA(data, width, height, int(buf.Stride))
}

// OnTransaction handles one buffer and delegates other native window calls.
//
// Example: a queued frame is copied before its producer call returns.
func (p *producer) OnTransaction(
	ctx context.Context,
	code binder.TransactionCode,
	data *parcel.Parcel,
) (*parcel.Parcel, error) {
	token, err := data.ReadInterfaceToken()
	if err != nil {
		return nil, err
	}
	if token != igbp.Descriptor {
		return nil, fmt.Errorf("unexpected producer interface %q", token)
	}
	payloadPosition := data.Position()
	data.SetPosition(0)
	switch code {
	case igbp.RequestBuffer:
		data.SetPosition(payloadPosition)
		slot, err := data.ReadInt32()
		if err != nil {
			return nil, err
		}
		if slot != 0 || !p.Dequeued.Load() {
			return nil, fmt.Errorf("invalid requested slot %d", slot)
		}
		data.SetPosition(0)
	case igbp.DequeueBuffer:
		data.SetPosition(payloadPosition)
		if _, err := data.Read(20); err != nil {
			return nil, err
		}
		timestamps, err := data.ReadBool()
		if err != nil {
			return nil, err
		}
		data.SetPosition(0)
		if !p.Dequeued.CompareAndSwap(false, true) {
			r := parcel.New()
			r.WriteInt32(-1)
			r.WriteInt32(4)
			r.WriteInt32(0)
			r.WriteInt32(0)
			r.WriteUint64(0)
			if timestamps {
				igbp.WriteEmptyFrameEventHistoryDelta(r)
			}
			r.WriteInt32(-11)
			return r, nil
		}
	case igbp.CancelBuffer:
		if _, err := data.ReadInterfaceToken(); err != nil {
			return nil, err
		}
		slot, err := data.ReadInt32()
		if err != nil {
			return nil, err
		}
		size, err := data.ReadInt32()
		if err != nil {
			return nil, err
		}
		count, err := data.ReadInt32()
		if err != nil {
			return nil, err
		}
		if size != 4 || count < 0 || count > 1 {
			return nil, errors.New("invalid cancel fence")
		}
		nested, err := data.ReadInt32()
		if err != nil {
			return nil, err
		}
		var closeErr error
		if count == 1 {
			fd, err := data.ReadFileDescriptor()
			if err != nil {
				return nil, err
			}
			closeErr = unix.Close(int(fd))
		}
		if nested != count {
			return nil, errors.Join(errors.New("invalid cancel fence count"), closeErr)
		}
		p.reportCancel(errors.Join(fmt.Errorf("buffer slot %d canceled before capture", slot), closeErr))
		data.SetPosition(0)
	case igbp.Query:
		if _, err := data.ReadInterfaceToken(); err != nil {
			return nil, err
		}
		q, err := data.ReadInt32()
		if err != nil {
			return nil, err
		}
		data.SetPosition(0)
		if q == int32(igbp.NativeWindowFormat) {
			r := parcel.New()
			r.WriteInt32(rgba8888)
			r.WriteInt32(0)
			return r, nil
		}
	case igbp.QueueBuffer:
		if _, err := data.ReadInterfaceToken(); err != nil {
			return nil, err
		}
		slot, fd, err := readQueueFence(data)
		p.Mu.Lock()
		defer p.Mu.Unlock()
		if p.Closed {
			if fd >= 0 {
				if err := unix.Close(int(fd)); err != nil {
					return nil, fmt.Errorf("close late fence: %w", err)
				}
			}
			data.SetPosition(0)
			return p.ProducerStub.OnTransaction(ctx, code, data)
		}
		if err == nil && slot != 0 {
			var closeErr error
			if fd >= 0 {
				closeErr = unix.Close(int(fd))
			}
			err = errors.Join(fmt.Errorf("unexpected slot %d", slot), closeErr)
		}
		var pixels []byte
		if err == nil {
			pixels, err = copyFrame(p.Buffer, fd)
		}
		select {
		case p.Result <- frameResult{Pixels: pixels, Err: err}:
		default:
		}
		data.SetPosition(0)
	}
	return p.ProducerStub.OnTransaction(ctx, code, data)
}

// run creates the virtual display, captures one frame and releases it.
//
// Example: run can write the resulting raw pixels to a file.
func run(
	ctx context.Context,
	sourceWidth int,
	sourceHeight int,
) (_ []byte, resultErr error) {
	if sourceWidth <= 0 || sourceHeight <= 0 || int64(sourceWidth) > math.MaxInt32 || int64(sourceHeight) > math.MaxInt32 {
		return nil, errors.New("physical display dimensions must be positive int32 values")
	}
	if api := versionaware.DetectAPILevel(); api != 34 {
		return nil, fmt.Errorf("example supports Android API 34 only; detected %d", api)
	}
	drv, err := kernelbinder.Open(ctx, binder.WithMapSize(4*1024*1024))
	if err != nil {
		return nil, err
	}
	defer // Release Binder resources even when capture fails.
	//
	// Example: preserve a close error after a timeout.
	func() {
		if err := drv.Close(context.Background()); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("close binder: %w", err))
		}
	}()
	transport, err := versionaware.NewTransport(ctx, drv, 34, versionaware.OptionCachePath(""))
	if err != nil {
		return nil, err
	}
	sm := servicemanager.New(transport)
	alloc, err := sm.GetService(ctx, "android.hardware.graphics.allocator.IAllocator/default")
	if err != nil {
		return nil, err
	}
	desc := make([]byte, pixelDescriptorSize)
	binary.LittleEndian.PutUint32(desc[0:], pixelDescriptorVersion)
	binary.LittleEndian.PutUint32(desc[4:], width)
	binary.LittleEndian.PutUint32(desc[8:], height)
	binary.LittleEndian.PutUint32(desc[12:], 1)
	binary.LittleEndian.PutUint32(desc[16:], uint32(rgba8888))
	binary.LittleEndian.PutUint64(desc[20:], pixelUsage)
	req := parcel.New()
	req.WriteInterfaceToken(allocator.DescriptorIAllocator)
	req.WriteByteArray(desc)
	req.WriteInt32(1)
	reply, err := alloc.Transact(ctx, allocateCode, 0, req)
	if err != nil {
		return nil, err
	}
	defer reply.Recycle()
	var ownedFDs []int32
	defer // Close descriptors decoded from the allocation response.
	//
	// Example: partial decoding still releases acquired FDs.
	func() {
		if err := closeFDs(ownedFDs); err != nil {
			resultErr = errors.Join(resultErr, err)
		}
	}()
	if err = binder.ReadStatus(reply); err != nil {
		return nil, fmt.Errorf("Pixel gralloc4 allocation: %w", err)
	}
	present, err := reply.ReadInt32()
	if err != nil || present == 0 {
		return nil, fmt.Errorf("allocator returned no result: %v", err)
	}
	allocation, err := readAllocation(reply, &ownedFDs)
	if err != nil {
		return nil, err
	}
	if len(allocation.Buffers) != 1 {
		return nil, errors.New("allocator did not return one buffer")
	}
	buf := &gralloc.Buffer{Handle: allocation.Buffers[0], Width: width, Height: height, Stride: allocation.Stride, Format: rgba8888, Usage: pixelUsage}
	sf, err := sm.GetService(ctx, "SurfaceFlingerAIDL")
	if err != nil {
		return nil, err
	}
	req = parcel.New()
	req.WriteInterfaceToken("android.gui.ISurfaceComposer")
	req.WriteString16("binder-go-capture")
	req.WriteBool(false)
	req.WriteFloat32(0)
	reply, err = sf.Transact(ctx, createDisplayCode, 0, req)
	if err != nil {
		return nil, err
	}
	defer reply.Recycle()
	if err = binder.ReadStatus(reply); err != nil {
		return nil, err
	}
	display, err := reply.ReadStrongBinder()
	if err != nil {
		return nil, err
	}
	defer // Destroy the virtual display before closing Binder.
	//
	// Example: use a fresh context after capture cancellation.
	func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		p := parcel.New()
		p.WriteInterfaceToken("android.gui.ISurfaceComposer")
		p.WriteStrongBinder(display)
		r, e := sf.Transact(cleanupCtx, destroyDisplayCode, 0, p)
		if e == nil {
			defer r.Recycle()
			e = binder.ReadStatus(r)
		}
		if e != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("destroy virtual display: %w", e))
		}
	}()
	raw, err := sm.GetService(ctx, "SurfaceFlinger")
	if err != nil {
		return nil, err
	}
	var buffers [4]*gralloc.Buffer
	buffers[0] = buf
	prod := &producer{ProducerStub: igbp.NewProducerStub(width, height, buffers), Buffer: buf, Result: make(chan frameResult, 1)}
	defer // Prevent late callbacks from reading the released buffer.
	//
	// Example: shutdown waits for a copy already in progress.
	func() {
		prod.Mu.Lock()
		defer prod.Mu.Unlock()
		prod.Closed = true
	}()
	stub := binder.NewStubBinder(prod)
	stub.RegisterWithTransport(ctx, transport)
	req = parcel.New()
	req.WriteInterfaceToken("android.ui.ISurfaceComposer")
	head := parcel.WriteParcelableHeader(req)
	req.WriteInt64(-1)
	req.WriteInt32(0)
	req.WriteInt64(0)
	req.WriteBool(false)
	parcel.WriteParcelableFooter(req, head)
	req.WriteInt32(0)
	req.WriteInt32(1)
	req.WriteStrongBinder(display)
	binder.WriteBinderToParcel(ctx, req, stub, transport)
	req.WriteInt32(displayStateChanges)
	req.WriteInt32(0)
	req.WriteInt32(0)
	req.WriteInt32(0)
	for _, v := range []int32{0, 0, int32(sourceWidth), int32(sourceHeight), 0, 0, width, height} {
		req.WriteInt32(v)
	}
	req.WriteInt32(width)
	req.WriteInt32(height)
	req.WriteInt32(0)
	binder.WriteBinderToParcel(ctx, req, stub, transport)
	req.WriteInt32(0)
	req.WriteInt32(0)
	req.WriteInt64(0)
	req.WriteBool(true)
	req.WriteInt32(0)
	req.WriteBool(false)
	req.WriteInt32(0)
	req.WriteUint64(1)
	req.WriteInt32(0)
	if _, err = raw.Transact(ctx, configureDisplayCode, 0, req); err != nil {
		return nil, fmt.Errorf("configure virtual display: %w", err)
	}
	select {
	case result := <-prod.Result:
		return result.Pixels, result.Err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// main parses the physical display dimensions and writes one raw frame.
//
// Example: capture -source-width 1080 -source-height 2400 -output frame.rgba.
func main() {
	sw := flag.Int("source-width", 0, "physical display width")
	sh := flag.Int("source-height", 0, "physical display height")
	output := flag.String("output", "frame.rgba", "raw output file")
	flag.Parse()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pixels, err := run(ctx, *sw, *sh)
	if err == nil {
		err = os.WriteFile(*output, pixels, 0600)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("wrote %dx%d RGBA8888 to %s\n", width, height, *output)
}

// waitFence waits for a readable fence within one monotonic deadline.
//
// Example: an interrupted poll resumes with the remaining time.
func waitFence(
	fd int32,
	timeout time.Duration,
	poll func([]unix.PollFd, int) (int, error),
) error {
	deadline := time.Now().Add(timeout)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return errors.New("acquire fence timed out")
		}
		fds := []unix.PollFd{{Fd: fd, Events: unix.POLLIN}}
		n, err := poll(fds, int((remaining+time.Millisecond-1)/time.Millisecond))
		if errors.Is(err, unix.EINTR) || errors.Is(err, unix.EAGAIN) {
			continue
		}
		if err != nil {
			return fmt.Errorf("poll acquire fence: %w", err)
		}
		if n != 1 || fds[0].Revents&unix.POLLIN == 0 || fds[0].Revents&(unix.POLLERR|unix.POLLNVAL) != 0 {
			return fmt.Errorf("acquire fence did not signal: %#x", fds[0].Revents)
		}
		return nil
	}
}
