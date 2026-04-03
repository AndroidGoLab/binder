// Package hwparcel implements the HIDL HwParcel wire format for hwbinder
// transactions. Unlike AIDL parcels that use inline data, HIDL uses
// scatter-gather serialization with binder_buffer_object entries
// (BINDER_TYPE_PTR) that reference external data buffers.
//
// The HIDL wire format is:
//   - Interface token: null-terminated C string in the data buffer
//   - Structured data: binder_buffer_object entries in the data buffer,
//     each pointing to a separate buffer in process memory.
//     The kernel copies these buffers to the target process.
//   - hidl_string: 16-byte struct (8-byte pointer + 4-byte size + 4-byte pad)
//     with string data as a child buffer
//   - hidl_vec<T>: 16-byte struct (8-byte pointer + 4-byte size + 4-byte pad)
//     with element data as a child buffer
package hwparcel

import (
	"encoding/binary"
	"fmt"
	"runtime"
	"unsafe"

	"github.com/AndroidGoLab/binder/parcel"
)

const (
	// binderTypePTR is BINDER_TYPE_PTR for scatter-gather buffer objects.
	// Kernel value: B_PACK_CHARS('p','t','*',0x85) = 0x70742a85.
	binderTypePTR = uint32(0x70742a85)

	// binderBufferFlagHasParent indicates this buffer is embedded in a parent.
	binderBufferFlagHasParent = uint32(1)

	// binderBufferObjectSize is sizeof(binder_buffer_object) on 64-bit:
	//   binder_object_header.type: __u32 (4, offset 0)
	//   flags:                     __u32 (4, offset 4)
	//   buffer:                    __u64 (8, offset 8)
	//   length:                    __u64 (8, offset 16)
	//   parent:                    __u64 (8, offset 24)
	//   parent_offset:             __u64 (8, offset 32)
	//   Total = 40
	binderBufferObjectSize = 40

	// hidlStringSize is sizeof(hidl_string) on 64-bit:
	// hidl_pointer<char> (8) + uint32_t size (4) + bool owns (1) + pad[3] = 16.
	hidlStringSize = 16

	// hidlVecHeaderSize is sizeof(hidl_vec<T>) on 64-bit:
	// hidl_pointer<T> (8) + uint32_t size (4) + bool owns (1) + pad[3] = 16.
	hidlVecHeaderSize = 16
)

// bufferEntry holds a scatter-gather buffer and its metadata.
type bufferEntry struct {
	data []byte
}

// HwParcel builds a HIDL HwParcel for hwbinder transactions.
// It produces data and objects arrays compatible with the kernel binder
// scatter-gather protocol.
type HwParcel struct {
	// data holds the inline data (interface token + binder_buffer_object structs).
	data []byte

	// objects holds offsets into data where binder_buffer_object entries start.
	objects []uint64

	// buffers holds the actual buffer data referenced by binder_buffer_objects.
	// Index i corresponds to the i-th buffer object added.
	buffers []bufferEntry

	// bufferCount tracks the number of buffer objects added.
	bufferCount int
}

// New creates a new empty HwParcel.
func New() *HwParcel {
	return &HwParcel{}
}

// WriteInterfaceToken writes the HIDL interface descriptor as a
// null-terminated C string in the data buffer.
func (p *HwParcel) WriteInterfaceToken(descriptor string) {
	p.data = append(p.data, []byte(descriptor)...)
	p.data = append(p.data, 0)
	// Align to 4 bytes.
	for len(p.data)%4 != 0 {
		p.data = append(p.data, 0)
	}
}

// writeBufferObject writes a binder_buffer_object into the data buffer,
// records the offset in objects, and stores the buffer data for KeepAlive.
// Returns the buffer index (handle) for parent references.
func (p *HwParcel) writeBufferObject(
	buf []byte,
	flags uint32,
	parentHandle int,
	parentOffset uint64,
) int {
	handle := p.bufferCount
	p.bufferCount++

	// Record the offset where this binder_buffer_object starts.
	offset := uint64(len(p.data))
	p.objects = append(p.objects, offset)

	// Store the buffer data.
	p.buffers = append(p.buffers, bufferEntry{data: buf})

	// Build the binder_buffer_object in the data buffer.
	// The buffer pointer will be patched in ToParcel() once all buffers
	// are finalized.
	obj := make([]byte, binderBufferObjectSize)

	// binder_object_header.type (offset 0)
	binary.LittleEndian.PutUint32(obj[0:], binderTypePTR)
	// flags (offset 4)
	binary.LittleEndian.PutUint32(obj[4:], flags)
	// buffer (offset 8, placeholder, patched in ToParcel)
	binary.LittleEndian.PutUint64(obj[8:], 0)
	// length (offset 16)
	binary.LittleEndian.PutUint64(obj[16:], uint64(len(buf)))
	// parent (offset 24)
	binary.LittleEndian.PutUint64(obj[24:], uint64(parentHandle))
	// parent_offset (offset 32)
	binary.LittleEndian.PutUint64(obj[32:], parentOffset)

	p.data = append(p.data, obj...)

	return handle
}

// WriteBuffer writes a top-level scatter-gather buffer object.
// Returns the handle (index) for parent references.
func (p *HwParcel) WriteBuffer(buf []byte) int {
	return p.writeBufferObject(buf, 0, 0, 0)
}

// WriteEmbeddedBuffer writes a child scatter-gather buffer embedded within
// a parent buffer. parentOffset is the byte offset within the parent buffer
// where the pointer to this child buffer lives (typically 0 for hidl_string
// and hidl_vec, since the pointer is the first field).
func (p *HwParcel) WriteEmbeddedBuffer(
	buf []byte,
	parentHandle int,
	parentOffset uint64,
) int {
	return p.writeBufferObject(buf, binderBufferFlagHasParent, parentHandle, parentOffset)
}

// WriteHidlString writes a hidl_string value. Creates two buffer objects:
//  1. The hidl_string struct (16 bytes)
//  2. The string data (child of #1, linked at the pointer field offset 0)
func (p *HwParcel) WriteHidlString(s string) {
	structBuf := make([]byte, hidlStringSize)
	// pointer at offset 0 is zero (kernel patches it)
	binary.LittleEndian.PutUint32(structBuf[8:], uint32(len(s)))
	// owns=false and pad are already zero

	parentHandle := p.WriteBuffer(structBuf)

	// String data with null terminator.
	strData := make([]byte, len(s)+1)
	copy(strData, s)

	p.WriteEmbeddedBuffer(strData, parentHandle, 0)
}

// WriteHidlVecUint32 writes a hidl_vec<uint32_t>. Creates two buffer objects:
//  1. The hidl_vec header (16 bytes)
//  2. The uint32 array data (child of #1)
func (p *HwParcel) WriteHidlVecUint32(values []uint32) {
	headerBuf := make([]byte, hidlVecHeaderSize)
	binary.LittleEndian.PutUint32(headerBuf[8:], uint32(len(values)))

	parentHandle := p.WriteBuffer(headerBuf)

	dataBuf := make([]byte, len(values)*4)
	for i, v := range values {
		binary.LittleEndian.PutUint32(dataBuf[i*4:], v)
	}

	p.WriteEmbeddedBuffer(dataBuf, parentHandle, 0)
}

// WriteNullBinder writes a null HIDL binder reference into the inline
// data. HIDL null binders use flat_binder_object with BINDER_TYPE_BINDER
// and binder=0, cookie=0. Unlike AIDL, HIDL does NOT append a stability
// level. The offset is NOT recorded in the objects array (matching AOSP
// behavior where null binder objects are inert data).
func (p *HwParcel) WriteNullBinder() {
	const flatBinderObjectSize = 24
	const binderTypeBinder = uint32(0x73622a85)

	obj := make([]byte, flatBinderObjectSize)
	binary.LittleEndian.PutUint32(obj[0:], binderTypeBinder)
	// flags, binder pointer, cookie are all zero.
	p.data = append(p.data, obj...)
}

// WriteLocalBinder writes a local HIDL binder reference (BINDER_TYPE_BINDER)
// into the inline data with a real binder/cookie pair. The kernel will
// create a binder_node for this object and translate it to a
// BINDER_TYPE_HANDLE in the receiving process.
//
// Unlike AIDL, HIDL does NOT write a stability level after the flat_binder_object.
// The offset IS recorded in the objects array so the kernel processes it.
func (p *HwParcel) WriteLocalBinder(
	binderPtr uintptr,
	cookie uintptr,
) {
	const flatBinderObjectSize = 24
	const binderTypeBinder = uint32(0x73622a85)
	const binderFlagsAcceptFDs = uint32(0x100 | 0x13)

	offset := uint64(len(p.data))
	p.objects = append(p.objects, offset)

	obj := make([]byte, flatBinderObjectSize)
	binary.LittleEndian.PutUint32(obj[0:], binderTypeBinder)
	binary.LittleEndian.PutUint32(obj[4:], binderFlagsAcceptFDs)
	binary.LittleEndian.PutUint64(obj[8:], uint64(binderPtr))
	binary.LittleEndian.PutUint64(obj[16:], uint64(cookie))

	p.data = append(p.data, obj...)
}

// WriteBool writes a boolean as a uint8 into the inline data buffer,
// padded to 4-byte alignment.
func (p *HwParcel) WriteBool(v bool) {
	b := make([]byte, 4)
	if v {
		b[0] = 1
	}
	p.data = append(p.data, b...)
}

// WriteInt32 writes an int32 directly into the inline data buffer.
func (p *HwParcel) WriteInt32(v int32) {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, uint32(v))
	p.data = append(p.data, b...)
}

// WriteUint64 writes a uint64 directly into the inline data buffer.
func (p *HwParcel) WriteUint64(v uint64) {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, v)
	p.data = append(p.data, b...)
}

// WriteHidlVecBytes writes a hidl_vec<uint8_t>. Creates two buffer objects:
//  1. The hidl_vec header (16 bytes)
//  2. The byte array data (child of #1)
func (p *HwParcel) WriteHidlVecBytes(data []byte) {
	headerBuf := make([]byte, hidlVecHeaderSize)
	binary.LittleEndian.PutUint32(headerBuf[8:], uint32(len(data)))

	parentHandle := p.WriteBuffer(headerBuf)

	// Copy data to ensure it stays alive.
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)

	p.WriteEmbeddedBuffer(dataCopy, parentHandle, 0)
}

// WriteUint32 writes a uint32 directly into the inline data buffer
// (not as a scatter-gather buffer object). Used for simple scalar
// arguments that follow buffer objects.
func (p *HwParcel) WriteUint32(v uint32) {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, v)
	p.data = append(p.data, b...)
}

// BufferCount returns the number of scatter-gather buffer objects.
func (p *HwParcel) BufferCount() int {
	return p.bufferCount
}

// DataBytes returns the raw inline data bytes.
func (p *HwParcel) DataBytes() []byte {
	return p.data
}

// ObjectOffsets returns the objects array (byte offsets of binder objects).
func (p *HwParcel) ObjectOffsets() []uint64 {
	return p.objects
}

// DumpBufferObjects returns a debug description of all buffer objects
// in the parcel (type, flags, length, parent, parent_offset).
func (p *HwParcel) DumpBufferObjects() []string {
	var result []string
	for i, off := range p.objects {
		if off+binderBufferObjectSize > uint64(len(p.data)) {
			result = append(result, fmt.Sprintf("[%d] off=%d: truncated", i, off))
			continue
		}
		objType := binary.LittleEndian.Uint32(p.data[off:])
		flags := binary.LittleEndian.Uint32(p.data[off+4:])
		length := binary.LittleEndian.Uint64(p.data[off+16:])
		parent := binary.LittleEndian.Uint64(p.data[off+24:])
		parentOff := binary.LittleEndian.Uint64(p.data[off+32:])
		result = append(result, fmt.Sprintf("[%d] off=%d type=%#x flags=%#x len=%d parent=%d parentOff=%d bufLen=%d",
			i, off, objType, flags, length, parent, parentOff, len(p.buffers[i].data)))
	}
	return result
}

// ToParcel converts this HwParcel into a standard parcel.Parcel.
// It patches the buffer pointers in each binder_buffer_object to point
// to the actual buffer data in memory. The returned keepAlive slice must
// be kept alive (via runtime.KeepAlive) until the transaction completes.
func (p *HwParcel) ToParcel() (*parcel.Parcel, [][]byte) {
	// Patch buffer pointers in the data.
	for i, entry := range p.buffers {
		objOffset := p.objects[i]
		// The buffer pointer is at offset 8 within the binder_buffer_object.
		ptrOffset := objOffset + 8
		if len(entry.data) > 0 {
			ptr := uint64(uintptr(unsafe.Pointer(&entry.data[0])))
			binary.LittleEndian.PutUint64(p.data[ptrOffset:], ptr)
		}
	}

	result := parcel.FromBytesWithObjects(p.data, p.objects)

	// Collect all buffer slices for KeepAlive.
	keepAlive := make([][]byte, len(p.buffers))
	for i, entry := range p.buffers {
		keepAlive[i] = entry.data
	}

	return result, keepAlive
}

// KeepBuffersAlive calls runtime.KeepAlive on all buffer slices.
// Must be called after the binder ioctl completes.
func KeepBuffersAlive(buffers [][]byte) {
	for _, b := range buffers {
		runtime.KeepAlive(b)
	}
}

// ResponseParcel wraps a reply parcel.Parcel from a HIDL transaction
// and provides methods to read HIDL-formatted response data.
// HIDL replies don't use scatter-gather -- the kernel copies all
// buffer data into the target's mmap region, and the driver copies
// it into the reply parcel data buffer inline.
type ResponseParcel struct {
	p *parcel.Parcel
}

// NewResponseParcel wraps a standard parcel for HIDL response reading.
func NewResponseParcel(p *parcel.Parcel) *ResponseParcel {
	return &ResponseParcel{p: p}
}

// ReadInt32 reads an int32 from the response.
func (r *ResponseParcel) ReadInt32() (int32, error) {
	return r.p.ReadInt32()
}

// ReadUint32 reads a uint32 from the response.
func (r *ResponseParcel) ReadUint32() (uint32, error) {
	return r.p.ReadUint32()
}

// ReadStrongBinder reads a binder handle from the HIDL response.
// HIDL does NOT write a stability level after the flat_binder_object
// (unlike AIDL).
func (r *ResponseParcel) ReadStrongBinder() (uint32, error) {
	const flatBinderObjectSize = 24
	b, err := r.p.Read(flatBinderObjectSize)
	if err != nil {
		return 0, fmt.Errorf("hwparcel: reading binder object: %w", err)
	}

	objType := binary.LittleEndian.Uint32(b[0:])
	handle := binary.LittleEndian.Uint32(b[8:])

	const binderTypeHandle = uint32(0x73682a85)
	const binderTypeBinder = uint32(0x73622a85)

	switch objType {
	case binderTypeHandle, binderTypeBinder:
		return handle, nil
	default:
		return 0, fmt.Errorf("hwparcel: unexpected binder type %#x", objType)
	}
}

// Position returns the current read position.
func (r *ResponseParcel) Position() int {
	return r.p.Position()
}

// DataLen returns the total data length.
func (r *ResponseParcel) DataLen() int {
	return r.p.Len()
}

// Remaining returns the number of unread bytes.
func (r *ResponseParcel) Remaining() int {
	return r.p.Len() - r.p.Position()
}

// ReadRawBytes reads n raw bytes.
func (r *ResponseParcel) ReadRawBytes(n int) ([]byte, error) {
	return r.p.Read(n)
}

// Underlying returns the wrapped parcel for low-level access.
func (r *ResponseParcel) Underlying() *parcel.Parcel {
	return r.p
}

// SkipBytes advances the read position by n bytes (4-byte aligned).
func (r *ResponseParcel) SkipBytes(n int) error {
	_, err := r.p.Read(n)
	return err
}

// ReadNativeHandle reads a native_handle_t from the HIDL response.
// Wire format: int32 numFds, int32 numInts, then flat_binder_object
// entries for each FD, then int32 entries for each int.
func (r *ResponseParcel) ReadNativeHandle() (fds []int32, ints []int32, err error) {
	// HIDL serializes native_handle as:
	// bool isNull (written as int32 in some versions)
	// If not null:
	//   int32 numFds
	//   int32 numInts
	//   numFds x FD (as flat_binder_object)
	//   numInts x int32

	// TODO: implement based on actual wire format observed on device
	return nil, nil, fmt.Errorf("hwparcel: ReadNativeHandle not yet implemented")
}

// WriteFdArrayObject writes a binder_fd_array_object into the data buffer
// and records its offset in the objects array. This tells the kernel that
// the buffer at bufferHandle contains file descriptors starting at
// parentOffset (typically offsetof(native_handle_t, data) = 12).
//
// binder_fd_array_object layout (32 bytes):
//
//	[0:4]   uint32 type (BINDER_TYPE_FDA = 0x66646185)
//	[4:8]   uint32 pad
//	[8:16]  uint64 num_fds
//	[16:24] uint64 parent (buffer handle index)
//	[24:32] uint64 parent_offset (offset of FD array within parent buffer)
func (p *HwParcel) WriteFdArrayObject(
	numFds int,
	bufferHandle int,
	parentOffset uint64,
) {
	const binderTypeFDA = uint32(0x66646185)
	const fdArrayObjectSize = 32

	offset := uint64(len(p.data))
	p.objects = append(p.objects, offset)

	obj := make([]byte, fdArrayObjectSize)
	binary.LittleEndian.PutUint32(obj[0:], binderTypeFDA)
	// pad at offset 4 is zero
	binary.LittleEndian.PutUint64(obj[8:], uint64(numFds))
	binary.LittleEndian.PutUint64(obj[16:], uint64(bufferHandle))
	binary.LittleEndian.PutUint64(obj[24:], parentOffset)

	p.data = append(p.data, obj...)
}

// WriteNullNativeHandle writes the HIDL serialization for a null
// native_handle (hidl_handle with mHandle == nullptr).
//
// Wire format: uint64(0) inline.
func (p *HwParcel) WriteNullNativeHandle() {
	p.WriteUint64(0)
}

// WriteEmbeddedNativeHandle writes a non-null native_handle_t as an
// embedded child buffer, plus a binder_fd_array_object so the kernel
// can translate the file descriptors.
//
// Wire format:
//
//	uint64 native_handle_size (inline)
//	binder_buffer_object pointing to native_handle_t data (child of parent)
//	binder_fd_array_object referencing the FDs in the buffer
//
// The native_handle_t buffer contains:
//
//	int32 version (= 12 = sizeof(native_handle_t))
//	int32 numFds
//	int32 numInts
//	int32[numFds] fds
//	int32[numInts] ints
func (p *HwParcel) WriteEmbeddedNativeHandle(
	fds []int32,
	ints []int32,
	parentHandle int,
	parentOffset uint64,
) {
	const nativeHandleVersion = 12 // sizeof(native_handle_t)
	const nativeHandleDataOffset = 12

	nativeHandleSize := uint64(nativeHandleVersion + len(fds)*4 + len(ints)*4)

	// Write the size inline.
	p.WriteUint64(nativeHandleSize)

	// Build the native_handle_t buffer.
	buf := make([]byte, nativeHandleSize)
	binary.LittleEndian.PutUint32(buf[0:], uint32(nativeHandleVersion))
	binary.LittleEndian.PutUint32(buf[4:], uint32(len(fds)))
	binary.LittleEndian.PutUint32(buf[8:], uint32(len(ints)))
	off := 12
	for _, fd := range fds {
		binary.LittleEndian.PutUint32(buf[off:], uint32(fd))
		off += 4
	}
	for _, v := range ints {
		binary.LittleEndian.PutUint32(buf[off:], uint32(v))
		off += 4
	}

	// Write the buffer as an embedded child.
	bufHandle := p.WriteEmbeddedBuffer(buf, parentHandle, parentOffset)

	// Write the FD array object so the kernel translates the FDs.
	p.WriteFdArrayObject(len(fds), bufHandle, nativeHandleDataOffset)
}

// Verify compile-time size assertion for binder_buffer_object.
var _ = [binderBufferObjectSize]byte{}

// Verify hidl_string and hidl_vec sizes.
func init() {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		// The sizes above are for 64-bit only. On 32-bit they would be different.
		// This package only supports 64-bit Android.
		panic("hwparcel: only 64-bit platforms are supported")
	}
}
