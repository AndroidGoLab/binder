package igbp

import (
	"encoding/binary"

	"github.com/xaionaro-go/binder/parcel"
)

// queueBufferOutputPayload is a pre-computed 61-byte QueueBufferOutput
// flattenable payload. Allocated once to avoid per-frame allocation.
var queueBufferOutputPayload = func() []byte {
	const flatSize = 61
	buf := make([]byte, flatSize)
	off := 0
	binary.LittleEndian.PutUint32(buf[off:], 0) // width
	off += 4
	binary.LittleEndian.PutUint32(buf[off:], 0) // height
	off += 4
	binary.LittleEndian.PutUint32(buf[off:], 0) // transformHint
	off += 4
	binary.LittleEndian.PutUint32(buf[off:], 0) // numPendingBuffers
	off += 4
	binary.LittleEndian.PutUint64(buf[off:], 1) // nextFrameNumber
	off += 8
	buf[off] = 0 // bufferReplaced
	off += 1
	binary.LittleEndian.PutUint32(buf[off:], 64) // maxBufferCount
	off += 4
	off += 24                                   // compositorTiming zeros
	binary.LittleEndian.PutUint32(buf[off:], 0) // deltaCount
	off += 4
	binary.LittleEndian.PutUint32(buf[off:], 0) // result
	_ = off
	return buf
}()

// WriteQueueBufferOutput writes a minimal QueueBufferOutput Flattenable.
func WriteQueueBufferOutput(reply *parcel.Parcel) {
	reply.WriteInt32(int32(len(queueBufferOutputPayload)))
	reply.WriteInt32(0) // fdCount
	reply.WriteRawBytes(queueBufferOutputPayload)
}

// WriteEmptyFrameEventHistoryDelta writes an empty FrameEventHistoryDelta
// as a Flattenable: compositorTiming (3 * int64) + int32(0 deltas) = 28 bytes.
func WriteEmptyFrameEventHistoryDelta(reply *parcel.Parcel) {
	reply.WriteInt32(28) // flattenedSize
	reply.WriteInt32(0)  // fdCount
	reply.WriteInt64(0)  // compositor deadline
	reply.WriteInt64(0)  // compositor interval
	reply.WriteInt64(0)  // compositor presentLatency
	reply.WriteInt32(0)  // delta count
}
