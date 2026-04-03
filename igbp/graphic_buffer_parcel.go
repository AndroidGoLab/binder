package igbp

import (
	"encoding/binary"

	"github.com/AndroidGoLab/binder/gralloc"
	"github.com/AndroidGoLab/binder/parcel"
)

// WriteGrallocGraphicBuffer writes a flattened GraphicBuffer backed by a
// gralloc NativeHandle. The wire format follows GraphicBuffer::flatten():
//
//	int32(flattenedSize) + int32(fdCount) + raw[flattenedSize] + fd objects
func WriteGrallocGraphicBuffer(
	p *parcel.Parcel,
	buf *gralloc.Buffer,
	bufID uint64,
) {
	numFds := int32(len(buf.Handle.Fds))
	numInts := int32(len(buf.Handle.Ints))

	// Flattened size: 13 int32s (header) + numInts int32s.
	flattenedSize := (13 + numInts) * 4

	p.WriteInt32(flattenedSize)
	p.WriteInt32(numFds)

	raw := make([]byte, flattenedSize)
	binary.LittleEndian.PutUint32(raw[0:], uint32(GraphicBufferMagicGB01))
	binary.LittleEndian.PutUint32(raw[4:], buf.Width)
	binary.LittleEndian.PutUint32(raw[8:], buf.Height)
	binary.LittleEndian.PutUint32(raw[12:], uint32(buf.Stride))
	binary.LittleEndian.PutUint32(raw[16:], uint32(buf.Format))
	binary.LittleEndian.PutUint32(raw[20:], 1)                        // layerCount
	binary.LittleEndian.PutUint32(raw[24:], uint32(buf.Usage))        // usage low
	binary.LittleEndian.PutUint32(raw[28:], uint32(bufID>>32))        // id high
	binary.LittleEndian.PutUint32(raw[32:], uint32(bufID&0xFFFFFFFF)) // id low
	binary.LittleEndian.PutUint32(raw[36:], 0)                        // generationNumber
	binary.LittleEndian.PutUint32(raw[40:], uint32(numFds))           // numFds
	binary.LittleEndian.PutUint32(raw[44:], uint32(numInts))          // numInts
	binary.LittleEndian.PutUint32(raw[48:], uint32(buf.Usage>>32))    // usage high

	// Append native_handle ints after the 13-word header.
	for i, v := range buf.Handle.Ints {
		binary.LittleEndian.PutUint32(raw[52+i*4:], uint32(v))
	}

	p.WriteRawBytes(raw)

	// Write each FD as a flat_binder_object.
	for _, fd := range buf.Handle.Fds {
		p.WriteFileDescriptor(fd)
	}
}
