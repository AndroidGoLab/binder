// Package hidlalloc implements the HIDL gralloc 3.0 IAllocator client
// for buffer allocation via hwbinder.
//
// The emulator's camera HAL uses android.hardware.graphics.allocator@3.0
// which runs on /dev/hwbinder. This package opens a separate hwbinder
// connection, looks up the allocator service via hwservicemanager, and
// calls allocate() with a BufferDescriptor constructed using the
// ranchu/goldfish encoding format.
//
// BufferDescriptor format (goldfish/ranchu):
//
//	[0] width
//	[1] height
//	[2] layerCount
//	[3] format (PixelFormat as uint32)
//	[4] usage (low 32 bits of BufferUsage)
package hidlalloc

import (
	"context"
	"encoding/binary"
	"fmt"

	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/hwparcel"
	"github.com/AndroidGoLab/binder/hwservicemanager"
	"github.com/AndroidGoLab/binder/logger"
)

const (
	// allocatorFQName is the HIDL fully-qualified interface name.
	allocatorFQName = "android.hardware.graphics.allocator@3.0::IAllocator"

	// allocatorInstance is the default service instance name.
	allocatorInstance = "default"

	// transactionAllocate is the HIDL transaction code for allocate().
	// IAllocator 3.0 methods: dumpDebugInfo=1, allocate=2.
	transactionAllocate = binder.TransactionCode(2)

	// binderTypePTR is BINDER_TYPE_PTR for scatter-gather buffer objects.
	binderTypePTR = uint32(0x70742a85)

	// binderTypeFDA is BINDER_TYPE_FDA for file descriptor arrays.
	binderTypeFDA = uint32(0x66646185)

	// bufObjSize is sizeof(binder_buffer_object) on 64-bit.
	bufObjSize = 40

	// fdaObjSize is sizeof(binder_fd_array_object) on 64-bit.
	fdaObjSize = 32
)

// AllocateResult holds the result of a gralloc 3.0 allocate() call.
type AllocateResult struct {
	// Error is the gralloc error code (0 = NONE).
	Error int32

	// Stride is the number of pixels between consecutive rows.
	Stride int32

	// Fds holds the file descriptors from the allocated native_handle.
	Fds []int32

	// Ints holds the integer data from the allocated native_handle.
	Ints []int32
}

// Allocate calls IAllocator::allocate() via HIDL/hwbinder.
func Allocate(
	ctx context.Context,
	transport binder.Transport,
	allocatorHandle uint32,
	width uint32,
	height uint32,
	layerCount uint32,
	format uint32,
	usage uint64,
	count uint32,
) (_ *AllocateResult, _err error) {
	logger.Tracef(ctx, "hidlalloc.Allocate(%dx%d fmt=0x%x usage=0x%x count=%d)", width, height, format, usage, count)
	defer func() { logger.Tracef(ctx, "/hidlalloc.Allocate: %v", _err) }()

	// BufferDescriptor: goldfish/ranchu encoding (5 uint32 values).
	descriptor := []uint32{width, height, layerCount, format, uint32(usage)}

	hp := hwparcel.New()
	hp.WriteInterfaceToken(allocatorFQName)
	hp.WriteHidlVecUint32(descriptor)
	hp.WriteUint32(count)

	reply, err := hwservicemanager.TransactHidl(ctx, transport, allocatorHandle, transactionAllocate, hp)
	if err != nil {
		return nil, fmt.Errorf("hidlalloc: allocate transaction: %w", err)
	}
	defer reply.Recycle()

	result, err := parseAllocateResponse(reply.Data())
	if err != nil {
		return nil, fmt.Errorf("hidlalloc: parsing response: %w", err)
	}

	return result, nil
}

// parseAllocateResponse parses the raw reply data from an allocate() call.
//
// Wire format after scatter-gather resolution:
//
//	[0:4]     int32 error
//	[4:8]     padding (alignment)
//	[8:12]    int32 stride
//	[12:52]   binder_buffer_object: hidl_vec<handle> header (bufLen=16)
//	[52:92]   binder_buffer_object: hidl_handle (bufLen=16, child of vec)
//	[92:132]  binder_buffer_object: native_handle_t data (child of handle)
//	[132:164] binder_fd_array_object: fd array referencing native_handle
//	[164+]    scatter-gather buffer data (resolved by kernel driver)
//
// The native_handle_t buffer contains:
//
//	int32 version (12)
//	int32 numFds
//	int32 numInts
//	int32[numFds] fds  (translated by kernel)
//	int32[numInts] ints
func parseAllocateResponse(data []byte) (*AllocateResult, error) {
	if len(data) < 12 {
		return nil, fmt.Errorf("response too short: %d bytes", len(data))
	}

	result := &AllocateResult{
		Error:  int32(binary.LittleEndian.Uint32(data[0:])),
		Stride: int32(binary.LittleEndian.Uint32(data[8:])),
	}

	if result.Error != 0 {
		return result, nil
	}

	// Find the native_handle_t data by scanning for PTR objects.
	// The third PTR object (index 2) contains the native_handle_t.
	pos := 12
	ptrIndex := 0
	var nativeHandleBufPtr uint64
	var nativeHandleBufLen uint64

	for pos+bufObjSize <= len(data) {
		objType := binary.LittleEndian.Uint32(data[pos:])

		switch objType {
		case binderTypePTR:
			bufPtr := binary.LittleEndian.Uint64(data[pos+8:])
			bufLen := binary.LittleEndian.Uint64(data[pos+16:])

			if ptrIndex == 2 {
				// Third PTR: native_handle_t data.
				nativeHandleBufPtr = bufPtr
				nativeHandleBufLen = bufLen
			}
			ptrIndex++
			pos += bufObjSize

		case binderTypeFDA:
			pos += fdaObjSize

		default:
			// Unknown object type or end of objects.
			pos += 4
		}
	}

	// Parse the native_handle_t from the resolved buffer data.
	if nativeHandleBufLen == 0 {
		return result, nil
	}

	if nativeHandleBufPtr >= uint64(len(data)) || nativeHandleBufPtr+nativeHandleBufLen > uint64(len(data)) {
		return result, fmt.Errorf("native_handle buffer out of range: ptr=%d len=%d dataLen=%d",
			nativeHandleBufPtr, nativeHandleBufLen, len(data))
	}

	nhData := data[nativeHandleBufPtr : nativeHandleBufPtr+nativeHandleBufLen]
	if len(nhData) < 12 {
		return result, fmt.Errorf("native_handle too short: %d bytes", len(nhData))
	}

	// native_handle_t: version, numFds, numInts, data[numFds+numInts]
	numFds := int(binary.LittleEndian.Uint32(nhData[4:]))
	numInts := int(binary.LittleEndian.Uint32(nhData[8:]))

	expectedSize := 12 + (numFds+numInts)*4
	if len(nhData) < expectedSize {
		return result, fmt.Errorf("native_handle data too short: have %d, need %d (numFds=%d numInts=%d)",
			len(nhData), expectedSize, numFds, numInts)
	}

	// Read FDs (kernel has already translated them to our fd table).
	offset := 12
	result.Fds = make([]int32, numFds)
	for i := range numFds {
		result.Fds[i] = int32(binary.LittleEndian.Uint32(nhData[offset:]))
		offset += 4
	}

	// Read ints.
	result.Ints = make([]int32, numInts)
	for i := range numInts {
		result.Ints[i] = int32(binary.LittleEndian.Uint32(nhData[offset:]))
		offset += 4
	}

	return result, nil
}

// GetAllocatorService looks up the gralloc 3.0 IAllocator service
// via hwservicemanager and returns its binder handle.
func GetAllocatorService(
	ctx context.Context,
	transport binder.Transport,
) (_ uint32, _err error) {
	logger.Tracef(ctx, "hidlalloc.GetAllocatorService")
	defer func() { logger.Tracef(ctx, "/hidlalloc.GetAllocatorService: %v", _err) }()

	sm := hwservicemanager.New(transport)
	handle, err := sm.GetService(ctx, allocatorFQName, allocatorInstance)
	if err != nil {
		return 0, fmt.Errorf("hidlalloc: getting allocator service: %w", err)
	}

	return handle, nil
}
