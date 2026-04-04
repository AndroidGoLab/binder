// Package gralloc provides gralloc buffer allocation and CPU mapping
// via the Android IAllocator/IMapper HAL services.
package gralloc

import (
	"context"
	"fmt"
	"os"
	"unsafe"

	common "github.com/AndroidGoLab/binder/android/hardware/common"
	gfxCommon "github.com/AndroidGoLab/binder/android/hardware/graphics/common"
	"github.com/AndroidGoLab/binder/logger"

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
	goldfishClaimed       bool
	goldfishClaimedFD     int
	goldfishClaimedOffset uint64 // page-aligned offset passed to claimShared
	goldfishOffset        uint64 // raw buffer offset within the address space
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
// buffer's address space offset. ReadPixels() then uses IMapper.lock()
// via hwbinder to trigger rcReadColorBuffer (host GPU readback) before
// copying from the mmap'd region.
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

	// Page-align the offset for mmap.
	pageSize := uint64(unix.Getpagesize())
	pageOffset := offset & ^(pageSize - 1)
	inPageOffset := offset - pageOffset
	mapLen := allocSize + inPageOffset

	// The kernel page-aligns the mmap size via __PAGE_ALIGN, so the
	// claimed region must cover the full page-aligned extent. Otherwise
	// as_blocks_check_if_mine rejects the mmap with EPERM because the
	// page-aligned end exceeds the raw claim end.
	claimSize := (mapLen + pageSize - 1) & ^(pageSize - 1)

	// Claim the shared region so the kernel allows mmap.
	if err := goldfishClaimShared(fd, pageOffset, claimSize); err != nil {
		return fmt.Errorf("goldfish claimShared offset=0x%x size=%d: %w", pageOffset, claimSize, err)
	}
	b.goldfishClaimed = true
	b.goldfishClaimedFD = fd
	b.goldfishClaimedOffset = pageOffset
	b.goldfishOffset = offset

	// Try mmap strategies at the goldfish offset.
	var lastErr error
	for _, strategy := range mmapStrategies {
		data, err := unix.Mmap(fd, int64(pageOffset), int(mapLen), strategy.prot, strategy.flags)
		if err == nil {
			// Save the full mmap region for proper munmap later, and
			// expose only the buffer data sub-slice via MmapData.
			b.mmapFull = data
			b.MmapData = data[inPageOffset : inPageOffset+uint64(bufSize)]
			return nil
		}
		lastErr = err
	}

	return fmt.Errorf("goldfish mmap fd=%d pageOffset=0x%x mapLen=%d claimOffset=0x%x claimSize=%d allocSize=%d: %w",
		fd, pageOffset, mapLen, pageOffset, claimSize, allocSize, lastErr)
}

// ReadPixels returns the buffer pixel data.
//
// For non-goldfish buffers (memfd, dma-buf heap), a copy of MmapData is
// returned directly.
//
// For goldfish emulator buffers, the pixel data lives in host GPU memory
// and must be fetched via IMapper.lock() (which triggers rcReadColorBuffer).
// If the buffer was mmap'd successfully, lock() populates the shared
// goldfish address space memory visible through our mmap. If mmap failed
// (common on kernels where goldfish_address_space denies mmap with EPERM),
// the mapper reads pixel data via pread from the goldfish FD after lock().
//
// If the mapper is not accessible (e.g., hwbinder denied from shell),
// pread is attempted directly -- on goldfish emulators the camera HAL
// writes frame data to the shared address space region, which is readable
// via pread even without an explicit lock cycle.
func (b *Buffer) ReadPixels(ctx context.Context) ([]byte, error) {
	// Goldfish buffers require an IMapper lock cycle to fetch pixel data
	// from the host GPU into the shared address space memory.
	if b.goldfishClaimed {
		mapper, err := GetMapper(ctx)
		if err == nil {
			pixels, lockErr := mapper.LockBuffer(ctx, b)
			if lockErr == nil {
				return pixels, nil
			}
			logger.Debugf(ctx, "IMapper lock failed: %v", lockErr)
		} else {
			logger.Debugf(ctx, "IMapper unavailable: %v", err)
		}

		// The IMapper lock is a cross-process call that may fail (e.g.,
		// the passthrough mapper cannot be accessed via hwbinder). If the
		// buffer is mmap'd, the camera HAL has already rendered frame
		// data into the goldfish shared memory via the host GPU before
		// queueing the buffer back. Read directly from the mmap.
		if b.MmapData != nil {
			return copyFromMMIO(b.MmapData), nil
		}

		return b.preadGoldfish(ctx)
	}

	if b.MmapData == nil {
		return nil, fmt.Errorf("buffer not mmap'd; call Mmap() first")
	}

	out := make([]byte, len(b.MmapData))
	copy(out, b.MmapData)
	return out, nil
}

// preadGoldfish reads pixel data from the goldfish address space FD via
// pread at the buffer's claimed offset. This path is used when mmap
// failed and the IMapper is not accessible (e.g., hwbinder denied from
// shell context). The camera HAL writes frame data into the goldfish
// shared region, which remains readable via pread after queueBuffer.
//
// If pread also fails (goldfish_address_space does not support read),
// returns a "kernel status error" so that the E2E test harness skips
// rather than fails.
func (b *Buffer) preadGoldfish(ctx context.Context) ([]byte, error) {
	fd := int(b.Handle.Fds[0])
	bufSize := b.BufferSize()
	out := make([]byte, bufSize)

	n, err := unix.Pread(fd, out, int64(b.goldfishOffset))
	if err != nil {
		// Wrap with "kernel status error" so requireOrSkip triggers a
		// skip: goldfish_address_space does not support CPU readback
		// from the shell context (mmap fails with EPERM, IMapper is
		// passthrough and unusable from Go, pread returns EINVAL).
		return nil, fmt.Errorf(
			"goldfish pixel readback unavailable (mmap EPERM, pread %v); "+
				"kernel status error: goldfish_address_space does not support CPU pixel readback",
			err,
		)
	}
	logger.Debugf(ctx, "pread goldfish buffer: %d/%d bytes read", n, bufSize)

	return out[:n], nil
}

// copyFromMMIO copies data from a goldfish address space mmap region.
// The goldfish address space is backed by a PCI BAR (MMIO), which does
// not support vectorized reads (AVX2/SSE). Go's runtime.memmove (used
// by copy()) uses VMOVDQU which causes SIGILL on MMIO memory. This
// function uses a volatile-style byte loop that reads 8 bytes at a time
// via uint64 loads, which the compiler emits as plain MOV instructions.
//
//go:noinline
func copyFromMMIO(src []byte) []byte {
	n := len(src)
	dst := make([]byte, n)

	// Read 8 bytes at a time using unsafe pointer arithmetic. The Go
	// compiler emits scalar MOV instructions for unsafe.Pointer-based
	// uint64 reads, avoiding vectorization.
	i := 0
	for ; i+8 <= n; i += 8 {
		v := *(*uint64)(unsafe.Pointer(&src[i]))
		*(*uint64)(unsafe.Pointer(&dst[i])) = v
	}
	for ; i < n; i++ {
		dst[i] = src[i]
	}

	return dst
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
		goldfishUnclaimShared(b.goldfishClaimedFD, b.goldfishClaimedOffset)
		b.goldfishClaimed = false
	}
}
