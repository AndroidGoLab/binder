// Package bridge provides typed Go wrappers around the gralloc_bridge.so
// C functions. It is the only package that imports purego or handles
// raw FFI concerns.
//
// Analogous to kernelbinder/ioctl.go which wraps raw ioctls behind
// typed Go functions.
package bridge

import (
	"fmt"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// BufferHandle is an opaque reference to an imported gralloc buffer.
type BufferHandle uintptr

// Package-level unexported function variables. Populated once by Open(),
// read-only after that. purego.RegisterFunc requires addressable
// function variables — this is the only viable location.
var (
	fnInit      func() int32
	fnImport    func(fds unsafe.Pointer, numFds int32, ints unsafe.Pointer, numInts int32) uintptr
	fnLock      func(buffer uintptr, width, height int32, data *unsafe.Pointer) int32
	fnLockYCbCr func(buffer uintptr, width, height int32, y, cb, cr *unsafe.Pointer, yStride, cStride, chromaStep *uint32) int32
	fnUnlock    func(buffer uintptr)
	fnFree      func(buffer uintptr)
)

var soPaths = []string{
	"/data/local/tmp/gralloc_bridge.so",
	"./gralloc_bridge.so",
}

var (
	openOnce sync.Once
	openErr  error
)

// Open loads the gralloc_bridge.so and initializes the HIDL IMapper.
// Safe to call multiple times; the .so is loaded only once.
func Open() error {
	openOnce.Do(func() {
		openErr = doOpen()
	})
	return openErr
}

func doOpen() error {
	var lib uintptr
	var errs []error
	for _, path := range soPaths {
		var err error
		lib, err = purego.Dlopen(path, purego.RTLD_LAZY)
		if err == nil {
			errs = nil
			break
		}
		errs = append(errs, fmt.Errorf("dlopen %s: %w", path, err))
	}
	if len(errs) > 0 {
		return fmt.Errorf("failed to load gralloc bridge: %v", errs)
	}

	symbols := []struct {
		name string
		fn   interface{}
	}{
		{"bridge_init", &fnInit},
		{"bridge_import", &fnImport},
		{"bridge_lock", &fnLock},
		{"bridge_lock_ycbcr", &fnLockYCbCr},
		{"bridge_unlock", &fnUnlock},
		{"bridge_free", &fnFree},
	}
	for _, s := range symbols {
		addr, err := purego.Dlsym(lib, s.name)
		if err != nil {
			return fmt.Errorf("dlsym %s: %w", s.name, err)
		}
		purego.RegisterFunc(s.fn, addr)
	}

	if ret := fnInit(); ret != 0 {
		return fmt.Errorf("bridge_init: error %d", ret)
	}
	return nil
}

// ImportBuffer imports a raw gralloc buffer handle. The returned
// BufferHandle must be released with Free when no longer needed.
func ImportBuffer(fds []int32, ints []int32) (BufferHandle, error) {
	h := fnImport(
		unsafe.Pointer(&fds[0]), int32(len(fds)),
		unsafe.Pointer(&ints[0]), int32(len(ints)),
	)
	if h == 0 {
		return 0, fmt.Errorf("bridge_import: failed")
	}
	return BufferHandle(h), nil
}

// LockGeneric locks a buffer for CPU read and copies the pixel data
// into a Go byte slice. The buffer is unlocked before returning.
func LockGeneric(h BufferHandle, width, height int32, bufSize int) ([]byte, error) {
	var dataPtr unsafe.Pointer
	if ret := fnLock(uintptr(h), width, height, &dataPtr); ret != 0 {
		return nil, fmt.Errorf("bridge_lock: error %d", ret)
	}
	result := make([]byte, bufSize)
	copy(result, unsafe.Slice((*byte)(dataPtr), bufSize))
	fnUnlock(uintptr(h))
	return result, nil
}

// LockYCbCr locks a YCbCr buffer for CPU read and copies all three
// planes (Y, Cb, Cr) into a Go byte slice. The buffer is unlocked
// before returning.
func LockYCbCr(h BufferHandle, width, height int32) ([]byte, error) {
	var yPtr, cbPtr, crPtr unsafe.Pointer
	var yStride, cStride, chromaStep uint32
	if ret := fnLockYCbCr(uintptr(h), width, height,
		&yPtr, &cbPtr, &crPtr,
		&yStride, &cStride, &chromaStep,
	); ret != 0 {
		return nil, fmt.Errorf("bridge_lock_ycbcr: error %d", ret)
	}

	ySize := int(yStride) * int(height)
	chromaHeight := int(height) / 2
	cbSize := int(cStride) * chromaHeight
	crSize := int(cStride) * chromaHeight

	result := make([]byte, ySize+cbSize+crSize)
	copy(result[:ySize], unsafe.Slice((*byte)(yPtr), ySize))
	if cbPtr != nil && cbSize > 0 {
		copy(result[ySize:ySize+cbSize], unsafe.Slice((*byte)(cbPtr), cbSize))
	}
	if crPtr != nil && crSize > 0 {
		copy(result[ySize+cbSize:], unsafe.Slice((*byte)(crPtr), crSize))
	}
	fnUnlock(uintptr(h))
	return result, nil
}

// Free releases an imported buffer handle.
func Free(h BufferHandle) {
	fnFree(uintptr(h))
}
