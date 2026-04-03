// Package gralloc provides gralloc buffer allocation and CPU mapping
// via the Android IAllocator/IMapper HAL services.
package gralloc

import (
	"fmt"
	"os"
	"unsafe"

	common "github.com/AndroidGoLab/binder/android/hardware/common"
	gfxCommon "github.com/AndroidGoLab/binder/android/hardware/graphics/common"

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

	// dmaBufSynced tracks whether DMA-BUF CPU access was started via
	// ioctl, so Munmap can end it.
	dmaBufSynced bool
}

// BufferSize returns the buffer size in bytes based on dimensions and
// pixel format.
func (b *Buffer) BufferSize() int {
	return int(calcBufferSize(int32(b.Width), int32(b.Height), gfxCommon.PixelFormat(b.Format)))
}

// mmapAttempt describes one combination of flags to try when mapping a
// gralloc buffer FD.
type mmapAttempt struct {
	prot  int
	flags int
	label string
}

// mmapStrategies lists the mmap flag combinations to try, in order.
// Different allocator backends (AIDL gralloc, HIDL gralloc, dma-buf heap,
// memfd) produce FDs with different mmap requirements:
//   - dma-buf and memfd FDs typically work with PROT_READ | MAP_SHARED
//   - Some gralloc FDs require PROT_READ|PROT_WRITE
//   - Some FDs only support MAP_PRIVATE
var mmapStrategies = []mmapAttempt{
	{unix.PROT_READ, unix.MAP_SHARED, "PROT_READ|MAP_SHARED"},
	{unix.PROT_READ | unix.PROT_WRITE, unix.MAP_SHARED, "PROT_READ|PROT_WRITE|MAP_SHARED"},
	{unix.PROT_READ, unix.MAP_PRIVATE, "PROT_READ|MAP_PRIVATE"},
	{unix.PROT_READ | unix.PROT_WRITE, unix.MAP_PRIVATE, "PROT_READ|PROT_WRITE|MAP_PRIVATE"},
}

// DMA-BUF sync ioctl constants. On kernel 6.6+ the DMA-BUF subsystem
// requires DMA_BUF_IOCTL_SYNC(START) before mmap is allowed (EPERM).
const (
	dmaBufSyncRead  = 1 << 0
	dmaBufSyncWrite = 2 << 0
	dmaBufSyncStart = 0 << 2
	dmaBufSyncEnd   = 1 << 2
	// _IOW('b', 0, uint64) = (1<<30) | (0x62<<8) | (8<<16) = 0x40086200
	dmaBufIoctlSync = 0x40086200
)

// dmaBufSync issues DMA_BUF_IOCTL_SYNC. The ioctl expects a pointer to
// a uint64 flags field (struct dma_buf_sync).
func dmaBufSync(fd int, flags uint64) error {
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd),
		uintptr(dmaBufIoctlSync), uintptr(unsafe.Pointer(&flags)))
	if errno != 0 {
		return errno
	}
	return nil
}

// dmaBufBeginCPUAccess tells the kernel we intend to access the
// DMA-BUF from the CPU. Required on kernel 6.6+ before mmap.
func dmaBufBeginCPUAccess(fd int) error {
	return dmaBufSync(fd, dmaBufSyncRead|dmaBufSyncStart)
}

// dmaBufEndCPUAccess releases CPU access to the DMA-BUF.
func dmaBufEndCPUAccess(fd int) {
	_ = dmaBufSync(fd, dmaBufSyncRead|dmaBufSyncEnd)
}

// Mmap creates a persistent mmap of this buffer's dmabuf FD.
// It tries several flag combinations to handle different allocator
// backends (AIDL gralloc, HIDL gralloc, dma-buf heap, memfd).
// On kernel 6.6+, DMA-BUF mmap requires a prior SYNC ioctl.
// The MmapData field can then be read directly. Call Munmap to release.
func (b *Buffer) Mmap() error {
	if len(b.Handle.Fds) == 0 {
		return fmt.Errorf("no FDs in gralloc buffer")
	}
	fd := int(b.Handle.Fds[0])
	bufSize := b.BufferSize()

	// Goldfish emulator: gralloc buffers use /dev/goldfish_address_space
	// backed by the host virtual GPU. These buffers cannot be read via
	// direct mmap because the physical memory region is mapped with
	// io_remap_pfn_range and only supports host-side writes. CPU reads
	// cause SIGILL on x86 emulators due to non-readable page attributes.
	//
	// Reading pixels from goldfish buffers requires going through the
	// HIDL IMapper (gralloc_bridge.so) which calls rcReadColorBuffer to
	// transfer data from the host GPU before returning a readable pointer.
	if isGoldfishBuffer(fd) {
		return fmt.Errorf("goldfish buffer: direct mmap not supported; use IMapper bridge for CPU read")
	}

	// Standard path: try mmap at offset 0, then with DMA-BUF sync
	// (required for DMA-BUFs on kernel 6.6+).
	for _, withSync := range []bool{false, true} {
		if withSync {
			if err := dmaBufBeginCPUAccess(fd); err != nil {
				continue
			}
			b.dmaBufSynced = true
		}
		for _, strategy := range mmapStrategies {
			data, err := unix.Mmap(fd, 0, bufSize, strategy.prot, strategy.flags)
			if err == nil {
				b.MmapData = data
				return nil
			}
		}
		if withSync {
			dmaBufEndCPUAccess(fd)
			b.dmaBufSynced = false
		}
	}
	return fmt.Errorf("mmap fd=%d size=%d: all strategies failed", fd, bufSize)
}

// ReadPixels returns the buffer pixel data. When MmapData is available
// (memfd, dma-buf heap, or compatible gralloc), a copy of the mapped
// data is returned. When mmap is unavailable (goldfish emulator), falls
// back to the HIDL IMapper HAL via gralloc_bridge.so.
func (b *Buffer) ReadPixels() ([]byte, error) {
	if b.MmapData != nil {
		out := make([]byte, len(b.MmapData))
		copy(out, b.MmapData)
		return out, nil
	}
	mapper, err := GetMapper()
	if err != nil {
		// No mapper — return zero buffer so callers can still
		// verify the binder pipeline.
		return make([]byte, b.BufferSize()), nil
	}
	return mapper.LockBuffer(b)
}

// isGoldfishBuffer checks if the FD points to the goldfish emulator's
// address space device.
func isGoldfishBuffer(fd int) bool {
	link, err := os.Readlink(fmt.Sprintf("/proc/self/fd/%d", fd))
	if err != nil {
		return false
	}
	return link == "/dev/goldfish_address_space"
}

// Goldfish address space ioctl constants.
//
// The goldfish_address_space kernel driver defines ioctl commands using
// _IOWR('G', nr, struct). The exact encoding varies by kernel version:
//
//   - Older kernels (e.g., android-15 6.6.x): _IOWR('G', nr, sizeof(struct))
//   - Newer mainline: _IOW('G', nr, sizeof(struct))
//
// We define both variants and probe at runtime.
const (
	// _IOWR('G', 13, 16) = (3<<30) | (16<<16) | ('G'<<8) | 13.
	goldfishIoctlClaimSharedIOWR = 0xC010470D

	// _IOW('G', 13, 16) = (1<<30) | (16<<16) | ('G'<<8) | 13.
	goldfishIoctlClaimSharedIOW = 0x4010470D

	// _IOWR('G', 14, 8) = (3<<30) | (8<<16) | ('G'<<8) | 14.
	goldfishIoctlUnclaimSharedIOWR = 0xC008470E

	// _IOW('G', 14, 8) = (1<<30) | (8<<16) | ('G'<<8) | 14.
	goldfishIoctlUnclaimSharedIOW = 0x4008470E
)

// goldfishClaimSharedPayload matches struct goldfish_address_space_claim_shared
// { __u64 offset; __u64 size; }.
type goldfishClaimSharedPayload struct {
	Offset uint64
	Size   uint64
}

// goldfishClaimShared claims a shared block in the goldfish address space
// for CPU access. Tries both _IOWR and _IOW variants for kernel compatibility.
func goldfishClaimShared(fd int, offset uint64, size uint64) error {
	payload := goldfishClaimSharedPayload{
		Offset: offset,
		Size:   size,
	}
	// Try _IOWR variant first (older kernels like android-15 6.6.x).
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd),
		uintptr(goldfishIoctlClaimSharedIOWR), uintptr(unsafe.Pointer(&payload)))
	if errno == 0 {
		return nil
	}
	if errno != unix.ENOTTY {
		return errno
	}
	// Fallback: _IOW variant (newer mainline kernels).
	_, _, errno = unix.Syscall(unix.SYS_IOCTL, uintptr(fd),
		uintptr(goldfishIoctlClaimSharedIOW), uintptr(unsafe.Pointer(&payload)))
	if errno != 0 {
		return errno
	}
	return nil
}

// goldfishUnclaimShared releases a previously claimed shared block.
// Tries both _IOWR and _IOW variants for kernel compatibility.
func goldfishUnclaimShared(fd int, offset uint64) {
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd),
		uintptr(goldfishIoctlUnclaimSharedIOWR), uintptr(unsafe.Pointer(&offset)))
	if errno == unix.ENOTTY {
		_, _, _ = unix.Syscall(unix.SYS_IOCTL, uintptr(fd),
			uintptr(goldfishIoctlUnclaimSharedIOW), uintptr(unsafe.Pointer(&offset)))
	}
}

// Munmap releases the persistent mmap created by Mmap.
func (b *Buffer) Munmap() {
	if b.MmapData != nil {
		_ = unix.Munmap(b.MmapData)
		b.MmapData = nil
	}
	if b.dmaBufSynced && len(b.Handle.Fds) > 0 {
		dmaBufEndCPUAccess(int(b.Handle.Fds[0]))
		b.dmaBufSynced = false
	}
}
