package parcel

import (
	"encoding/binary"
	"fmt"
)

const (
	// binderTypeFD is the type for a file descriptor in a flat_binder_object.
	// Kernel value: B_PACK_CHARS('f','d','*',0x85) = 0x66642a85.
	binderTypeFD = uint32(0x66642a85)
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
	binary.LittleEndian.PutUint32(buf[0:], binderTypeFD)

	// flags (uint32, offset 4)
	binary.LittleEndian.PutUint32(buf[4:], binderFlagsAcceptFDs)

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

	objType := binary.LittleEndian.Uint32(b[0:])
	if objType != binderTypeFD {
		return 0, fmt.Errorf("parcel: expected binder FD type %#x, got %#x", binderTypeFD, objType)
	}

	fd := int32(binary.LittleEndian.Uint32(b[8:]))
	return fd, nil
}
