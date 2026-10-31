// This example captures one raw RGBA frame through an Android 14 virtual display.
// It uses the Pixel gralloc4 descriptor and requires ACCESS_SURFACE_FLINGER.
// Build and run on a Pixel 6a with Android 14, using a privileged account:
//
// GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o capture ./examples/virtual_display_capture
// adb push capture /data/local/tmp/capture
// adb shell su 0 /data/local/tmp/capture -source-width 1080 -source-height 2400 -output /data/local/tmp/frame.rgba
//
// The output is one tightly packed RGBA8888 frame, 360 by 800 pixels.
// Its allocator descriptor is specific to Pixel gralloc4; other devices and
// Android versions need their own allocator and SurfaceFlinger protocols.
package main

import (
	"context"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/AndroidGoLab/binder/android/hardware/graphics/allocator"
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
	width          = 360
	height         = 800
	rgba8888       = 1
	pixelUsage     = 0xb03
	fenceTimeoutMS = 5000
	dmaBufSync     = 0x40086200
	dmaBufSyncRead = 1
	dmaBufSyncEnd  = 4
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
	if _, err = p.Read(int(size)); err != nil {
		return 0, -1, err
	}
	if count == 0 {
		return slot, -1, nil
	}
	fd, err := p.ReadFileDescriptor()
	return slot, fd, err
}

// packedRGBA removes row padding from an RGBA8888 buffer.
//
// Example: a 360-pixel row with stride 368 loses eight padding pixels.
func packedRGBA(data []byte, w, h, stride int) ([]byte, error) {
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
func copyFrame(buf *gralloc.Buffer, fd int32) ([]byte, error) {
	if fd >= 0 {
		defer unix.Close(int(fd))
		fds := []unix.PollFd{{Fd: fd, Events: unix.POLLIN}}
		n, err := unix.Poll(fds, fenceTimeoutMS)
		if err != nil {
			return nil, err
		}
		if n != 1 || fds[0].Revents&unix.POLLIN == 0 {
			return nil, fmt.Errorf("acquire fence did not signal: %#x", fds[0].Revents)
		}
	}
	if len(buf.Handle.Fds) == 0 || buf.Stride < width {
		return nil, errors.New("invalid vendor buffer")
	}
	bufferFD := int(buf.Handle.Fds[0])
	flags := uint64(dmaBufSyncRead)
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(bufferFD), dmaBufSync, uintptr(unsafe.Pointer(&flags))); errno != 0 {
		return nil, fmt.Errorf("begin CPU access: %w", errno)
	}
	defer func() {
		flags = uint64(dmaBufSyncRead | dmaBufSyncEnd)
		unix.Syscall(unix.SYS_IOCTL, uintptr(bufferFD), dmaBufSync, uintptr(unsafe.Pointer(&flags)))
	}()
	data, err := unix.Mmap(bufferFD, 0, int(buf.Stride)*height*4, unix.PROT_READ, unix.MAP_SHARED)
	if err != nil {
		return nil, err
	}
	defer unix.Munmap(data)
	return packedRGBA(data, width, height, int(buf.Stride))
}

// OnTransaction handles one buffer and delegates other native window calls.
//
// Example: a queued frame is copied before its producer call returns.
func (p *producer) OnTransaction(ctx context.Context, code binder.TransactionCode, data *parcel.Parcel) (*parcel.Parcel, error) {
	switch code {
	case igbp.DequeueBuffer:
		if !p.Dequeued.CompareAndSwap(false, true) {
			r := parcel.New()
			r.WriteInt32(-1)
			r.WriteInt32(4)
			r.WriteInt32(0)
			r.WriteInt32(0)
			r.WriteUint64(0)
			r.WriteInt32(-11)
			return r, nil
		}
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
				unix.Close(int(fd))
			}
			data.SetPosition(0)
			return p.ProducerStub.OnTransaction(ctx, code, data)
		}
		if err == nil && slot != 0 {
			if fd >= 0 {
				unix.Close(int(fd))
			}
			err = fmt.Errorf("unexpected slot %d", slot)
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
func run(ctx context.Context, sourceWidth, sourceHeight int) (_ []byte, resultErr error) {
	if sourceWidth <= 0 || sourceHeight <= 0 {
		return nil, errors.New("physical display dimensions must be positive")
	}
	if api := versionaware.DetectAPILevel(); api != 34 {
		return nil, fmt.Errorf("example supports Android API 34 only; detected %d", api)
	}
	drv, err := kernelbinder.Open(ctx, binder.WithMapSize(4*1024*1024))
	if err != nil {
		return nil, err
	}
	defer drv.Close(ctx)
	transport, err := versionaware.NewTransport(ctx, drv, 34, versionaware.OptionCachePath(""))
	if err != nil {
		return nil, err
	}
	sm := servicemanager.New(transport)
	alloc, err := sm.GetService(ctx, "android.hardware.graphics.allocator.IAllocator/default")
	if err != nil {
		return nil, err
	}
	desc := make([]byte, 37)
	binary.LittleEndian.PutUint32(desc[0:], 40)
	binary.LittleEndian.PutUint32(desc[4:], width)
	binary.LittleEndian.PutUint32(desc[8:], height)
	binary.LittleEndian.PutUint32(desc[12:], 1)
	binary.LittleEndian.PutUint32(desc[16:], rgba8888)
	binary.LittleEndian.PutUint64(desc[20:], pixelUsage)
	req := parcel.New()
	req.WriteInterfaceToken(allocator.DescriptorIAllocator)
	req.WriteByteArray(desc)
	req.WriteInt32(1)
	reply, err := alloc.Transact(ctx, 1, 0, req)
	if err != nil {
		return nil, err
	}
	defer reply.Recycle()
	if err = binder.ReadStatus(reply); err != nil {
		return nil, fmt.Errorf("Pixel gralloc4 allocation: %w", err)
	}
	present, err := reply.ReadInt32()
	if err != nil || present == 0 {
		return nil, fmt.Errorf("allocator returned no result: %v", err)
	}
	var allocation allocator.AllocationResult
	if err = allocation.UnmarshalParcel(reply); err != nil {
		return nil, err
	}
	if len(allocation.Buffers) != 1 {
		return nil, errors.New("allocator did not return one buffer")
	}
	buf := &gralloc.Buffer{Handle: allocation.Buffers[0], Width: width, Height: height, Stride: allocation.Stride, Format: rgba8888, Usage: pixelUsage}
	defer func() {
		for _, fd := range buf.Handle.Fds {
			unix.Close(int(fd))
		}
	}()
	sf, err := sm.GetService(ctx, "SurfaceFlingerAIDL")
	if err != nil {
		return nil, err
	}
	req = parcel.New()
	req.WriteInterfaceToken("android.gui.ISurfaceComposer")
	req.WriteString16("binder-go-capture")
	req.WriteBool(false)
	req.WriteFloat32(0)
	reply, err = sf.Transact(ctx, 4, 0, req)
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
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		p := parcel.New()
		p.WriteInterfaceToken("android.gui.ISurfaceComposer")
		p.WriteStrongBinder(display)
		r, e := sf.Transact(cleanupCtx, 5, 0, p)
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
	defer func() {
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
	req.WriteInt32(7)
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
	if _, err = raw.Transact(ctx, 8, 0, req); err != nil {
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
