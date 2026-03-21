package parcel

import (
	"encoding/binary"
	"fmt"
)

// WriteFileDescriptor writes a flat_binder_object with type BINDER_TYPE_FD
// containing the given file descriptor.
func (p *Parcel) WriteFileDescriptor(
	fd int32,
) {
	offset := uint64(p.Len())
	p.objects = append(p.objects, offset)

	buf := p.grow(flatBinderObjectSize)

	// type (uint32, offset 0)
	binary.LittleEndian.PutUint32(buf[0:], uint32(binderTypeFD))

	// flags (uint32, offset 4)
	binary.LittleEndian.PutUint32(buf[4:], uint32(binderFlagsAcceptFDs))

	// handle/fd (uint32, offset 8)
	binary.LittleEndian.PutUint32(buf[8:], uint32(fd))

	// pad (uint32, offset 12)
	binary.LittleEndian.PutUint32(buf[12:], 0)

	// cookie (uint64, offset 16)
	binary.LittleEndian.PutUint64(buf[16:], 0)
}

// ReadFileDescriptor reads a flat_binder_object with type BINDER_TYPE_FD
// and returns the file descriptor.
func (p *Parcel) ReadFileDescriptor() (int32, error) {
	b, err := p.read(flatBinderObjectSize)
	if err != nil {
		return 0, err
	}

	objType := binderObjectType(binary.LittleEndian.Uint32(b[0:]))
	if objType != binderTypeFD {
		return 0, fmt.Errorf("parcel: expected binder FD type %#x, got %#x", binderTypeFD, objType)
	}

	fd := int32(binary.LittleEndian.Uint32(b[8:]))
	if fd < 0 {
		return 0, fmt.Errorf("parcel: invalid file descriptor: %d", fd)
	}
	return fd, nil
}

// WriteParcelFileDescriptor writes a ParcelFileDescriptor (AIDL type) to
// the parcel. The wire format matches Android's NDK serialization:
//
//	int32(1)   - non-null indicator (from AParcel_writeParcelFileDescriptor)
//	int32(0)   - hasComm flag (from Parcel::writeParcelFileDescriptor)
//	FD object  - flat_binder_object with BINDER_TYPE_FD
//
// A negative fd writes int32(0) as the null indicator (no further data).
func (p *Parcel) WriteParcelFileDescriptor(
	fd int32,
) {
	if fd < 0 {
		p.WriteInt32(0) // null ParcelFileDescriptor
		return
	}
	p.WriteInt32(1) // non-null indicator
	p.WriteInt32(0) // hasComm = 0 (no communication channel)
	p.WriteFileDescriptor(fd)
}

// ReadParcelFileDescriptor reads a ParcelFileDescriptor (AIDL type) from
// the parcel. The wire format matches Android's NDK deserialization:
//
//	int32      - null indicator (0 = null, non-zero = non-null)
//	int32      - hasComm flag (from Parcel::readParcelFileDescriptor)
//	FD object  - flat_binder_object with BINDER_TYPE_FD
//
// Returns -1 for a null ParcelFileDescriptor.
func (p *Parcel) ReadParcelFileDescriptor() (int32, error) {
	nullInd, err := p.ReadInt32()
	if err != nil {
		return -1, fmt.Errorf("parcel: reading ParcelFileDescriptor null indicator: %w", err)
	}
	if nullInd == 0 {
		return -1, nil
	}

	// Read the hasComm flag (Parcel::readParcelFileDescriptor reads an
	// int32 that indicates whether a communication channel FD follows).
	hasComm, err := p.ReadInt32()
	if err != nil {
		return -1, fmt.Errorf("parcel: reading ParcelFileDescriptor hasComm: %w", err)
	}

	fd, err := p.ReadFileDescriptor()
	if err != nil {
		return -1, err
	}

	if hasComm != 0 {
		// Skip the communication channel FD (not used in our case).
		if _, err := p.ReadFileDescriptor(); err != nil {
			return -1, fmt.Errorf("parcel: reading ParcelFileDescriptor comm FD: %w", err)
		}
	}

	return fd, nil
}
