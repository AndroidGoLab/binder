package parcel

import (
	"encoding/binary"
	"fmt"
)

// Parcelable is the interface for types that can be serialized to/from a Parcel.
type Parcelable interface {
	MarshalParcel(p *Parcel) error
	UnmarshalParcel(p *Parcel) error
}

// WriteParcelableHeader writes a placeholder int32 for the total size
// of a parcelable payload. Returns the position of the placeholder,
// which must be passed to WriteParcelableFooter after writing the payload.
func WriteParcelableHeader(
	p *Parcel,
) int {
	headerPos := p.Len()
	p.WriteInt32(0) // placeholder for size
	return headerPos
}

// WriteParcelableFooter patches the size at headerPos with the actual
// number of bytes written since the header.
func WriteParcelableFooter(
	p *Parcel,
	headerPos int,
) {
	size := p.Len() - headerPos - 4
	binary.LittleEndian.PutUint32(p.data[headerPos:], uint32(size))
}

// ReadParcelableHeader reads the size of a parcelable payload and returns
// the end position (the byte offset where this parcelable's data ends).
func ReadParcelableHeader(
	p *Parcel,
) (int, error) {
	size, err := p.ReadInt32()
	if err != nil {
		return 0, fmt.Errorf("parcel: reading parcelable header: %w", err)
	}

	endPos := p.Position() + int(size)
	return endPos, nil
}

// SkipToParcelableEnd sets the parcel position to endPos, allowing
// forward-compatible skipping of unknown fields.
func SkipToParcelableEnd(
	p *Parcel,
	endPos int,
) {
	p.SetPosition(endPos)
}
