package dex

import (
	"encoding/binary"
	"fmt"
)

// fieldID represents a parsed field_id_item.
type fieldID struct {
	classIdx uint16
	typeIdx  uint16
	nameIdx  uint32
}

// readFieldID parses the field_id_item at the given field_ids index.
func (f *dexFile) readFieldID(idx uint32) (fieldID, error) {
	if idx >= f.fieldIDsSize {
		return fieldID{}, fmt.Errorf("field index %d out of range (size=%d)", idx, f.fieldIDsSize)
	}

	off := uint64(f.fieldIDsOff) + uint64(idx)*fieldIDItemSize
	if off+fieldIDItemSize > uint64(len(f.data)) {
		return fieldID{}, fmt.Errorf("field_id_item at offset 0x%x out of bounds", off)
	}

	return fieldID{
		classIdx: binary.LittleEndian.Uint16(f.data[off:]),
		typeIdx:  binary.LittleEndian.Uint16(f.data[off+2:]),
		nameIdx:  binary.LittleEndian.Uint32(f.data[off+4:]),
	}, nil
}
