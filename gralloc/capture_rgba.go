package gralloc

import (
	"errors"
	"fmt"
	"math"

	"golang.org/x/sys/unix"
)

type rgbaLayout struct {
	fd                     int
	offset, stride, length int
}

// pixelRGBALayout decodes the 64-bit gchips gralloc4 private_handle_t ABI in
// android-14.0.0_r35. Its plane fields are allocator-specific, not part of
// GraphicBuffer's portable contract.
// https://android.googlesource.com/platform/hardware/google/gchips/+/refs/tags/android-14.0.0_r35/gralloc4/src/mali_gralloc_buffer.h
func pixelRGBALayout(b *Buffer) (rgbaLayout, error) {
	var out rgbaLayout
	if b == nil || len(b.Handle.Fds) != 2 || len(b.Handle.Ints) != 77 {
		return out, fmt.Errorf("unsupported gralloc handle shape")
	}
	a := b.Handle.Ints
	u64 := func(i int) uint64 { return uint64(uint32(a[i])) | uint64(uint32(a[i+1]))<<32 }
	if a[0] != -1 || a[1] != -1 || a[2] != 0x3141592 || a[4] != 1 || a[53] != 1 || u64(75) != 400 {
		return out, fmt.Errorf("unsupported gchips handle")
	}
	if b.Format != 1 || a[7] != 1 || u64(15) != 1 {
		return out, fmt.Errorf("unsupported gralloc format or compression")
	}
	if b.Width == 0 || b.Height == 0 || uint64(b.Width) != uint64(uint32(a[5])) || uint64(b.Height) != uint64(uint32(a[6])) {
		return out, fmt.Errorf("inconsistent gralloc dimensions")
	}
	fdIndex := uint32(a[19])
	if fdIndex >= uint32(a[4]) || int(fdIndex) >= len(b.Handle.Fds) || b.Handle.Fds[fdIndex] < 0 {
		return out, fmt.Errorf("invalid pixel plane fd index %d", fdIndex)
	}
	offset, stride, width, height, allocation := u64(17), u64(23), u64(25), u64(27), u64(63)
	if width < uint64(b.Width) || height < uint64(b.Height) || stride < uint64(b.Width)*4 || stride > math.MaxInt || offset > math.MaxInt || uint64(b.Height) > math.MaxInt/stride {
		return out, fmt.Errorf("invalid pixel plane dimensions")
	}
	extent := stride * uint64(b.Height)
	planeSize := u64(21)
	if offset > math.MaxInt-extent || offset+extent > allocation || (planeSize != 0 && extent > planeSize) {
		return out, fmt.Errorf("pixel plane exceeds allocation")
	}
	out = rgbaLayout{fd: int(b.Handle.Fds[fdIndex]), offset: int(offset), stride: int(stride), length: int(offset + extent)}
	return out, nil
}

// CopyRGBA copies a supported linear gralloc buffer into tightly packed RGBA.
// The caller must wait for its acquire fence before calling this function.
func CopyRGBA(b *Buffer) (pixels []byte, resultErr error) {
	l, err := pixelRGBALayout(b)
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
	if err = dmaBufBeginCPUAccess(l.fd); err != nil {
		return nil, fmt.Errorf("start DMA-BUF read: %w", err)
	}
	defer func() {
		if err := dmaBufSync(l.fd, dmaBufSyncRead|dmaBufSyncEnd); err != nil {
			pixels = nil
			resultErr = errors.Join(resultErr, fmt.Errorf("end DMA-BUF read: %w", err))
		}
	}()
	data, err := unix.Mmap(l.fd, 0, l.length, unix.PROT_READ, unix.MAP_SHARED)
	if err != nil {
		return nil, fmt.Errorf("map pixel plane: %w", err)
	}
	defer unix.Munmap(data)
	row := int(b.Width) * 4
	out := make([]byte, row*int(b.Height))
	for y := 0; y < int(b.Height); y++ {
		copy(out[y*row:(y+1)*row], data[l.offset+y*l.stride:l.offset+y*l.stride+row])
	}
	return out, nil
}
