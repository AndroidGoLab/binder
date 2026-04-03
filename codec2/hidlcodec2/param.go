package hidlcodec2

import "encoding/binary"

// C2Param index bit layout (from AOSP C2Param.h):
//
//	bits 31-30: Kind     (INFO=0xC0, SETTING=0x80, TUNING=0x40)
//	bits 29-28: Direction (INPUT=0x00, OUTPUT=0x10, GLOBAL=0x20)
//	bit  25:    IS_STREAM_FLAG
//	bits 24-20: Stream ID (5 bits)
//	bit  16:    IS_FLEX_FLAG
//	bit  15:    IS_VENDOR_FLAG
//	bits 15-0:  Type index
const (
	paramKindInfo    = uint32(0xC0000000)
	paramKindSetting = uint32(0x80000000)
	paramKindTuning  = uint32(0x40000000)

	paramDirInput  = uint32(0x00000000)
	paramDirOutput = uint32(0x10000000)
	paramDirGlobal = uint32(0x20000000)

	paramIsStreamFlag = uint32(0x02000000)
	paramStreamShift  = 20

	// Core param indices from AOSP C2Config.h.
	paramIndexPictureSize = uint32(0x1800) // kParamIndexPictureSize
	paramIndexBitrate     = uint32(0x1000) // kParamIndexBitrate

	// Audio param indices from AOSP C2Config.h.
	// C2_PARAM_INDEX_AUDIO_PARAM_START = 0x3000.
	paramIndexSampleRate  = uint32(0x3000) // kParamIndexSampleRate
	paramIndexChannelCount = uint32(0x3001) // kParamIndexChannelCount
)

// c2StreamParamIndex computes the full C2Param index for a stream
// parameter from its kind, direction, stream ID, and core index.
func c2StreamParamIndex(
	kind uint32,
	dir uint32,
	stream uint32,
	coreIndex uint32,
) uint32 {
	return kind | dir | paramIsStreamFlag | (stream << paramStreamShift) | coreIndex
}

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

// BuildPictureSizeParam builds a C2StreamPictureSizeInfo::input parameter.
//
// The AVC encoder declares picture size as an input-direction stream
// parameter (C2StreamPictureSizeInfo::input). The full index is:
// KIND_INFO | DIR_INPUT | IS_STREAM | (stream << 20) | 0x1800.
// Payload: uint32 width, uint32 height.
func BuildPictureSizeParam(
	stream uint32,
	width uint32,
	height uint32,
) []byte {
	index := c2StreamParamIndex(paramKindInfo, paramDirInput, stream, paramIndexPictureSize)
	payload := make([]byte, 8)
	binary.LittleEndian.PutUint32(payload[0:], width)
	binary.LittleEndian.PutUint32(payload[4:], height)
	return BuildC2Param(index, payload)
}

// BuildBitrateParam builds a C2StreamBitrateInfo::output parameter.
//
// The AVC encoder declares bitrate as an output-direction stream
// parameter (C2StreamBitrateInfo::output). The full index is:
// KIND_INFO | DIR_OUTPUT | IS_STREAM | (stream << 20) | 0x1000.
// Payload: uint32 bitrate.
func BuildBitrateParam(
	stream uint32,
	bitrate uint32,
) []byte {
	index := c2StreamParamIndex(paramKindInfo, paramDirOutput, stream, paramIndexBitrate)
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

// BuildSampleRateParam builds a C2StreamSampleRateInfo::input parameter.
//
// The AAC encoder declares sample rate as an input-direction stream
// parameter. The full index is:
// KIND_INFO | DIR_INPUT | IS_STREAM | (stream << 20) | 0x3000.
// Payload: uint32 sampleRate.
func BuildSampleRateParam(
	stream uint32,
	sampleRate uint32,
) []byte {
	index := c2StreamParamIndex(paramKindInfo, paramDirInput, stream, paramIndexSampleRate)
	payload := make([]byte, 4)
	binary.LittleEndian.PutUint32(payload[0:], sampleRate)
	return BuildC2Param(index, payload)
}

// BuildChannelCountParam builds a C2StreamChannelCountInfo::input parameter.
//
// The AAC encoder declares channel count as an input-direction stream
// parameter. The full index is:
// KIND_INFO | DIR_INPUT | IS_STREAM | (stream << 20) | 0x3001.
// Payload: uint32 channelCount.
func BuildChannelCountParam(
	stream uint32,
	channelCount uint32,
) []byte {
	index := c2StreamParamIndex(paramKindInfo, paramDirInput, stream, paramIndexChannelCount)
	payload := make([]byte, 4)
	binary.LittleEndian.PutUint32(payload[0:], channelCount)
	return BuildC2Param(index, payload)
}

// C2HandleGrallocMagic is the magic value for C2HandleGralloc.
// Computed from the C++ multi-char literal '\xc2gr\x00':
// bytes LE: 0xC2, 'g'(0x67), 'r'(0x72), 0x00 -> uint32 0x007267C2.
const C2HandleGrallocMagic = int32(0x007267C2)

// C2HandleGrallocExtraInts is the number of extra int32 values appended
// by C2HandleGralloc::WrapNativeHandle to the gralloc native_handle.
const C2HandleGrallocExtraInts = 11

// C2HandleGrallocInts builds the extra ints that C2HandleGralloc appends
// to a gralloc native_handle_t. The Codec2 framework recognizes handles
// with this trailing ExtraData as graphic blocks.
//
// ExtraData layout (11 x int32):
//
//	[0] width
//	[1] height
//	[2] format (PixelFormat)
//	[3] usage_lo (low 32 bits of BufferUsage)
//	[4] usage_hi (high 32 bits of BufferUsage)
//	[5] stride
//	[6] generation
//	[7] igbp_id_lo
//	[8] igbp_id_hi
//	[9] igbp_slot
//	[10] magic (C2HandleGrallocMagic)
func C2HandleGrallocInts(
	width uint32,
	height uint32,
	format uint32,
	usage uint64,
	stride uint32,
) []int32 {
	return []int32{
		int32(width),
		int32(height),
		int32(format),
		int32(usage & 0xFFFFFFFF),
		int32(usage >> 32),
		int32(stride),
		0, // generation
		0, // igbp_id_lo
		0, // igbp_id_hi
		0, // igbp_slot
		C2HandleGrallocMagic,
	}
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
