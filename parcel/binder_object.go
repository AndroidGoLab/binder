package parcel

import (
	"encoding/binary"
	"fmt"
)

const (
	// binderTypeBinder is the type for a local Binder object.
	// Kernel value: B_PACK_CHARS('s','b','*',0x85) = 0x73622a85.
	binderTypeBinder = uint32(0x73622a85)

	// binderTypeHandle is the type for a Binder handle reference.
	// Kernel value: B_PACK_CHARS('s','h','*',0x85) = 0x73682a85.
	binderTypeHandle = uint32(0x73682a85)

	// binderFlagsAcceptFDs combines the default priority mask (0x7f) with
	// FLAT_BINDER_FLAG_ACCEPTS_FDS (0x100).
	binderFlagsAcceptFDs = uint32(0x7f | 0x100)

	// flatBinderObjectSize is the size of a flat_binder_object (24 bytes on 64-bit).
	flatBinderObjectSize = 24
)

// WriteStrongBinder writes a flat_binder_object with the given handle.
// Records the offset in the parcel's objects array.
func (p *Parcel) WriteStrongBinder(
	handle uint32,
) {
	offset := uint64(p.Len())
	p.objects = append(p.objects, offset)

	buf := p.grow(flatBinderObjectSize)

	// type (uint32, offset 0)
	binary.LittleEndian.PutUint32(buf[0:], binderTypeHandle)

	// flags (uint32, offset 4)
	binary.LittleEndian.PutUint32(buf[4:], binderFlagsAcceptFDs)

	// handle (uint32, offset 8)
	binary.LittleEndian.PutUint32(buf[8:], handle)

	// pad (uint32, offset 12)
	binary.LittleEndian.PutUint32(buf[12:], 0)

	// cookie (uint64, offset 16)
	binary.LittleEndian.PutUint64(buf[16:], 0)
}

// ReadStrongBinder reads a flat_binder_object and returns the handle.
// Accepts both BINDER_TYPE_HANDLE and BINDER_TYPE_BINDER; for both types,
// reads the uint32 at offset 8 (handle or low 32 bits of binder pointer).
func (p *Parcel) ReadStrongBinder() (uint32, error) {
	b, err := p.read(flatBinderObjectSize)
	if err != nil {
		return 0, err
	}

	objType := binary.LittleEndian.Uint32(b[0:])
	if objType != binderTypeHandle && objType != binderTypeBinder {
		return 0, fmt.Errorf("parcel: expected binder type %#x or %#x, got %#x",
			binderTypeHandle, binderTypeBinder, objType)
	}

	handle := binary.LittleEndian.Uint32(b[8:])
	return handle, nil
}

// WriteNullStrongBinder writes a null flat_binder_object (all zeros).
func (p *Parcel) WriteNullStrongBinder() {
	offset := uint64(p.Len())
	p.objects = append(p.objects, offset)
	p.grow(flatBinderObjectSize)
}

// ReadNullableStrongBinder reads a flat_binder_object that may be null.
// Returns the handle and true if a valid binder is present,
// or 0 and false if the binder is null.
// Null is detected as: type==0 (all-zero object) or BINDER_TYPE_BINDER with
// binder pointer==0 (Android's representation of a null IBinder).
func (p *Parcel) ReadNullableStrongBinder() (uint32, bool, error) {
	b, err := p.read(flatBinderObjectSize)
	if err != nil {
		return 0, false, err
	}

	objType := binary.LittleEndian.Uint32(b[0:])
	if objType == 0 {
		return 0, false, nil
	}

	if objType != binderTypeHandle && objType != binderTypeBinder {
		return 0, false, fmt.Errorf("parcel: expected binder type %#x, %#x, or null, got %#x",
			binderTypeHandle, binderTypeBinder, objType)
	}

	handle := binary.LittleEndian.Uint32(b[8:])

	// BINDER_TYPE_BINDER with binder==0 is Android's null IBinder.
	if objType == binderTypeBinder && handle == 0 {
		return 0, false, nil
	}

	return handle, true, nil
}
