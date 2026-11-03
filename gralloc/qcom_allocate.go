package gralloc

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/AndroidGoLab/binder/android/hardware/common"
	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/hwparcel"
	"github.com/AndroidGoLab/binder/hwservicemanager"
	"github.com/AndroidGoLab/binder/kernelbinder"
	"github.com/AndroidGoLab/binder/parcel"
	"github.com/AndroidGoLab/binder/servicemanager"
	"golang.org/x/sys/unix"
	"math"

	gfx "github.com/AndroidGoLab/binder/android/hardware/graphics/common"
)

const (
	// qcomDescriptorVersion identifies the SDM845 v1.0 allocator descriptor.
	//
	// Example: Android 10 allocator@2 receives seven uint32 words.
	qcomDescriptorVersion uint32 = 0x76312e30
	// qcomHandleMagic identifies the SDM845 private handle ABI.
	//
	// Example: unknown vendor handles are rejected before mapping.
	qcomHandleMagic int32 = 0x676d736d
	// qcomRGBAUsage requests uncached CPU reads and GPU display rendering.
	//
	// Example: CPU_READ_RARELY selects an uncached ION allocation.
	qcomRGBAUsage uint64 = 0xb02
	// qcomUnsafeFlags identifies cached, compressed, secure, or tiled handles.
	//
	// Example: a UBWC allocation must not be interpreted as linear RGBA.
	qcomUnsafeFlags int32 = 0x0b004601
	// qcomIonFlag identifies the ION-backed allocation path.
	//
	// Example: non-ION handles are not accepted by this reader.
	qcomIonFlag int32 = 8
)

var errQcomAllocatorUnavailable = errors.New("Qualcomm allocator@2.0 is not registered")

// qcomDescriptor serializes the Android 10 SDM845 gralloc2 buffer descriptor.
// https://android.googlesource.com/platform/hardware/qcom/sdm845/display/+/refs/tags/android-10.0.0_r40/gralloc/gr_buf_descriptor.h
//
// Example: request a CPU-readable RGBA8888 virtual-display buffer.
func qcomDescriptor(width, height int32) ([]uint32, error) {
	if width <= 0 || height <= 0 {
		return nil, errors.New("buffer dimensions must be positive")
	}
	return []uint32{qcomDescriptorVersion, uint32(width), uint32(height), 1, uint32(gfx.PixelFormatRgba8888), uint32(qcomRGBAUsage), uint32(qcomRGBAUsage >> 32)}, nil
}

// qcomRGBALayout validates the Android 10 SDM845 private_handle_t layout.
// https://android.googlesource.com/platform/hardware/qcom/sdm845/display/+/refs/tags/android-10.0.0_r40/gralloc/gr_priv_handle.h
//
// Example: reject compressed, cached, non-ION, or inconsistent buffers.
func qcomRGBALayout(b *Buffer) (rgbaLayout, error) {
	var out rgbaLayout
	if b == nil || len(b.Handle.Fds) != 2 || len(b.Handle.Ints) != 22 {
		return out, errors.New("unsupported Qualcomm handle shape")
	}
	a := b.Handle.Ints
	if a[0] != qcomHandleMagic || a[1]&qcomIonFlag == 0 || a[1]&qcomUnsafeFlags != 0 {
		return out, errors.New("unsupported Qualcomm allocation flags or magic")
	}
	if b.Handle.Fds[0] < 0 || b.Format != int32(gfx.PixelFormatRgba8888) || a[6] != int32(gfx.PixelFormatRgba8888) || a[8] != 1 {
		return out, errors.New("unsupported Qualcomm format or descriptor")
	}
	if b.Width == 0 || b.Height == 0 || b.Stride <= 0 || a[2] != b.Stride || uint32(a[2]) < b.Width || a[3] <= 0 || uint32(a[3]) < b.Height || uint32(a[4]) != b.Width || uint32(a[5]) != b.Height {
		return out, errors.New("inconsistent Qualcomm dimensions")
	}
	usage := uint64(uint32(a[11])) | uint64(uint32(a[12]))<<32
	if usage&qcomRGBAUsage != qcomRGBAUsage || b.Usage&qcomRGBAUsage != qcomRGBAUsage || usage&0xf != 2 || b.Usage&0xf != 2 {
		return out, errors.New("unsupported Qualcomm buffer usage")
	}
	if a[13] <= 0 || a[14] != 0 {
		return out, errors.New("invalid Qualcomm allocation size or offset")
	}
	stride := uint64(uint32(b.Stride)) * 4
	if stride > math.MaxInt || uint64(b.Height) > uint64(math.MaxInt)/stride {
		return out, errors.New("Qualcomm buffer dimensions overflow")
	}
	extent := stride * uint64(b.Height)
	if extent > uint64(uint32(a[13])) {
		return out, fmt.Errorf("Qualcomm pixel plane exceeds allocation")
	}
	out = rgbaLayout{fd: int(b.Handle.Fds[0]), stride: int(stride), length: int(extent)}
	return out, nil
}

// readQcomAllocation decodes one HIDL allocator@2 reply and transfers its FDs.
//
// Example: reject HAL errors before accepting a native handle.
func readQcomAllocation(p *parcel.Parcel) (_ int32, _ common.NativeHandle, resultErr error) {
	var handle common.NativeHandle
	defer // Close transferred FDs if subsequent validation fails.
	//
	// Example: a missing second FD cannot leak the first FD.
	func() {
		if resultErr != nil {
			for _, fd := range handle.Fds {
				resultErr = errors.Join(resultErr, unix.Close(int(fd)))
			}
		}
	}()
	data := p.Data()
	objs := p.Objects()
	if len(data) < 12 {
		return 0, handle, errors.New("truncated HIDL allocation reply")
	}
	status := int32(binary.LittleEndian.Uint32(data))
	halErr := int32(binary.LittleEndian.Uint32(data[4:]))
	stride := int32(binary.LittleEndian.Uint32(data[8:]))
	if status != 0 || halErr != 0 {
		return 0, handle, fmt.Errorf("HIDL allocator status=%d error=%d", status, halErr)
	}
	if len(objs) != 4 || objs[0] != 12 || objs[1] != 52 || objs[2] != 92 || objs[3] != 132 || len(data) < 164 {
		return 0, handle, errors.New("unsupported HIDL allocation object layout")
	}
	for _, off := range objs[:3] {
		if binary.LittleEndian.Uint32(data[off:]) != 0x70742a85 {
			return 0, handle, errors.New("invalid HIDL buffer object")
		}
	}
	if binary.LittleEndian.Uint32(data[132:]) != 0x66646185 || binary.LittleEndian.Uint64(data[140:]) != 2 || binary.LittleEndian.Uint64(data[148:]) != 2 || binary.LittleEndian.Uint64(data[156:]) != 12 {
		return 0, handle, errors.New("invalid HIDL descriptor array")
	}
	vecOff := binary.LittleEndian.Uint64(data[20:])
	vecSize := binary.LittleEndian.Uint64(data[28:])
	if vecSize != 16 || vecOff > uint64(len(data))-16 || binary.LittleEndian.Uint32(data[vecOff+8:]) != 1 {
		return 0, handle, errors.New("invalid HIDL buffer vector")
	}
	handleOff := binary.LittleEndian.Uint64(data[100:])
	handleSize := binary.LittleEndian.Uint64(data[108:])
	const nativeHandleSize = 12 + 2*4 + 22*4
	if handleSize != nativeHandleSize || handleOff > uint64(len(data))-nativeHandleSize {
		return 0, handle, errors.New("invalid Qualcomm native handle size")
	}
	raw := data[handleOff : handleOff+nativeHandleSize]
	if binary.LittleEndian.Uint32(raw) != 12 || binary.LittleEndian.Uint32(raw[4:]) != 2 || binary.LittleEndian.Uint32(raw[8:]) != 22 {
		return 0, handle, errors.New("invalid Qualcomm native handle header")
	}
	for i := 0; i < 2; i++ {
		fd, err := p.TakeReceivedFileDescriptor(int(handleOff) + 12 + i*4)
		if err != nil {
			return 0, handle, fmt.Errorf("take allocator descriptor: %w", err)
		}
		handle.Fds = append(handle.Fds, fd)
	}
	for i := 0; i < 22; i++ {
		handle.Ints = append(handle.Ints, int32(binary.LittleEndian.Uint32(raw[20+i*4:])))
	}
	return stride, handle, nil
}

// AllocateRGBA allocates buffers for a verified platform-specific capture ABI.
//
// Example: API 29 probes the SDM845 allocator and rejects unknown handles.
func AllocateRGBA(ctx context.Context, sm *servicemanager.ServiceManager, api int, width, height int32, count int) ([]*Buffer, error) {
	switch api {
	case 29:
		if count < 1 || count > 64 {
			return nil, errors.New("invalid Qualcomm buffer count")
		}
		if _, err := qcomDescriptor(width, height); err != nil {
			return nil, err
		}
		driver, err := kernelbinder.Open(ctx, binder.WithDevicePath("/dev/hwbinder"), binder.WithMapSize(256*1024))
		if err != nil {
			return nil, fmt.Errorf("open hwbinder: %w", err)
		}
		return allocateAndroid10RGBA(ctx, driver, width, height, count)
	case 34:
		return AllocatePixelRGBA(ctx, sm, api, width, height, count)
	default:
		return nil, fmt.Errorf("unsupported capture allocator API %d", api)
	}
}

// allocateAndroid10RGBA probes Android 10 allocators and owns the connection.
//
// Example: an absent Qualcomm allocator triggers the Mali probe.
func allocateAndroid10RGBA(ctx context.Context, driver binder.Transport, width, height int32, count int) (buffers []*Buffer, resultErr error) {
	defer func() {
		resultErr = errors.Join(resultErr, driver.Close(context.Background()))
		if resultErr != nil {
			resultErr = errors.Join(resultErr, CloseBuffers(buffers))
			buffers = nil
		}
	}()
	buffers, resultErr = allocateQcomRGBA(ctx, driver, width, height, count)
	if errors.Is(resultErr, errQcomAllocatorUnavailable) {
		buffers, resultErr = allocateMaliRGBA(ctx, driver, width, height, count)
	}
	return buffers, resultErr
}

// allocateQcomRGBA uses Android 10's HIDL allocator@2 and validates each handle.
//
// Example: allocate one buffer per transaction to simplify HIDL FD ownership.
func allocateQcomRGBA(ctx context.Context, driver binder.Transport, width, height int32, count int) (_ []*Buffer, resultErr error) {
	if count < 1 || count > 64 {
		return nil, errors.New("invalid Qualcomm buffer count")
	}
	desc, err := qcomDescriptor(width, height)
	if err != nil {
		return nil, err
	}
	var buffers []*Buffer
	defer // Close partial buffers on error.
	//
	// Example: an unsupported second handle releases the first allocation.
	func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, CloseBuffers(buffers))
		}
	}()
	const fqName = "android.hardware.graphics.allocator@2.0::IAllocator"
	handle, err := hwservicemanager.New(driver).GetService(ctx, fqName, "default")
	if err != nil {
		return nil, fmt.Errorf("get Qualcomm allocator@2: %w", err)
	}
	if handle == 0 {
		return nil, errQcomAllocatorUnavailable
	}
	for range count {
		req := hwparcel.New()
		req.WriteInterfaceToken(fqName)
		req.WriteHidlVecUint32(desc)
		req.WriteUint32(1)
		reply, err := hwservicemanager.TransactHidl(ctx, driver, handle, 2, req)
		if err != nil {
			return nil, fmt.Errorf("allocate Qualcomm buffer: %w", err)
		}
		stride, h, parseErr := readQcomAllocation(reply)
		readErr := errors.Join(parseErr, reply.Recycle())
		if readErr != nil {
			if parseErr == nil {
				for _, fd := range h.Fds {
					readErr = errors.Join(readErr, unix.Close(int(fd)))
				}
			}
			return nil, fmt.Errorf("decode Qualcomm buffer: %w", readErr)
		}
		b := &Buffer{Handle: h, Width: uint32(width), Height: uint32(height), Stride: stride, Format: int32(gfx.PixelFormatRgba8888), Usage: qcomRGBAUsage}
		buffers = append(buffers, b)
		if _, err = qcomRGBALayout(b); err != nil {
			return nil, fmt.Errorf("unsupported Qualcomm allocation: %w", err)
		}
	}
	return buffers, nil
}

// copyQcomRGBAInto maps an explicitly validated uncached SDM845 ION allocation.
// The caller must have waited for the producer's acquire fence. The vendor's
// mapper skips cache operations for uncached read-only buffers; its ION mapper
// also requires /dev/ion open in the mapping process.
// https://android.googlesource.com/platform/hardware/qcom/sdm845/display/+/refs/tags/android-10.0.0_r40/gralloc/gr_buf_mgr.cpp
// https://android.googlesource.com/platform/hardware/qcom/sdm845/display/+/refs/tags/android-10.0.0_r40/gralloc/gr_ion_alloc.cpp
//
// Example: read tightly packed RGBA rows from a virtual-display buffer.
func copyQcomRGBAInto(b *Buffer, layout rgbaLayout, pixels []byte) (resultErr error) {
	ion, err := unix.Open("/dev/ion", unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("open ion: %w", err)
	}
	defer // Keep ION open for the lifetime of the mapping.
	//
	// Example: preserve a descriptor close error after copying pixels.
	func() {
		resultErr = errors.Join(resultErr, unix.Close(ion))
	}()
	return copyQcomMappedRGBAInto(b, layout, pixels)
}

// copyQcomMappedRGBAInto maps a validated plane into a caller-owned slice.
//
// Example: reuse a packed output buffer while stripping padded ION rows.
func copyQcomMappedRGBAInto(b *Buffer, layout rgbaLayout, pixels []byte) (resultErr error) {
	if err := validateRGBADestination(b, pixels); err != nil {
		return err
	}
	var stat unix.Stat_t
	if err := unix.Fstat(layout.fd, &stat); err != nil {
		return fmt.Errorf("stat Qualcomm buffer: %w", err)
	}
	if stat.Size < int64(layout.length) {
		return errors.New("Qualcomm buffer descriptor is too short")
	}
	data, err := unix.Mmap(layout.fd, 0, layout.length, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		return fmt.Errorf("map Qualcomm buffer: %w", err)
	}
	defer // Release the mapping and surface any cleanup failure.
	//
	// Example: an unmap failure invalidates the returned frame.
	func() {
		resultErr = errors.Join(resultErr, unix.Munmap(data))
	}()
	copyRGBARows(pixels, data, layout, b.Width, b.Height)
	return nil
}
