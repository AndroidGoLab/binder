package parcel

import (
	"fmt"
)

// Parcel is a container for serialized Binder IPC data.
// Data is 4-byte aligned, little-endian.
type Parcel struct {
	data    []byte
	pos     int      // current read position
	objects []uint64 // offsets of flat_binder_objects
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

// Data returns the underlying byte buffer.
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

// SetPosition sets the current read position.
func (p *Parcel) SetPosition(
	pos int,
) {
	p.pos = pos
}

// Recycle resets the parcel for reuse.
func (p *Parcel) Recycle() {
	p.data = p.data[:0]
	p.pos = 0
	p.objects = p.objects[:0]
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
		newData := make([]byte, needed, needed*2)
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
