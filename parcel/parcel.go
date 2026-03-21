package parcel

import (
	"fmt"
)

// maxParcelableDepth limits how deeply nested parcelable types can be
// unmarshalled. Prevents stack overflow from malicious or cyclic data.
const maxParcelableDepth = 32

// Parcel is a container for serialized Binder IPC data.
// Data is 4-byte aligned, little-endian.
//
// Parcel is not safe for concurrent use. Callers must synchronize access externally.
type Parcel struct {
	data           []byte
	pos            int      // current read position
	objects        []uint64 // offsets of flat_binder_objects
	parcelableDepth int     // current nesting depth for parcelable unmarshal
}

// New creates a new empty Parcel.
func New() *Parcel {
	return &Parcel{}
}

// FromBytes creates a Parcel from existing serialized data.
func FromBytes(
	data []byte,
) *Parcel {
	return &Parcel{data: data}
}

// Data returns the internal byte slice. Callers must not modify it.
func (p *Parcel) Data() []byte {
	return p.data
}

// Objects returns the offsets of flat_binder_objects.
func (p *Parcel) Objects() []uint64 {
	return p.objects
}

// Len returns the length of the data buffer.
func (p *Parcel) Len() int {
	return len(p.data)
}

// Position returns the current read position.
func (p *Parcel) Position() int {
	return p.pos
}

// SetPosition sets the current read position, clamping to [0, len(data)].
// Non-aligned positions are allowed; read operations align internally.
func (p *Parcel) SetPosition(
	pos int,
) {
	switch {
	case pos < 0:
		p.pos = 0
	case pos > len(p.data):
		p.pos = len(p.data)
	default:
		p.pos = pos
	}
}

// Recycle resets the parcel for reuse.
// Allocates fresh slices instead of reusing the old backing arrays
// to prevent aliased mutation: callers that retained a reference to
// Data() or Objects() before Recycle must not observe writes from
// subsequent parcel operations. This trades pooling efficiency for
// correctness.
func (p *Parcel) Recycle() {
	p.data = nil
	p.pos = 0
	p.objects = nil
}

// grow ensures capacity and returns a slice of n bytes for writing.
// Padding bytes are zero-filled.
func (p *Parcel) grow(
	n int,
) []byte {
	aligned := (n + 3) &^ 3
	start := len(p.data)
	needed := start + aligned

	if cap(p.data) < needed {
		// Grow by 50% (not 100%) to reduce memory waste for large parcels.
		newData := make([]byte, needed, needed+needed/2)
		copy(newData, p.data)
		p.data = newData
	} else {
		p.data = p.data[:needed]
		// Zero-fill the newly exposed region (including padding).
		for i := start; i < needed; i++ {
			p.data[i] = 0
		}
	}

	return p.data[start : start+n]
}

// read returns n bytes from the current position, advancing it with 4-byte alignment.
func (p *Parcel) read(
	n int,
) ([]byte, error) {
	if n < 0 || p.pos < 0 {
		return nil, fmt.Errorf(
			"parcel: invalid read: n=%d, pos=%d", n, p.pos,
		)
	}
	aligned := (n + 3) &^ 3
	if p.pos+aligned > len(p.data) {
		return nil, fmt.Errorf(
			"parcel: read beyond end: need %d bytes at offset %d, have %d",
			aligned,
			p.pos,
			len(p.data),
		)
	}

	out := p.data[p.pos : p.pos+n]
	p.pos += aligned
	return out, nil
}
// test
// test comment
