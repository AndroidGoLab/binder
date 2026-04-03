package hidlcodec2

import "encoding/binary"

// BuildC2Param constructs a single C2 parameter blob.
//
// C2 param wire format:
//
//	[0:4] uint32 totalSize (= 8 + len(payload))
//	[4:8] uint32 paramIndex
//	[8:]  payload bytes
func BuildC2Param(
	index uint32,
	payload []byte,
) []byte {
	totalSize := 8 + uint32(len(payload))
	buf := make([]byte, totalSize)
	binary.LittleEndian.PutUint32(buf[0:], totalSize)
	binary.LittleEndian.PutUint32(buf[4:], index)
	copy(buf[8:], payload)
	return buf
}

// BuildPictureSizeParam builds a C2StreamPictureSizeInfo parameter.
//
// C2StreamPictureSizeInfo::PARAM_TYPE = 0x4B400000 | (stream << 17).
// Payload: uint32 width, uint32 height.
func BuildPictureSizeParam(
	stream uint32,
	width uint32,
	height uint32,
) []byte {
	index := uint32(0x4B400000) | (stream << 17)
	payload := make([]byte, 8)
	binary.LittleEndian.PutUint32(payload[0:], width)
	binary.LittleEndian.PutUint32(payload[4:], height)
	return BuildC2Param(index, payload)
}

// BuildBitrateParam builds a C2StreamBitrateInfo parameter.
//
// C2StreamBitrateInfo::PARAM_TYPE = 0x4B200000 | (stream << 17).
// Payload: uint32 bitrate.
func BuildBitrateParam(
	stream uint32,
	bitrate uint32,
) []byte {
	index := uint32(0x4B200000) | (stream << 17)
	payload := make([]byte, 4)
	binary.LittleEndian.PutUint32(payload[0:], bitrate)
	return BuildC2Param(index, payload)
}

// BuildRangeInfoParam builds a C2Hal_RangeInfo parameter, which is
// required as the Block.Meta for linear blocks.
//
// C2Hal_RangeInfo is C2GlobalParam<C2Info, C2Hal_Range, 0>.
// Index = KIND_INFO(0xC0000000) | DIR_GLOBAL(0x20000000) | 0 = 0xE0000000.
// Payload: uint32 offset, uint32 length.
func BuildRangeInfoParam(offset uint32, length uint32) []byte {
	const rangeInfoIndex = uint32(0xE0000000)
	payload := make([]byte, 8)
	binary.LittleEndian.PutUint32(payload[0:], offset)
	binary.LittleEndian.PutUint32(payload[4:], length)
	return BuildC2Param(rangeInfoIndex, payload)
}

// C2HandleIonMagic is the magic value for C2HandleIon / C2HandleBuf.
// Both Ion and DmaBuf allocators use this same magic to identify
// linear block handles.
//
// Computed from the C++ multi-char literal '\xc2io\x00'.
const C2HandleIonMagic = int32(-1033277696) // 0xc2696f00

// C2HandleLinearInts builds the ints array for a C2HandleIon /
// C2HandleBuf native_handle_t. This handle format is recognized by
// the Codec2 framework as a linear block.
//
// The native_handle_t should have numFds=1 (the buffer fd) and
// numInts=3 (sizeLo, sizeHi, magic).
func C2HandleLinearInts(size uint64) []int32 {
	return []int32{
		int32(size & 0xFFFFFFFF),         // sizeLo
		int32((size >> 32) & 0xFFFFFFFF), // sizeHi
		C2HandleIonMagic,                 // magic
	}
}

// ConcatParams concatenates multiple C2 param blobs into a single
// byte slice, with 8-byte alignment padding between params as required
// by the Codec2 wire format.
func ConcatParams(params ...[]byte) []byte {
	var total int
	for _, p := range params {
		total += len(p)
		// Add padding to 8-byte alignment.
		if pad := len(p) % 8; pad != 0 {
			total += 8 - pad
		}
	}
	result := make([]byte, 0, total)
	for _, p := range params {
		result = append(result, p...)
		// Pad to 8-byte alignment.
		if pad := len(p) % 8; pad != 0 {
			result = append(result, make([]byte, 8-pad)...)
		}
	}
	return result
}
