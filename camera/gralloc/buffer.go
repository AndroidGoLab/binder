// Package gralloc provides gralloc buffer allocation via the Android
// IAllocator HAL service.
package gralloc

import (
	"fmt"

	common "github.com/xaionaro-go/binder/android/hardware/common"

	"golang.org/x/sys/unix"
)

// Buffer holds a gralloc-allocated buffer with its NativeHandle.
type Buffer struct {
	Handle common.NativeHandle
	Stride int32
	Width  uint32
	Height uint32
	Format int32
	Usage  uint64

	// MmapData holds a persistent mmap of the dmabuf, set by Mmap().
	// Keeping it mapped avoids mmap/munmap syscalls per frame read.
	MmapData []byte
}

// Mmap creates a persistent read-only mmap of this buffer's dmabuf FD.
// The MmapData field can then be read directly. Call Munmap to release.
func (b *Buffer) Mmap() error {
	if len(b.Handle.Fds) == 0 {
		return fmt.Errorf("no FDs in gralloc buffer")
	}
	fd := int(b.Handle.Fds[0])
	// YCbCr_420_888: Y plane (w*h) + CbCr interleaved (w*h/2).
	bufSize := int(b.Width) * int(b.Height) * 3 / 2
	data, err := unix.Mmap(fd, 0, bufSize, unix.PROT_READ, unix.MAP_SHARED)
	if err != nil {
		return fmt.Errorf("mmap fd=%d size=%d: %w", fd, bufSize, err)
	}
	b.MmapData = data
	return nil
}

// Munmap releases the persistent mmap created by Mmap.
func (b *Buffer) Munmap() {
	if b.MmapData != nil {
		_ = unix.Munmap(b.MmapData)
		b.MmapData = nil
	}
}
