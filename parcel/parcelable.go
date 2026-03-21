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

// WriteParcelableFooter patches the size at headerPos. The size value
// is the distance from headerPos to the current end of the parcel,
// which includes the 4-byte size field itself. This matches the AIDL
// NDK convention: writeToParcel writes size = end_pos - start_pos,
// and readFromParcel skips to start_pos + size.
func WriteParcelableFooter(
	p *Parcel,
	headerPos int,
) {
	size := p.Len() - headerPos
	binary.LittleEndian.PutUint32(p.data[headerPos:], uint32(size))
}

// ReadParcelableHeader reads the size of a parcelable payload and returns
// the end position (the byte offset where this parcelable's data ends).
// The size includes the 4-byte size field itself, so
// endPos = (position before size) + size.
//
// Increments the parcel's nesting depth counter and returns an error if
// the maximum depth (maxParcelableDepth) is exceeded. Call
// SkipToParcelableEnd (or the generated footer code) to decrement it.
func ReadParcelableHeader(
	p *Parcel,
) (int, error) {
	p.parcelableDepth++
	if p.parcelableDepth > maxParcelableDepth {
		depth := p.parcelableDepth // capture the depth that triggered the error
		p.parcelableDepth--
		return 0, fmt.Errorf("parcel: parcelable nesting depth %d exceeds maximum %d", depth, maxParcelableDepth)
	}

	startPos := p.Position()
	size, err := p.ReadInt32()
	if err != nil {
		p.parcelableDepth--
		return 0, fmt.Errorf("parcel: reading parcelable header: %w", err)
	}

	// A negative size typically means "null parcelable" (-1) when the
	// caller forgot to check the null indicator before UnmarshalParcel.
	// Treat it as zero-length (nothing to read) rather than erroring.
	if size <= 0 {
		return startPos + 4, nil
	}

	endPos := startPos + int(size)
	// Clamp to data length — the sender may encode a size larger than
	// what we received (e.g., newer API with more fields). Clamping
	// lets SkipToParcelableEnd safely advance to the end without
	// reading past the buffer.
	if endPos > p.Len() {
		endPos = p.Len()
	}

	return endPos, nil
}

// SkipToParcelableEnd sets the parcel position to endPos, allowing
// forward-compatible skipping of unknown fields. Decrements the
// parcelable nesting depth counter incremented by ReadParcelableHeader.
func SkipToParcelableEnd(
	p *Parcel,
	endPos int,
) {
	if p.parcelableDepth > 0 {
		p.parcelableDepth--
	}
	p.SetPosition(endPos)
}
