// Package dmaheap provides buffer allocation via Linux dma-buf heaps
// (/dev/dma_heap/system). This is a fallback allocator when AIDL/HIDL
// gralloc services are unavailable.
//
// Buffers allocated this way are plain dma-buf FDs without gralloc
// metadata. They work for camera HALs that can import raw dma-buf FDs
// but may not work with HALs that require gralloc-specific handle formats.
package dmaheap

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	// dmaHeapPath is the default dma-buf heap device.
	dmaHeapPath = "/dev/dma_heap/system"

	// dmaHeapUncachedPath is the uncached variant.
	dmaHeapUncachedPath = "/dev/dma_heap/system-uncached"
)

// dmaHeapAllocData matches struct dma_heap_allocation_data from
// linux/dma-heap.h.
type dmaHeapAllocData struct {
	Len       uint64
	Fd        uint32
	FdFlags   uint32
	HeapFlags uint64
}

// DMA_HEAP_IOCTL_ALLOC = _IOWR('H', 0x0, struct dma_heap_allocation_data)
// Direction: read|write, type: 'H' (0x48), nr: 0, size: 24
var dmaHeapIoctlAlloc = iowrH(0x48, 0, unsafe.Sizeof(dmaHeapAllocData{}))

func iowrH(typ, nr, size uintptr) uintptr {
	const (
		iocWrite = uintptr(1)
		iocRead  = uintptr(2)

		iocNRShift   = 0
		iocTypeShift = 8
		iocSizeShift = 16
		iocDirShift  = 30
	)
	return ((iocRead | iocWrite) << iocDirShift) |
		(typ << iocTypeShift) |
		(nr << iocNRShift) |
		(size << iocSizeShift)
}

// Allocate allocates a dma-buf of the given size via /dev/dma_heap/system.
// Returns the file descriptor for the dma-buf.
// The caller is responsible for closing the FD.
func Allocate(size uint64) (int32, error) {
	return allocateFromHeap(dmaHeapPath, size)
}

// AllocateUncached allocates an uncached dma-buf.
func AllocateUncached(size uint64) (int32, error) {
	return allocateFromHeap(dmaHeapUncachedPath, size)
}

func allocateFromHeap(path string, size uint64) (int32, error) {
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, fmt.Errorf("open %s: %w", path, err)
	}
	defer unix.Close(fd)

	allocData := dmaHeapAllocData{
		Len:     size,
		FdFlags: uint32(unix.O_CLOEXEC | unix.O_RDWR),
	}

	_, _, errno := unix.Syscall(
		unix.SYS_IOCTL,
		uintptr(fd),
		dmaHeapIoctlAlloc,
		uintptr(unsafe.Pointer(&allocData)),
	)
	if errno != 0 {
		return -1, fmt.Errorf("DMA_HEAP_IOCTL_ALLOC size=%d: %w", size, errno)
	}

	return int32(allocData.Fd), nil
}
