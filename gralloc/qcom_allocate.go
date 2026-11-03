package gralloc

import (
	"errors"
	"fmt"
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

// qcomAllocatorFQName identifies the allocator used by the Qualcomm profile.
const qcomAllocatorFQName = "android.hardware.graphics.allocator@2.0::IAllocator"

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
