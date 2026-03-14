package parcel

import (
	"encoding/binary"
	"math"
)

// WriteInt32 writes a 32-bit signed integer.
func (p *Parcel) WriteInt32(
	v int32,
) {
	binary.LittleEndian.PutUint32(p.grow(4), uint32(v))
}

// ReadInt32 reads a 32-bit signed integer.
func (p *Parcel) ReadInt32() (int32, error) {
	b, err := p.read(4)
	if err != nil {
		return 0, err
	}
	return int32(binary.LittleEndian.Uint32(b)), nil
}

// WriteUint32 writes a 32-bit unsigned integer.
func (p *Parcel) WriteUint32(
	v uint32,
) {
	binary.LittleEndian.PutUint32(p.grow(4), v)
}

// ReadUint32 reads a 32-bit unsigned integer.
func (p *Parcel) ReadUint32() (uint32, error) {
	b, err := p.read(4)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(b), nil
}

// WriteInt64 writes a 64-bit signed integer.
func (p *Parcel) WriteInt64(
	v int64,
) {
	binary.LittleEndian.PutUint64(p.grow(8), uint64(v))
}

// ReadInt64 reads a 64-bit signed integer.
func (p *Parcel) ReadInt64() (int64, error) {
	b, err := p.read(8)
	if err != nil {
		return 0, err
	}
	return int64(binary.LittleEndian.Uint64(b)), nil
}

// WriteUint64 writes a 64-bit unsigned integer.
func (p *Parcel) WriteUint64(
	v uint64,
) {
	binary.LittleEndian.PutUint64(p.grow(8), v)
}

// ReadUint64 reads a 64-bit unsigned integer.
func (p *Parcel) ReadUint64() (uint64, error) {
	b, err := p.read(8)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint64(b), nil
}

// WriteBool writes a boolean as an int32 (0 or 1).
func (p *Parcel) WriteBool(
	v bool,
) {
	var n int32
	if v {
		n = 1
	}
	p.WriteInt32(n)
}

// ReadBool reads a boolean from an int32 value.
func (p *Parcel) ReadBool() (bool, error) {
	v, err := p.ReadInt32()
	if err != nil {
		return false, err
	}
	return v != 0, nil
}

// WriteFloat32 writes a 32-bit floating point number.
func (p *Parcel) WriteFloat32(
	v float32,
) {
	p.WriteUint32(math.Float32bits(v))
}

// ReadFloat32 reads a 32-bit floating point number.
func (p *Parcel) ReadFloat32() (float32, error) {
	v, err := p.ReadUint32()
	if err != nil {
		return 0, err
	}
	return math.Float32frombits(v), nil
}

// WriteFloat64 writes a 64-bit floating point number.
func (p *Parcel) WriteFloat64(
	v float64,
) {
	p.WriteUint64(math.Float64bits(v))
}

// ReadFloat64 reads a 64-bit floating point number.
func (p *Parcel) ReadFloat64() (float64, error) {
	v, err := p.ReadUint64()
	if err != nil {
		return 0, err
	}
	return math.Float64frombits(v), nil
}

// WritePaddedByte writes a single byte (padded to 4 bytes).
func (p *Parcel) WritePaddedByte(
	v byte,
) {
	buf := p.grow(4)
	buf[0] = v
	buf[1] = 0
	buf[2] = 0
	buf[3] = 0
}

// ReadPaddedByte reads a single byte (consuming 4 bytes with padding).
func (p *Parcel) ReadPaddedByte() (byte, error) {
	b, err := p.read(4)
	if err != nil {
		return 0, err
	}
	return b[0], nil
}

// WriteByteArray writes a byte array with an int32 length prefix, padded to 4 bytes.
// A nil slice writes -1 as the length.
func (p *Parcel) WriteByteArray(
	data []byte,
) {
	if data == nil {
		p.WriteInt32(-1)
		return
	}

	p.WriteInt32(int32(len(data)))
	if len(data) > 0 {
		copy(p.grow(len(data)), data)
	}
}

// ReadByteArray reads a byte array with an int32 length prefix.
// Returns nil if the length is -1.
func (p *Parcel) ReadByteArray() ([]byte, error) {
	length, err := p.ReadInt32()
	if err != nil {
		return nil, err
	}

	if length < 0 {
		return nil, nil
	}

	b, err := p.read(int(length))
	if err != nil {
		return nil, err
	}

	result := make([]byte, length)
	copy(result, b)
	return result, nil
}
