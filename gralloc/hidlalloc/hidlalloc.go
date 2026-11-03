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
	"os"

	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/hwparcel"
	"github.com/AndroidGoLab/binder/hwservicemanager"
	"github.com/AndroidGoLab/binder/logger"
	"github.com/AndroidGoLab/binder/parcel"
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

	// bufObjSize is sizeof(binder_buffer_object) on 64-bit.
	bufObjSize = 40

	// fdArrayType is BINDER_TYPE_FDA for an array of file descriptors.
	fdArrayType = uint32(0x66646185)

	// Binder buffer-object fields are relative to the object header.
	// bufferFlagsOffset locates buffer flags.
	//
	// Example: check parent presence.
	bufferFlagsOffset = 4
	// bufferPointerOffset locates the resolved buffer pointer.
	//
	// Example: find vector data.
	bufferPointerOffset = 8
	// bufferLengthOffset locates the buffer length.
	//
	// Example: check handle size.
	bufferLengthOffset = 16
	// bufferParentOffset locates the parent object index.
	//
	// Example: check the handle's vector parent.
	bufferParentOffset = 24
	// bufferParentOffsetOffset locates the offset within the parent.
	//
	// Example: check the handle location.
	bufferParentOffsetOffset = 32

	// Binder fd-array fields are relative to the object header.
	// fdArrayCountOffset locates the descriptor count.
	//
	// Example: compare native handle counts.
	fdArrayCountOffset = 8
	// fdArrayParentOffset locates the parent object index.
	//
	// Example: check the native handle parent.
	fdArrayParentOffset = 16
	// fdArrayParentOffsetOffset locates the offset in the parent.
	//
	// Example: check the descriptor array location.
	fdArrayParentOffsetOffset = 24

	// The allocator reply places these objects after its status and result fields.
	// vectorObjectOffset locates the HIDL vector object.
	//
	// Example: validate the reply layout.
	vectorObjectOffset = 12
	// handleObjectOffset locates the HIDL handle object.
	//
	// Example: validate its parent.
	handleObjectOffset = vectorObjectOffset + bufObjSize
	// nativeHandleSizeOffset locates the declared native handle size.
	//
	// Example: compare its buffer length.
	nativeHandleSizeOffset = handleObjectOffset + bufObjSize
	// nativeHandleObjectOffset locates the native handle buffer object.
	//
	// Example: read its pointer.
	nativeHandleObjectOffset = nativeHandleSizeOffset + 8
	// fdArrayObjectOffset locates the descriptor array object.
	//
	// Example: validate its parent.
	fdArrayObjectOffset = nativeHandleObjectOffset + bufObjSize
	// allocationObjectsEnd is the end of the descriptor array object.
	//
	// Example: check reply length.
	allocationObjectsEnd = fdArrayObjectOffset + 32

	// HIDL handles and vector headers occupy 16 bytes; native handles start with
	// three 32-bit fields, followed by descriptor and integer words.
	// hidlHeaderSize is the size of a vector or handle header.
	//
	// Example: validate a vector buffer.
	hidlHeaderSize = 16
	// hidlVectorCountOffset locates the vector element count.
	//
	// Example: require one handle.
	hidlVectorCountOffset = 8
	// nativeHandleHeaderSize is the native handle header size and version.
	//
	// Example: locate descriptor words.
	nativeHandleHeaderSize = 12
	// nativeHandleFDCountOffset locates the descriptor count.
	//
	// Example: validate the fd array.
	nativeHandleFDCountOffset = 4
	// nativeHandleIntCountOffset locates the integer count.
	//
	// Example: validate the handle size.
	nativeHandleIntCountOffset = 8
	// wireWordSize is the size of one native handle data word.
	//
	// Example: advance to the next descriptor.
	wireWordSize = 4
	// maxNativeHandleEntries limits each native handle count.
	//
	// Example: reject excessive counts.
	maxNativeHandleEntries = 1024
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

// Allocate calls IAllocator::allocate() via HIDL/hwbinder. It supports one
// buffer per call because AllocateResult represents a single native handle.
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
	return AllocateDescriptor(ctx, transport, allocatorHandle, descriptor, count)
}

// AllocateDescriptor calls IAllocator::allocate with a descriptor produced
// using the target mapper's descriptor ABI. It supports one buffer per call.
func AllocateDescriptor(
	ctx context.Context,
	transport binder.Transport,
	allocatorHandle uint32,
	descriptor []uint32,
	count uint32,
) (_ *AllocateResult, _err error) {
	if len(descriptor) == 0 {
		return nil, fmt.Errorf("hidlalloc: empty buffer descriptor")
	}
	if count == 0 {
		return nil, fmt.Errorf("hidlalloc: buffer count must be positive")
	}
	if count != 1 {
		return nil, fmt.Errorf("hidlalloc: only one buffer per allocation is supported (count=%d)", count)
	}

	hp := hwparcel.New()
	hp.WriteInterfaceToken(allocatorFQName)
	hp.WriteHidlVecUint32(append([]uint32(nil), descriptor...))
	hp.WriteUint32(count)

	reply, err := hwservicemanager.TransactHidl(ctx, transport, allocatorHandle, transactionAllocate, hp)
	if err != nil {
		return nil, fmt.Errorf("hidlalloc: allocate transaction: %w", err)
	}
	defer reply.Recycle()

	result, err := ParseAllocateResponse(reply)
	if err != nil {
		return nil, fmt.Errorf("hidlalloc: parsing response: %w", err)
	}

	return result, nil
}

// ParseAllocateResponse parses a resolved HIDL allocator reply and transfers
// its received file descriptors to the caller.
//
// Wire format after scatter-gather resolution:
//
//	[0:4]     int32 HIDL transport status
//	[4:8]     int32 allocator error
//	[8:12]    int32 stride
//	[12:52]   binder_buffer_object: hidl_vec<handle> header (bufLen=16)
//	[52:92]   binder_buffer_object: hidl_handle (bufLen=16, child of vec)
//	[92:100]  uint64 native_handle_t size
//	[100:140] binder_buffer_object: native_handle_t data (child of handle)
//	[140:172] binder_fd_array_object: fd array referencing native_handle
//	[172+]    scatter-gather buffer data (resolved by kernel driver)
//
// The native_handle_t buffer contains:
//
//	int32 version (12)
//	int32 numFds
//	int32 numInts
//	int32[numFds] fds  (translated by kernel)
//	int32[numInts] ints
func ParseAllocateResponse(p *parcel.Parcel) (*AllocateResult, error) {
	data := p.Data()
	if len(data) < 12 {
		return nil, fmt.Errorf("response too short: %d bytes", len(data))
	}

	transportStatus := int32(binary.LittleEndian.Uint32(data[0:]))
	if transportStatus != 0 {
		return nil, fmt.Errorf("HIDL allocator transport status: %d", transportStatus)
	}
	result := &AllocateResult{
		Error:  int32(binary.LittleEndian.Uint32(data[4:])),
		Stride: int32(binary.LittleEndian.Uint32(data[8:])),
	}

	if result.Error != 0 {
		return result, nil
	}

	// Binder's object-offset table identifies objects; payload bytes can contain
	// the same type values and must not be interpreted as object headers.
	objects := p.Objects()
	if len(objects) != 4 {
		return nil, fmt.Errorf("missing native handle buffer objects")
	}
	for _, off := range objects[:3] {
		if off > uint64(len(data)) || uint64(len(data))-off < bufObjSize || binary.LittleEndian.Uint32(data[off:]) != binderTypePTR {
			return nil, fmt.Errorf("invalid native handle buffer object")
		}
	}
	if objects[0] != vectorObjectOffset || objects[1] != handleObjectOffset ||
		objects[2] != nativeHandleObjectOffset || objects[3] != fdArrayObjectOffset || len(data) < allocationObjectsEnd {
		return nil, fmt.Errorf("invalid allocation object layout")
	}
	vecObject := data[vectorObjectOffset:]
	handleObject := data[handleObjectOffset:]
	nativeHandleObject := data[nativeHandleObjectOffset:]
	fdArrayObject := data[fdArrayObjectOffset:]
	if binary.LittleEndian.Uint32(vecObject[bufferFlagsOffset:]) != 0 || binary.LittleEndian.Uint32(handleObject[bufferFlagsOffset:]) != 1 ||
		binary.LittleEndian.Uint64(handleObject[bufferParentOffset:]) != 0 || binary.LittleEndian.Uint64(handleObject[bufferParentOffsetOffset:]) != 0 ||
		binary.LittleEndian.Uint32(nativeHandleObject[bufferFlagsOffset:]) != 1 || binary.LittleEndian.Uint64(nativeHandleObject[bufferParentOffset:]) != 1 ||
		binary.LittleEndian.Uint64(nativeHandleObject[bufferParentOffsetOffset:]) != 0 || binary.LittleEndian.Uint32(fdArrayObject) != fdArrayType ||
		binary.LittleEndian.Uint64(fdArrayObject[fdArrayParentOffset:]) != 2 || binary.LittleEndian.Uint64(fdArrayObject[fdArrayParentOffsetOffset:]) != nativeHandleHeaderSize {
		return nil, fmt.Errorf("invalid allocation object topology")
	}
	vecOffset := binary.LittleEndian.Uint64(vecObject[bufferPointerOffset:])
	vecLen := binary.LittleEndian.Uint64(vecObject[bufferLengthOffset:])
	childOffset := binary.LittleEndian.Uint64(handleObject[bufferPointerOffset:])
	childLen := binary.LittleEndian.Uint64(handleObject[bufferLengthOffset:])
	if vecLen != hidlHeaderSize || vecOffset > uint64(len(data))-hidlHeaderSize || binary.LittleEndian.Uint32(data[vecOffset+hidlVectorCountOffset:]) != 1 ||
		childLen != hidlHeaderSize || childOffset > uint64(len(data))-hidlHeaderSize {
		return nil, fmt.Errorf("invalid allocation buffer vector")
	}
	nativeHandleBufPtr := binary.LittleEndian.Uint64(nativeHandleObject[bufferPointerOffset:])
	nativeHandleBufLen := binary.LittleEndian.Uint64(nativeHandleObject[bufferLengthOffset:])
	if binary.LittleEndian.Uint64(data[nativeHandleSizeOffset:]) != nativeHandleBufLen {
		return nil, fmt.Errorf("native handle size mismatch")
	}

	if nativeHandleBufPtr > uint64(len(data)) || nativeHandleBufLen > uint64(len(data))-nativeHandleBufPtr {
		return result, fmt.Errorf("native_handle buffer out of range: ptr=%d len=%d dataLen=%d",
			nativeHandleBufPtr, nativeHandleBufLen, len(data))
	}

	nhData := data[nativeHandleBufPtr : nativeHandleBufPtr+nativeHandleBufLen]
	if len(nhData) < nativeHandleHeaderSize {
		return result, fmt.Errorf("native_handle too short: %d bytes", len(nhData))
	}
	if binary.LittleEndian.Uint32(nhData) != nativeHandleHeaderSize {
		return nil, fmt.Errorf("invalid native_handle version")
	}

	// native_handle_t: version, numFds, numInts, data[numFds+numInts]
	fdCount := binary.LittleEndian.Uint32(nhData[nativeHandleFDCountOffset:])
	intCount := binary.LittleEndian.Uint32(nhData[nativeHandleIntCountOffset:])
	if binary.LittleEndian.Uint64(fdArrayObject[fdArrayCountOffset:]) != uint64(fdCount) {
		return nil, fmt.Errorf("native handle descriptor count mismatch")
	}
	if fdCount > maxNativeHandleEntries || intCount > maxNativeHandleEntries || uint64(fdCount)+uint64(intCount) != uint64(len(nhData)-nativeHandleHeaderSize)/wireWordSize || (len(nhData)-nativeHandleHeaderSize)%wireWordSize != 0 {
		return nil, fmt.Errorf("invalid native_handle size or counts: have %d bytes (numFds=%d numInts=%d)",
			len(nhData), fdCount, intCount)
	}
	numFds, numInts := int(fdCount), int(intCount)

	// Read FDs (kernel has already translated them to our fd table).
	offset := nativeHandleHeaderSize
	result.Fds = make([]int32, numFds)
	for i := range numFds {
		fd, err := p.TakeReceivedFileDescriptor(int(nativeHandleBufPtr) + offset)
		if err != nil {
			for _, taken := range result.Fds[:i] {
				_ = os.NewFile(uintptr(taken), "hidl descriptor").Close()
			}
			return nil, fmt.Errorf("native_handle descriptor %d: %w", i, err)
		}
		result.Fds[i] = fd
		offset += wireWordSize
	}

	// Read ints.
	result.Ints = make([]int32, numInts)
	for i := range numInts {
		result.Ints[i] = int32(binary.LittleEndian.Uint32(nhData[offset:]))
		offset += wireWordSize
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
