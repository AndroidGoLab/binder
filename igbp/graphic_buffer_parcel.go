package igbp

import (
	"encoding/binary"
	"errors"
	"os"
	"sync/atomic"

	"github.com/AndroidGoLab/binder/gralloc"
	"github.com/AndroidGoLab/binder/parcel"
)

// graphicBufferIDGenerator allocates IDs without reusing an exhausted counter.
//
// Example: each flattened wrapper receives its own ID.
type graphicBufferIDGenerator struct{ counter atomic.Uint32 }

// graphicBufferIDs owns the process-wide synthetic ID sequence.
//
// Example: separate producers allocate from the same sequence.
var graphicBufferIDs graphicBufferIDGenerator

// next returns a synthetic ID unique within this process lifetime or an exhaustion error.
// Bit 63 separates it from native GraphicBuffer IDs made with positive PIDs.
//
// Example: a producer calls next once for each slot wrapper.
func (g *graphicBufferIDGenerator) next() (uint64, error) {
	pid := os.Getpid()
	if pid <= 0 || uint64(pid) > 0x7fffffff {
		return 0, errors.New("invalid process ID for GraphicBuffer ID")
	}
	for {
		current := g.counter.Load()
		if current == ^uint32(0) {
			return 0, errors.New("GraphicBuffer ID space exhausted")
		}
		if g.counter.CompareAndSwap(current, current+1) {
			return uint64(1)<<63 | uint64(pid)<<32 | uint64(current+1), nil
		}
	}
}

// WriteGrallocGraphicBuffer writes a flattened GraphicBuffer backed by a
// gralloc NativeHandle. The wire format follows GraphicBuffer::flatten():
//
//	int32(flattenedSize) + int32(fdCount) + raw[flattenedSize] + fd objects
func WriteGrallocGraphicBuffer(
	p *parcel.Parcel,
	buf *gralloc.Buffer,
	bufID uint64,
) {
	if raw, ok := buf.FlattenedGraphicBuffer(); ok {
		writeFlattenedGraphicBuffer(p, raw, buf.Handle.Fds)
		return
	}
	numFds := int32(len(buf.Handle.Fds))
	numInts := int32(len(buf.Handle.Ints))

	// Flattened size: 13 int32s (header) + numInts int32s.
	flattenedSize := (13 + numInts) * 4

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

	writeFlattenedGraphicBuffer(p, raw, buf.Handle.Fds)
}

// writeFlattenedGraphicBuffer preserves the flattened bytes and descriptor order.
//
// Example: send an allocator's native GraphicBuffer metadata unchanged.
func writeFlattenedGraphicBuffer(p *parcel.Parcel, raw []byte, fds []int32) {
	p.WriteInt32(int32(len(raw)))
	p.WriteInt32(int32(len(fds)))
	p.WriteRawBytes(raw)

	// Write each FD as a flat_binder_object.
	for _, fd := range fds {
		p.WriteFileDescriptor(fd)
	}
}
