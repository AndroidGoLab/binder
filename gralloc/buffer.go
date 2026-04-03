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

	// mmapFull holds the full mmap'd region when MmapData is a sub-slice
	// (e.g., goldfish buffers where the data starts at an in-page offset).
	// Munmap uses this for the actual munmap syscall.
	mmapFull []byte

	// goldfishClaimed tracks whether a goldfish address space region
	// was claimed, so Munmap can unclaim it.
	goldfishClaimed   bool
	goldfishClaimedFD int
	goldfishOffset    uint64
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

// goldfishIntsOffsetMmapedOffset is the index into the native handle's
// ints array where the ranchu/goldfish gralloc stores the mmap offset
// within the goldfish address space. This matches the cb_handle_30_t
// layout: ints[7] = mmapedOffset.
const goldfishIntsOffsetMmapedOffset = 7

// goldfishIntsOffsetAllocSize is the index into the native handle's
// ints array where the ranchu/goldfish gralloc stores the allocation
// size. This matches the cb_handle_30_t layout: ints[5] = allocSize.
const goldfishIntsOffsetAllocSize = 5

// Mmap creates a persistent mmap of this buffer's dmabuf FD.
// It tries several flag combinations to handle different allocator
// backends (AIDL gralloc, HIDL gralloc, dma-buf heap, memfd).
// On kernel 6.6+, DMA-BUF mmap requires a prior SYNC ioctl.
// The MmapData field can then be read directly. Call Munmap to release.
//
// For goldfish emulator buffers (/dev/goldfish_address_space), the FD
// requires claiming a shared region via ioctl and mmapping at the
// buffer's address space offset. The mapped data reflects whatever the
// host GPU has written; after the camera HAL queues a buffer (which
// calls rcReadColorBuffer internally), the pixels are readable.
// If the goldfish path fails, ReadPixels() falls back to the IMapper
// bridge (gralloc_bridge.so).
func (b *Buffer) Mmap() error {
	if len(b.Handle.Fds) == 0 {
		return fmt.Errorf("no FDs in gralloc buffer")
	}
	fd := int(b.Handle.Fds[0])
	bufSize := b.BufferSize()

	// Goldfish emulator: gralloc buffers use /dev/goldfish_address_space
	// backed by the host virtual GPU. Standard mmap at offset 0 returns
	// EPERM. We must claim the shared address space region and mmap at
	// the buffer's offset within the goldfish address space.
	if isGoldfishFD(fd) {
		return b.mmapGoldfish(fd, bufSize)
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

// mmapGoldfish maps a goldfish address space buffer by claiming the
// shared region and mmapping at the buffer's offset. The ranchu
// cb_handle_30_t stores the mmap offset at ints[7] and the allocation
// size at ints[5].
func (b *Buffer) mmapGoldfish(fd int, bufSize int) error {
	if len(b.Handle.Ints) <= goldfishIntsOffsetMmapedOffset {
		return fmt.Errorf("goldfish buffer: native handle too short (%d ints, need >%d)",
			len(b.Handle.Ints), goldfishIntsOffsetMmapedOffset)
	}

	offset := uint64(uint32(b.Handle.Ints[goldfishIntsOffsetMmapedOffset]))
	if offset == 0 {
		return fmt.Errorf("goldfish buffer: mmapedOffset is 0")
	}

	// Use the allocSize from the handle if available, otherwise use our
	// calculated buffer size.
	allocSize := uint64(bufSize)
	if len(b.Handle.Ints) > goldfishIntsOffsetAllocSize {
		handleAllocSize := uint64(uint32(b.Handle.Ints[goldfishIntsOffsetAllocSize]))
		if handleAllocSize > 0 {
			allocSize = handleAllocSize
		}
	}

	// Claim the shared region so the kernel allows mmap.
	if err := goldfishClaimShared(fd, offset, allocSize); err != nil {
		return fmt.Errorf("goldfish claimShared offset=0x%x size=%d: %w", offset, allocSize, err)
	}
	b.goldfishClaimed = true
	b.goldfishClaimedFD = fd
	b.goldfishOffset = offset

	// Page-align the offset for mmap.
	pageSize := int64(unix.Getpagesize())
	pageOffset := int64(offset) & ^(pageSize - 1)
	inPageOffset := int64(offset) - pageOffset
	mapLen := int(int64(allocSize) + inPageOffset)

	// Try mmap strategies at the goldfish offset.
	for _, strategy := range mmapStrategies {
		data, err := unix.Mmap(fd, pageOffset, mapLen, strategy.prot, strategy.flags)
		if err == nil {
			// Save the full mmap region for proper munmap later, and
			// expose only the buffer data sub-slice via MmapData.
			b.mmapFull = data
			b.MmapData = data[inPageOffset : inPageOffset+int64(bufSize)]
			return nil
		}
	}

	// Mmap failed even after claiming. Unclaim and report failure.
	goldfishUnclaimShared(fd, offset)
	b.goldfishClaimed = false
	return fmt.Errorf("goldfish buffer: mmap at offset=0x%x failed for all strategies", offset)
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
		return nil, fmt.Errorf("buffer not mmapped and IMapper bridge unavailable: %w", err)
	}
	return mapper.LockBuffer(b)
}

// isGoldfishFD checks if an FD points to the goldfish emulator's
// address space device.
func isGoldfishFD(fd int) bool {
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
	if b.mmapFull != nil {
		// Goldfish path: MmapData is a sub-slice of mmapFull, so
		// munmap the full region instead.
		_ = unix.Munmap(b.mmapFull)
		b.mmapFull = nil
		b.MmapData = nil
	} else if b.MmapData != nil {
		_ = unix.Munmap(b.MmapData)
		b.MmapData = nil
	}
	if b.dmaBufSynced && len(b.Handle.Fds) > 0 {
		dmaBufEndCPUAccess(int(b.Handle.Fds[0]))
		b.dmaBufSynced = false
	}
	if b.goldfishClaimed {
		goldfishUnclaimShared(b.goldfishClaimedFD, b.goldfishOffset)
		b.goldfishClaimed = false
	}
}
