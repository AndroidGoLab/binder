package hidlcodec2

import (
	"encoding/binary"

	"github.com/AndroidGoLab/binder/hwparcel"
)

// WorkOrdinal describes the ordering of a frame.
type WorkOrdinal struct {
	TimestampUs   uint64
	FrameIndex    uint64
	CustomOrdinal uint64
}

// FrameDataFlags are bitfield flags for FrameData.
type FrameDataFlags uint32

const (
	FrameDataDropFrame    FrameDataFlags = 1 << 0
	FrameDataEndOfStream  FrameDataFlags = 1 << 1
	FrameDataDiscardFrame FrameDataFlags = 1 << 2
	FrameDataIncomplete   FrameDataFlags = 1 << 3
	FrameDataCodecConfig  FrameDataFlags = 1 << 31
)

// BaseBlock represents a memory block. For our use case we only support
// nativeBlock (a native_handle with FD + size).
type BaseBlock struct {
	// Tag selects which union variant is active.
	// 0 = nativeBlock (handle), 1 = pooledBlock.
	Tag uint32

	// NativeBlock is used when Tag == 0.
	NativeBlockFds  []int32
	NativeBlockInts []int32
}

// Block references a BaseBlock within a WorkBundle.
type Block struct {
	Index    uint32
	Meta     []byte  // Params: concatenated C2Param blobs
	FenceFds []int32 // Fence native_handle FDs (nil for null fence)
	FenceInts []int32 // Fence native_handle ints (nil for null fence)
}

// Buffer is a collection of Blocks with metadata.
type Buffer struct {
	Info   []byte // Params
	Blocks []Block
}

// FrameData carries input or output frame data.
type FrameData struct {
	Flags        FrameDataFlags
	Ordinal      WorkOrdinal
	Buffers      []Buffer
	ConfigUpdate []byte // Params
	// InfoBuffers omitted for encoder input.
}

// Worklet describes processing for one output frame.
type Worklet struct {
	ComponentId uint32
	Tunings     []byte // Params
	// Failures and Output are omitted for input worklets.
}

// Work represents a single work item.
type Work struct {
	ChainInfo         []byte // Params
	Input             FrameData
	Worklets          []Worklet
	WorkletsProcessed uint32
	Result            Status
}

// WorkBundle bundles works and base blocks for IPC efficiency.
type WorkBundle struct {
	Works      []Work
	BaseBlocks []BaseBlock
}

// WriteTo serializes the WorkBundle into the HwParcel using HIDL
// scatter-gather format with proper parent-child buffer relationships.
func (wb *WorkBundle) WriteTo(hp *hwparcel.HwParcel) {
	writeWorkBundleToParcel(hp, wb)
}

// Inline struct sizes in the HIDL wire format.
// These are the sizes of the fixed headers that are embedded in their
// parent arrays, with dynamic data (strings, vecs, handles) stored in
// separate child buffer objects.
const (
	hidlVecHeaderSize    = 16 // ptr(8) + size(4) + owns(1) + pad(3)
	hidlHandleHeaderSize = 16 // ptr(8) + pad(8) -- hidl_handle is just a pointer
)

// writeWorkBundleToParcel performs the full HIDL scatter-gather
// serialization of a WorkBundle.
//
// WorkBundle layout:
//
//	[0:16]  hidl_vec<Work> works
//	[16:32] hidl_vec<BaseBlock> baseBlocks
//
// Total: 32 bytes for the top-level struct.
func writeWorkBundleToParcel(hp *hwparcel.HwParcel, wb *WorkBundle) {
	// Top-level struct: two hidl_vec headers.
	topBuf := make([]byte, 32)
	binary.LittleEndian.PutUint32(topBuf[8:], uint32(len(wb.Works)))
	binary.LittleEndian.PutUint32(topBuf[24:], uint32(len(wb.BaseBlocks)))
	topHandle := hp.WriteBuffer(topBuf)

	// Works array data (child of top at offset 0 = works.ptr).
	writeWorksVec(hp, topHandle, 0, wb.Works)

	// BaseBlocks array data (child of top at offset 16 = baseBlocks.ptr).
	writeBaseBlocksVec(hp, topHandle, 16, wb.BaseBlocks)
}

// writeWorksVec writes the hidl_vec<Work> data as a child buffer.
//
// Each Work struct inline layout (120 bytes):
//
//	[0:16]   hidl_vec<uint8_t> chainInfo
//	[16:96]  FrameData input (80 bytes)
//	[96:112] hidl_vec<Worklet> worklets
//	[112:116] uint32 workletsProcessed
//	[116:120] int32 result (Status)
//
// FrameData inline layout (80 bytes):
//
//	[0:4]   uint32 flags
//	[4:8]   padding
//	[8:32]  WorkOrdinal (3 x uint64)
//	[32:48] hidl_vec<Buffer> buffers
//	[48:64] hidl_vec<uint8_t> configUpdate
//	[64:80] hidl_vec<InfoBuffer> infoBuffers
const workInlineSize = 120

func writeWorksVec(
	hp *hwparcel.HwParcel,
	parentHandle int,
	parentOffset uint64,
	works []Work,
) {
	if len(works) == 0 {
		hp.WriteEmbeddedBuffer([]byte{}, parentHandle, parentOffset)
		return
	}

	// Build the flattened Work array.
	worksBuf := make([]byte, workInlineSize*len(works))
	for i, w := range works {
		base := i * workInlineSize

		// chainInfo vec header at [0:16]
		binary.LittleEndian.PutUint32(worksBuf[base+8:], uint32(len(w.ChainInfo)))

		// FrameData at [16:96]
		fd := base + 16
		binary.LittleEndian.PutUint32(worksBuf[fd:], uint32(w.Input.Flags))
		binary.LittleEndian.PutUint64(worksBuf[fd+8:], w.Input.Ordinal.TimestampUs)
		binary.LittleEndian.PutUint64(worksBuf[fd+16:], w.Input.Ordinal.FrameIndex)
		binary.LittleEndian.PutUint64(worksBuf[fd+24:], w.Input.Ordinal.CustomOrdinal)
		binary.LittleEndian.PutUint32(worksBuf[fd+40:], uint32(len(w.Input.Buffers)))
		binary.LittleEndian.PutUint32(worksBuf[fd+56:], uint32(len(w.Input.ConfigUpdate)))
		// infoBuffers at fd+64 = 0 (empty)

		// worklets vec at [96:112]
		binary.LittleEndian.PutUint32(worksBuf[base+104:], uint32(len(w.Worklets)))

		// workletsProcessed at [112:116]
		binary.LittleEndian.PutUint32(worksBuf[base+112:], w.WorkletsProcessed)

		// result at [116:120]
		binary.LittleEndian.PutUint32(worksBuf[base+116:], uint32(w.Result))
	}

	worksHandle := hp.WriteEmbeddedBuffer(worksBuf, parentHandle, parentOffset)

	// Now write child buffers for each Work element.
	for i, w := range works {
		baseOffset := uint64(i * workInlineSize)

		// chainInfo data (child of works array, at offset base+0 = chainInfo.ptr).
		writeVecDataChild(hp, worksHandle, baseOffset+0, w.ChainInfo)

		// FrameData children at base+16.
		writeFrameDataChildren(hp, worksHandle, baseOffset+16, &w.Input)

		// worklets data (child of works array, at offset base+96 = worklets.ptr).
		writeWorkletsVec(hp, worksHandle, baseOffset+96, w.Worklets)
	}
}

// writeFrameDataChildren writes children for a FrameData struct embedded
// at the given offset within its parent buffer.
//
// FrameData children:
//   - buffers vec data at parentOffset+32
//   - configUpdate vec data at parentOffset+48
//   - infoBuffers vec data at parentOffset+64
func writeFrameDataChildren(
	hp *hwparcel.HwParcel,
	parentHandle int,
	parentOffset uint64,
	fd *FrameData,
) {
	// buffers vec data.
	writeBuffersVec(hp, parentHandle, parentOffset+32, fd.Buffers)

	// configUpdate data.
	writeVecDataChild(hp, parentHandle, parentOffset+48, fd.ConfigUpdate)

	// infoBuffers (empty).
	hp.WriteEmbeddedBuffer([]byte{}, parentHandle, parentOffset+64)
}

// writeVecDataChild writes a hidl_vec<uint8_t>'s data as a child buffer.
// parentOffset is the offset of the hidl_vec header's pointer field within
// the parent buffer.
// writeVecDataChild writes a hidl_vec's data as a child buffer.
// Even for empty data, a zero-length child buffer object is written
// so the server's sequential buffer-object reader stays in sync.
func writeVecDataChild(
	hp *hwparcel.HwParcel,
	parentHandle int,
	parentOffset uint64,
	data []byte,
) {
	if len(data) == 0 {
		hp.WriteEmbeddedBuffer([]byte{}, parentHandle, parentOffset)
		return
	}
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)
	hp.WriteEmbeddedBuffer(dataCopy, parentHandle, parentOffset)
}

// writeBuffersVec writes the hidl_vec<Buffer> data and its children.
//
// Each Buffer inline (32 bytes):
//
//	[0:16]  hidl_vec<uint8_t> info
//	[16:32] hidl_vec<Block> blocks
const bufferInlineSize = 32

func writeBuffersVec(
	hp *hwparcel.HwParcel,
	parentHandle int,
	parentOffset uint64,
	buffers []Buffer,
) {
	if len(buffers) == 0 {
		hp.WriteEmbeddedBuffer([]byte{}, parentHandle, parentOffset)
		return
	}

	bufsBuf := make([]byte, bufferInlineSize*len(buffers))
	for i, b := range buffers {
		base := i * bufferInlineSize
		binary.LittleEndian.PutUint32(bufsBuf[base+8:], uint32(len(b.Info)))
		binary.LittleEndian.PutUint32(bufsBuf[base+24:], uint32(len(b.Blocks)))
	}

	bufsHandle := hp.WriteEmbeddedBuffer(bufsBuf, parentHandle, parentOffset)

	for i, b := range buffers {
		baseOff := uint64(i * bufferInlineSize)

		// info data.
		writeVecDataChild(hp, bufsHandle, baseOff+0, b.Info)

		// blocks data.
		writeBlocksVec(hp, bufsHandle, baseOff+16, b.Blocks)
	}
}

// writeBlocksVec writes the hidl_vec<Block> data and its children.
//
// Each Block inline (40 bytes):
//
//	[0:4]   uint32 index
//	[4:8]   padding
//	[8:24]  hidl_vec<uint8_t> meta (Params)
//	[24:40] hidl_handle fence
const blockInlineSize = 40

func writeBlocksVec(
	hp *hwparcel.HwParcel,
	parentHandle int,
	parentOffset uint64,
	blocks []Block,
) {
	if len(blocks) == 0 {
		hp.WriteEmbeddedBuffer([]byte{}, parentHandle, parentOffset)
		return
	}

	blkBuf := make([]byte, blockInlineSize*len(blocks))
	for i, blk := range blocks {
		base := i * blockInlineSize
		binary.LittleEndian.PutUint32(blkBuf[base:], blk.Index)
		binary.LittleEndian.PutUint32(blkBuf[base+16:], uint32(len(blk.Meta)))
		// fence at base+24 is all zeros (null handle).
	}

	blkHandle := hp.WriteEmbeddedBuffer(blkBuf, parentHandle, parentOffset)

	for i, blk := range blocks {
		baseOff := uint64(i * blockInlineSize)

		// meta data.
		writeVecDataChild(hp, blkHandle, baseOff+8, blk.Meta)

		// fence: hidl_handle serialization. The fence is at offset 24
		// within each Block struct. The hidl_handle pointer is at
		// offset 0 of the hidl_handle struct (= baseOff+24).
		//
		// For null handles: writeNullNativeHandle writes uint64(0).
		// For non-null handles: writeEmbeddedNativeHandle writes
		// uint64(size) + PTR buffer + FDA object.
		if len(blk.FenceFds) > 0 || len(blk.FenceInts) > 0 {
			hp.WriteEmbeddedNativeHandle(
				blk.FenceFds, blk.FenceInts,
				blkHandle, baseOff+24,
			)
		} else {
			hp.WriteNullNativeHandle()
		}
	}
}

// writeWorkletsVec writes the hidl_vec<Worklet> data and its children.
//
// Each Worklet inline (120 bytes):
//
//	[0:4]    uint32 componentId
//	[4:8]    padding
//	[8:24]   hidl_vec<uint8_t> tunings (Params)
//	[24:40]  hidl_vec<SettingResult> failures
//	[40:120] FrameData output (80 bytes)
const workletInlineSize = 120

func writeWorkletsVec(
	hp *hwparcel.HwParcel,
	parentHandle int,
	parentOffset uint64,
	worklets []Worklet,
) {
	if len(worklets) == 0 {
		return
	}

	wlBuf := make([]byte, workletInlineSize*len(worklets))
	for i, wl := range worklets {
		base := i * workletInlineSize
		binary.LittleEndian.PutUint32(wlBuf[base:], wl.ComponentId)
		binary.LittleEndian.PutUint32(wlBuf[base+16:], uint32(len(wl.Tunings)))
		// failures at base+24 = 0 (empty)
		// output FrameData at base+40 = all zeros (empty)
	}

	wlHandle := hp.WriteEmbeddedBuffer(wlBuf, parentHandle, parentOffset)

	for i, wl := range worklets {
		baseOff := uint64(i * workletInlineSize)

		// tunings data.
		writeVecDataChild(hp, wlHandle, baseOff+8, wl.Tunings)

		// failures (empty).
		hp.WriteEmbeddedBuffer([]byte{}, wlHandle, baseOff+24)

		// output FrameData children.
		writeFrameDataChildren(hp, wlHandle, baseOff+40, &FrameData{})
	}
}

// writeBaseBlocksVec writes the hidl_vec<BaseBlock> data.
//
// BaseBlock is a safe_union (hidl_union):
//
//	[0:8]   hidl_discriminator (uint64 for 8-byte aligned safe_union)
//	[8:...]  union data
//
// For nativeBlock (tag=0): union data is hidl_handle (16 bytes)
// For pooledBlock (tag=1): union data is BufferStatusMessage (40 bytes)
//
// The union size is max(hidl_handle=16, BufferStatusMessage=40) = 40.
// Total BaseBlock = 8 (discriminator) + 40 (union) = 48 bytes.
const baseBlockInlineSize = 48

func writeBaseBlocksVec(
	hp *hwparcel.HwParcel,
	parentHandle int,
	parentOffset uint64,
	blocks []BaseBlock,
) {
	if len(blocks) == 0 {
		hp.WriteEmbeddedBuffer([]byte{}, parentHandle, parentOffset)
		return
	}

	bbBuf := make([]byte, baseBlockInlineSize*len(blocks))
	for i, bb := range blocks {
		base := i * baseBlockInlineSize
		// hidl_discriminator as uint64.
		binary.LittleEndian.PutUint64(bbBuf[base:], uint64(bb.Tag))
		// Union data at base+8. For nativeBlock, hidl_handle is ptr(8)+pad(8)=16 bytes.
		// The pointer will be patched by the kernel.
	}

	bbHandle := hp.WriteEmbeddedBuffer(bbBuf, parentHandle, parentOffset)

	for i, bb := range blocks {
		baseOff := uint64(i * baseBlockInlineSize)

		switch bb.Tag {
		case 0: // nativeBlock (hidl_handle)
			// hidl_handle serialization. The hidl_handle pointer is
			// at baseOff+8 within the BaseBlock struct.
			//
			// HIDL native_handle serialization requires:
			// 1. uint64(size) inline (or uint64(0) if null)
			// 2. binder_buffer_object for native_handle_t data
			// 3. binder_fd_array_object for FD translation
			if len(bb.NativeBlockFds) > 0 || len(bb.NativeBlockInts) > 0 {
				hp.WriteEmbeddedNativeHandle(
					bb.NativeBlockFds, bb.NativeBlockInts,
					bbHandle, baseOff+8,
				)
			} else {
				hp.WriteNullNativeHandle()
			}
		case 1: // pooledBlock
			// Not implemented; null pointer, no child.
		}
	}
}

