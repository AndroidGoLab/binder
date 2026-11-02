package gralloc

import (
	"errors"
	"fmt"
	"math"

	gfxCommon "github.com/AndroidGoLab/binder/android/hardware/graphics/common"
	"golang.org/x/sys/unix"
)

// The gchips private handle's integer offsets follow the 64-bit Android 14 r35
// ABI after its two transported descriptors, including native alignment padding.
//
// Example: gchipsPlaneByteStride identifies the first plane's byte stride.
const (
	// gchipsDescriptorCount is the number of transported descriptors.
	//
	// Example: a pixel buffer and its metadata are separate descriptors.
	gchipsDescriptorCount = 2
	// gchipsIntCount is the number of remaining native-handle words.
	//
	// Example: reject handles with a different ABI size.
	gchipsIntCount = 77
	// gchipsUnusedFD0 is the first unused descriptor slot's word offset.
	//
	// Example: validate the slot is unset.
	gchipsUnusedFD0 = 0
	// gchipsUnusedFD1 is the second unused descriptor slot's word offset.
	//
	// Example: validate the slot is unset.
	gchipsUnusedFD1 = 1
	// gchipsMagicIndex is the magic field's word offset.
	//
	// Example: recognize a gchips handle.
	gchipsMagicIndex = 2
	// gchipsFDCountIndex is the allocation FD count's word offset.
	//
	// Example: validate the pixel plane FD index.
	gchipsFDCountIndex = 4
	// gchipsWidthIndex is the requested width's word offset.
	//
	// Example: compare it with GraphicBuffer width.
	gchipsWidthIndex = 5
	// gchipsHeightIndex is the requested height's word offset.
	//
	// Example: compare it with GraphicBuffer height.
	gchipsHeightIndex = 6
	// gchipsRequestedFormatIndex is the requested format's word offset.
	//
	// Example: require RGBA8888 pixels.
	gchipsRequestedFormatIndex = 7
	// gchipsAllocFormatIndex is the allocated format's word offset.
	//
	// Example: reject compressed allocation formats.
	gchipsAllocFormatIndex = 15
	// gchipsPlaneOffsetIndex is the first plane's byte offset field.
	//
	// Example: locate a plane after an allocation prefix.
	gchipsPlaneOffsetIndex = 17
	// gchipsPlaneFDIndex is the first plane's descriptor index field.
	//
	// Example: select the FD containing pixel data.
	gchipsPlaneFDIndex = 19
	// gchipsPlaneSizeIndex is the first plane's size field.
	//
	// Example: reject a plane smaller than its visible rows.
	gchipsPlaneSizeIndex = 21
	// gchipsPlaneByteStride is the first plane's byte-stride field.
	//
	// Example: account for padding between rows.
	gchipsPlaneByteStride = 23
	// gchipsPlaneWidthIndex is the first plane's allocation width field.
	//
	// Example: validate the allocated width covers the image.
	gchipsPlaneWidthIndex = 25
	// gchipsPlaneHeightIndex is the first plane's allocation height field.
	//
	// Example: validate the allocated height covers the image.
	gchipsPlaneHeightIndex = 27
	// gchipsLayerCountIndex is the layer count's word offset.
	//
	// Example: reject layered buffers.
	gchipsLayerCountIndex = 53
	// gchipsAllocSizeIndex is the first allocation's size field.
	//
	// Example: reject a pixel extent beyond the allocation.
	gchipsAllocSizeIndex = 63
	// gchipsMapperVersionIndex is the mapper version's word offset.
	//
	// Example: identify the supported gralloc4 layout.
	gchipsMapperVersionIndex = 75
	// gchipsHandleMagic identifies a gchips private handle.
	//
	// Example: reject handles from an unknown allocator.
	gchipsHandleMagic = 0x3141592
	// gchipsMapperVersion identifies the supported mapper ABI.
	//
	// Example: reject a handle from a different mapper version.
	gchipsMapperVersion = 400
	// gchipsSingleFD is the supported allocation FD count.
	//
	// Example: reject multi-FD pixel allocations.
	gchipsSingleFD = 1
	// gchipsSingleLayer is the supported layer count.
	//
	// Example: reject a layered allocation.
	gchipsSingleLayer = 1
	// rgbaBytesPerPixel is the size of one RGBA8888 pixel.
	//
	// Example: calculate the visible row length.
	rgbaBytesPerPixel = 4
)

// rgbaLayout identifies the mapped pixel plane and its byte geometry.
//
// Example: copy each visible row using stride bytes between source rows.
type rgbaLayout struct {
	fd                     int
	offset, stride, length int
}

// pixelRGBALayout decodes the 64-bit gchips gralloc4 private_handle_t ABI in
// android-14.0.0_r35. Its plane fields are allocator-specific, not part of
// GraphicBuffer's portable contract.
// https://android.googlesource.com/platform/hardware/google/gchips/+/refs/tags/android-14.0.0_r35/gralloc4/src/mali_gralloc_buffer.h
//
// Example: locate the uncompressed RGBA plane in a supported Pixel handle.
func pixelRGBALayout(b *Buffer) (rgbaLayout, error) {
	var out rgbaLayout
	if b == nil || len(b.Handle.Fds) != gchipsDescriptorCount || len(b.Handle.Ints) != gchipsIntCount {
		return out, fmt.Errorf("unsupported gralloc handle shape")
	}
	a := b.Handle.Ints
	if a[gchipsUnusedFD0] != -1 || a[gchipsUnusedFD1] != -1 || a[gchipsMagicIndex] != gchipsHandleMagic || a[gchipsFDCountIndex] != gchipsSingleFD || a[gchipsLayerCountIndex] != gchipsSingleLayer || handleUint64(a, gchipsMapperVersionIndex) != gchipsMapperVersion {
		return out, fmt.Errorf("unsupported gchips handle")
	}
	if b.Format != int32(gfxCommon.PixelFormatRgba8888) || a[gchipsRequestedFormatIndex] != int32(gfxCommon.PixelFormatRgba8888) || handleUint64(a, gchipsAllocFormatIndex) != uint64(gfxCommon.PixelFormatRgba8888) {
		return out, fmt.Errorf("unsupported gralloc format or compression")
	}
	if b.Width == 0 || b.Height == 0 || uint64(b.Width) != uint64(uint32(a[gchipsWidthIndex])) || uint64(b.Height) != uint64(uint32(a[gchipsHeightIndex])) {
		return out, fmt.Errorf("inconsistent gralloc dimensions")
	}
	fdIndex := uint32(a[gchipsPlaneFDIndex])
	if fdIndex >= uint32(a[gchipsFDCountIndex]) || int(fdIndex) >= len(b.Handle.Fds) || b.Handle.Fds[fdIndex] < 0 {
		return out, fmt.Errorf("invalid pixel plane fd index %d", fdIndex)
	}
	offset, stride, width, height, allocation := handleUint64(a, gchipsPlaneOffsetIndex), handleUint64(a, gchipsPlaneByteStride), handleUint64(a, gchipsPlaneWidthIndex), handleUint64(a, gchipsPlaneHeightIndex), handleUint64(a, gchipsAllocSizeIndex)
	if width < uint64(b.Width) || height < uint64(b.Height) || stride < uint64(b.Width)*rgbaBytesPerPixel || stride > math.MaxInt || offset > math.MaxInt || uint64(b.Height) > math.MaxInt/stride {
		return out, fmt.Errorf("invalid pixel plane dimensions")
	}
	extent := stride * uint64(b.Height)
	planeSize := handleUint64(a, gchipsPlaneSizeIndex)
	if offset > math.MaxInt-extent || offset+extent > allocation || (planeSize != 0 && extent > planeSize) {
		return out, fmt.Errorf("pixel plane exceeds allocation")
	}
	out = rgbaLayout{fd: int(b.Handle.Fds[fdIndex]), offset: int(offset), stride: int(stride), length: int(offset + extent)}
	return out, nil
}

// handleUint64 decodes a little-endian aligned 64-bit handle field.
//
// Example: read the allocation size from its two native-handle words.
func handleUint64(
	words []int32,
	index int,
) uint64 {
	return uint64(uint32(words[index])) | uint64(uint32(words[index+1]))<<32
}

// CopyRGBA copies a supported linear gralloc buffer into tightly packed RGBA.
// The caller must wait for its acquire fence before calling this function.
//
// Example: copy pixels after waiting for the screen capture fence.
func CopyRGBA(b *Buffer) (pixels []byte, resultErr error) {
	if _, err := captureRGBALayout(b); err != nil {
		return nil, err
	}
	pixels = make([]byte, int(b.Width)*int(b.Height)*rgbaBytesPerPixel)
	if err := CopyRGBAInto(b, pixels); err != nil {
		return nil, err
	}
	return pixels, nil
}

// captureRGBALayout validates the supported vendor-specific pixel layout.
//
// Example: reject a compressed or unknown handle before allocating output.
func captureRGBALayout(b *Buffer) (rgbaLayout, error) {
	if b != nil && len(b.Handle.Ints) == 22 && b.Handle.Ints[0] == qcomHandleMagic {
		return qcomRGBALayout(b)
	}
	return pixelRGBALayout(b)
}

// CopyRGBAInto copies a supported linear buffer into a caller-owned packed slice.
// The caller must wait for the acquire fence and supply exactly width*height*4 bytes.
// The destination may contain partial pixels if an error is returned.
//
// Example: reuse one destination across a sequence of captured frames.
func CopyRGBAInto(b *Buffer, pixels []byte) (resultErr error) {
	l, err := captureRGBALayout(b)
	if err != nil {
		return err
	}
	if err := validateRGBADestination(b, pixels); err != nil {
		return err
	}
	if len(b.Handle.Ints) == 22 && b.Handle.Ints[0] == qcomHandleMagic {
		return copyQcomRGBAInto(b, l, pixels)
	}
	var stat unix.Stat_t
	if err := unix.Fstat(l.fd, &stat); err != nil {
		return fmt.Errorf("stat pixel fd: %w", err)
	}
	if stat.Size < int64(l.length) {
		return fmt.Errorf("pixel fd too short: %d bytes, need %d", stat.Size, l.length)
	}
	if err = dmaBufBeginCPUAccess(l.fd); err != nil {
		return fmt.Errorf("start DMA-BUF read: %w", err)
	}
	defer // End CPU access even if mapping or copying fails.
	//
	// Example: report cache synchronization failures to the caller.
	func() {
		if err := dmaBufSync(l.fd, dmaBufSyncRead|dmaBufSyncEnd); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("end DMA-BUF read: %w", err))
		}
	}()
	data, err := unix.Mmap(l.fd, 0, l.length, unix.PROT_READ, unix.MAP_SHARED)
	if err != nil {
		return fmt.Errorf("map pixel plane: %w", err)
	}
	defer // Unmap the pixel plane and preserve any cleanup failure.
	//
	// Example: return an unmap error with any other error.
	func() {
		if err := unix.Munmap(data); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("unmap pixel plane: %w", err))
		}
	}()
	copyRGBARows(pixels, data, l, b.Width, b.Height)
	return nil
}

// validateRGBADestination enforces exact packed output geometry.
//
// Example: reject a short destination before mapping a graphics buffer.
func validateRGBADestination(b *Buffer, pixels []byte) error {
	if b == nil {
		return errors.New("invalid RGBA destination size")
	}
	return validateRGBASize(b.Width, b.Height, pixels)
}

func validateRGBASize(width, height uint32, pixels []byte) error {
	if width == 0 || height == 0 || uint64(width)*uint64(height) > uint64(math.MaxInt)/rgbaBytesPerPixel || len(pixels) != int(uint64(width)*uint64(height)*rgbaBytesPerPixel) {
		return errors.New("invalid RGBA destination size")
	}
	return nil
}

// RGBAReader owns a persistent mapping for repeated reads of one buffer.
// The buffer and its descriptors must outlive the reader. The caller must wait
// for the producer fence before each read and must serialize reads and Close.
//
// Example: retain one reader for the lifetime of a reusable graphics slot.
type RGBAReader struct {
	layout  rgbaLayout
	data    []byte
	width   uint32
	height  uint32
	ion     int
	hasION  bool
	syncDMA bool
}

// NewRGBAReader maps a supported linear capture buffer for repeated reads.
// Call Close before closing the buffer's descriptors.
//
// Example: map a capture allocation once before reading successive frames.
func NewRGBAReader(b *Buffer) (*RGBAReader, error) {
	qcom := b != nil && len(b.Handle.Ints) == 22 && b.Handle.Ints[0] == qcomHandleMagic
	return newRGBAReader(b, !qcom, qcom)
}

// newRGBAReader permits tests to use a regular file without DMA-BUF ioctls.
func newRGBAReader(b *Buffer, syncDMA, useION bool) (_ *RGBAReader, resultErr error) {
	l, err := captureRGBALayout(b)
	if err != nil {
		return nil, err
	}
	var stat unix.Stat_t
	if err := unix.Fstat(l.fd, &stat); err != nil {
		return nil, fmt.Errorf("stat pixel fd: %w", err)
	}
	if stat.Size < int64(l.length) {
		return nil, fmt.Errorf("pixel fd too short: %d bytes, need %d", stat.Size, l.length)
	}
	r := &RGBAReader{layout: l, width: b.Width, height: b.Height, ion: -1, syncDMA: syncDMA}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, r.Close())
		}
	}()
	if useION {
		r.ion, err = unix.Open("/dev/ion", unix.O_RDONLY|unix.O_CLOEXEC, 0)
		if err != nil {
			r.ion = -1
			return nil, fmt.Errorf("open ion: %w", err)
		}
		r.hasION = true
	}
	if syncDMA {
		if err := dmaBufBeginCPUAccess(l.fd); err != nil {
			return nil, fmt.Errorf("start DMA-BUF read: %w", err)
		}
	}
	prot := unix.PROT_READ
	if useION {
		prot |= unix.PROT_WRITE
	}
	r.data, err = unix.Mmap(l.fd, 0, l.length, prot, unix.MAP_SHARED)
	if syncDMA {
		err = errors.Join(err, dmaBufSync(l.fd, dmaBufSyncRead|dmaBufSyncEnd))
	}
	if err != nil {
		return nil, fmt.Errorf("map pixel plane: %w", err)
	}
	return r, nil
}

// CopyInto copies visible RGBA rows after the caller has waited for the fence.
// The destination may be partly modified if an error is returned.
//
// Example: copy a leased frame into a reusable packed destination.
func (r *RGBAReader) CopyInto(pixels []byte) (resultErr error) {
	if r.data == nil {
		return errors.New("RGBA reader closed")
	}
	if err := validateRGBASize(r.width, r.height, pixels); err != nil {
		return err
	}
	if r.syncDMA {
		if err := dmaBufBeginCPUAccess(r.layout.fd); err != nil {
			return fmt.Errorf("start DMA-BUF read: %w", err)
		}
		defer func() {
			if err := dmaBufSync(r.layout.fd, dmaBufSyncRead|dmaBufSyncEnd); err != nil {
				resultErr = errors.Join(resultErr, fmt.Errorf("end DMA-BUF read: %w", err))
			}
		}()
	}
	copyRGBARows(pixels, r.data, r.layout, r.width, r.height)
	return nil
}

func copyRGBARows(pixels, data []byte, layout rgbaLayout, width, height uint32) {
	row := int(width) * rgbaBytesPerPixel
	for y := 0; y < int(height); y++ {
		copy(pixels[y*row:(y+1)*row], data[layout.offset+y*layout.stride:layout.offset+y*layout.stride+row])
	}
}

// Close releases the mapping and its supporting descriptor. It is idempotent.
//
// Example: unmap a slot after its last frame lease is released.
func (r *RGBAReader) Close() error {
	var errs []error
	if r.data != nil {
		if err := unix.Munmap(r.data); err != nil {
			errs = append(errs, fmt.Errorf("unmap pixel plane: %w", err))
		} else {
			r.data = nil
		}
	}
	if r.data == nil && r.hasION {
		err := unix.Close(r.ion)
		r.hasION = false
		if err != nil {
			errs = append(errs, fmt.Errorf("close ion: %w", err))
		}
	}
	return errors.Join(errs...)
}
