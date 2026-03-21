package parcel

import (
	"encoding/binary"
	"fmt"
)

// binderObjectType represents the type field of a flat_binder_object,
// identifying the kind of object (local binder, handle, file descriptor, etc.).
type binderObjectType uint32

// binderFlags represents the flags field of a flat_binder_object.
type binderFlags uint32

const (
	// binderTypeBinder is the type for a local Binder object.
	// Kernel value: B_PACK_CHARS('s','b','*',0x85) = 0x73622a85.
	binderTypeBinder = binderObjectType(0x73622a85)

	// binderTypeHandle is the type for a Binder handle reference.
	// Kernel value: B_PACK_CHARS('s','h','*',0x85) = 0x73682a85.
	binderTypeHandle = binderObjectType(0x73682a85)

	// binderTypeFD is the type for a file descriptor in a flat_binder_object.
	// Kernel value: B_PACK_CHARS('f','d','*',0x85) = 0x66642a85.
	binderTypeFD = binderObjectType(0x66642a85)

	// binderFlagsSchedBits contains the default scheduling policy bits.
	// Android sets SCHED_NORMAL (0) with priority 19 (nice 19, lowest),
	// giving schedBits = 0x13.
	binderFlagsSchedBits = binderFlags(0x13)

	// binderFlagsAcceptFDs is FLAT_BINDER_FLAG_ACCEPTS_FDS (0x100) OR'd
	// with the default scheduling bits. Only used for local binders
	// (BINDER_TYPE_BINDER), NOT for handle (remote) binders.
	binderFlagsAcceptFDs = binderFlags(0x100) | binderFlagsSchedBits

	// flatBinderObjectSize is the size of a flat_binder_object (24 bytes on 64-bit).
	flatBinderObjectSize = 24
)

// WriteLocalBinder writes a local binder object to the parcel.
//
// binderPtr is the binder node address -- the kernel uses it to find or
// create the binder_node that represents this object. It must be a unique,
// non-zero process-space address (analogous to BBinder::getWeakRefs() in
// the C++ implementation).
//
// cookie is echoed back in incoming BR_TRANSACTION events and is used by
// the process for dispatch (analogous to the BBinder* pointer in C++).
// It must also be a non-zero process-space address.
//
// The kernel converts this BINDER_TYPE_BINDER into BINDER_TYPE_HANDLE in
// the receiving process.
//
// After the flat_binder_object, an int32 stability level is written
// (matching Android's Parcel::finishFlattenBinder).
func (p *Parcel) WriteLocalBinder(
	binderPtr uintptr,
	cookie uintptr,
) {
	// The kernel rejects zero binder/cookie values; catch the mistake
	// early rather than triggering a cryptic ioctl failure later.
	if binderPtr == 0 {
		panic("WriteLocalBinder: binderPtr must be non-zero")
	}
	if cookie == 0 {
		panic("WriteLocalBinder: cookie must be non-zero")
	}

	offset := uint64(p.Len())
	p.objects = append(p.objects, offset)

	buf := p.grow(flatBinderObjectSize)

	// type (uint32, offset 0)
	binary.LittleEndian.PutUint32(buf[0:], uint32(binderTypeBinder))

	// flags (uint32, offset 4)
	binary.LittleEndian.PutUint32(buf[4:], uint32(binderFlagsAcceptFDs))

	// binder (binder_uintptr_t, offset 8) — kernel node identity
	binary.LittleEndian.PutUint64(buf[8:], uint64(binderPtr))

	// cookie (binder_uintptr_t, offset 16) — dispatch key
	binary.LittleEndian.PutUint64(buf[16:], uint64(cookie))

	// Stability level — Android writes this via finishFlattenBinder.
	// System-level binders use SYSTEM stability (12).
	p.WriteInt32(int32(StabilitySystem))
}

// WriteStrongBinder writes a flat_binder_object with the given handle.
// Records the offset in the parcel's objects array.
// After the flat_binder_object, an int32 stability level is written
// (matching Android's Parcel::finishFlattenBinder).
func (p *Parcel) WriteStrongBinder(
	handle uint32,
) {
	offset := uint64(p.Len())
	p.objects = append(p.objects, offset)

	buf := p.grow(flatBinderObjectSize)

	// type (uint32, offset 0)
	binary.LittleEndian.PutUint32(buf[0:], uint32(binderTypeHandle))

	// flags (uint32, offset 4) — only schedBits for remote handles.
	// FLAT_BINDER_FLAG_ACCEPTS_FDS is only for local binders per AOSP.
	binary.LittleEndian.PutUint32(buf[4:], uint32(binderFlagsSchedBits))

	// handle (uint32, offset 8)
	binary.LittleEndian.PutUint32(buf[8:], handle)

	// pad (uint32, offset 12)
	binary.LittleEndian.PutUint32(buf[12:], 0)

	// cookie (uint64, offset 16)
	binary.LittleEndian.PutUint64(buf[16:], 0)

	// Stability level — system-level for handle references.
	p.WriteInt32(int32(StabilitySystem))
}

// ReadStrongBinder reads a non-null flat_binder_object and returns the handle.
// Accepts both BINDER_TYPE_HANDLE and BINDER_TYPE_BINDER; for both types,
// reads the uint32 at offset 8 (handle or low 32 bits of binder pointer).
// Also reads the int32 stability level that follows the flat_binder_object.
//
// Returns an error if the binder is null (BINDER_TYPE_BINDER with binder=0),
// matching the AIDL-generated UNEXPECTED_NULL check in the C++ implementation.
func (p *Parcel) ReadStrongBinder() (uint32, error) {
	b, err := p.read(flatBinderObjectSize)
	if err != nil {
		return 0, err
	}

	objType := binderObjectType(binary.LittleEndian.Uint32(b[0:]))
	if objType != binderTypeHandle && objType != binderTypeBinder {
		return 0, fmt.Errorf("parcel: expected binder type %#x or %#x, got %#x",
			binderTypeHandle, binderTypeBinder, objType)
	}

	handle := binary.LittleEndian.Uint32(b[8:])

	// Read and discard the stability level (finishUnflattenBinder).
	if _, err := p.ReadInt32(); err != nil {
		return 0, fmt.Errorf("parcel: reading binder stability: %w", err)
	}

	// Reject null binders: BINDER_TYPE_BINDER with binder pointer = 0.
	if objType == binderTypeBinder && handle == 0 {
		return 0, fmt.Errorf("parcel: unexpected null binder in non-nullable read")
	}

	return handle, nil
}

// WriteNullStrongBinder writes a null flat_binder_object.
// Android's flattenBinder(nullptr) writes type=BINDER_TYPE_BINDER with
// binder=0 and cookie=0. The null object is NOT recorded in the objects
// array (Parcel::writeObject skips it when binder==0 && !nullMetaData).
// Intentionally does not record an offset in the objects array, matching
// AOSP behavior: null binder objects are inert data, not real binder refs.
// Followed by UNDECLARED stability level (finishFlattenBinder).
func (p *Parcel) WriteNullStrongBinder() {
	buf := p.grow(flatBinderObjectSize)

	// type must be BINDER_TYPE_BINDER even for null (Android convention).
	binary.LittleEndian.PutUint32(buf[0:], uint32(binderTypeBinder))
	// flags, binder, cookie are all zero (from grow's zero-fill).

	// Null binder uses UNDECLARED stability.
	p.WriteInt32(int32(StabilityUndeclared))
}

// ReadNullableStrongBinder reads a flat_binder_object that may be null.
// Returns the handle and true if a valid binder is present,
// or 0 and false if the binder is null.
// Null is detected as: type==0 (all-zero object) or BINDER_TYPE_BINDER with
// binder pointer==0 (Android's representation of a null IBinder).
// Also reads the int32 stability level that follows the flat_binder_object.
func (p *Parcel) ReadNullableStrongBinder() (uint32, bool, error) {
	b, err := p.read(flatBinderObjectSize)
	if err != nil {
		return 0, false, err
	}

	objType := binderObjectType(binary.LittleEndian.Uint32(b[0:]))

	// Read the stability level (always present after the flat_binder_object).
	if _, err := p.ReadInt32(); err != nil {
		return 0, false, fmt.Errorf("parcel: reading binder stability: %w", err)
	}

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
