package parcel

import (
	"encoding/binary"
	"unicode/utf16"
)

// WriteString16 writes a string in UTF-16LE wire format.
// Writes int32 char count (number of UTF-16 code units),
// then UTF-16LE encoded data with a null terminator, padded to 4 bytes.
// An empty string writes length 0 followed by a UTF-16 null terminator.
// Use WriteNullString16 to write a null sentinel (-1 length).
func (p *Parcel) WriteString16(
	s string,
) {
	runes := []rune(s)
	encoded := utf16.Encode(runes)
	charCount := len(encoded)

	p.WriteInt32(int32(charCount))

	// Write UTF-16LE encoded data plus null terminator.
	dataBytes := (charCount + 1) * 2
	buf := p.grow(dataBytes)
	for i, u := range encoded {
		binary.LittleEndian.PutUint16(buf[i*2:], u)
	}
	// Null terminator.
	binary.LittleEndian.PutUint16(buf[charCount*2:], 0)
}

// WriteNullString16 writes a null string sentinel (-1 length) in UTF-16LE wire format.
func (p *Parcel) WriteNullString16() {
	p.WriteInt32(-1)
}

// ReadString16 reads a string in UTF-16LE wire format.
// Reads int32 char count. If -1, returns empty string.
// Then reads (charCount+1)*2 bytes (including null terminator), padded to 4 bytes.
func (p *Parcel) ReadString16() (string, error) {
	charCount, err := p.ReadInt32()
	if err != nil {
		return "", err
	}

	if charCount < 0 {
		return "", nil
	}

	// Read (charCount+1)*2 bytes for data plus null terminator.
	dataBytes := (int(charCount) + 1) * 2
	b, err := p.read(dataBytes)
	if err != nil {
		return "", err
	}

	units := make([]uint16, charCount)
	for i := range units {
		units[i] = binary.LittleEndian.Uint16(b[i*2:])
	}

	return string(utf16.Decode(units)), nil
}

// WriteString writes a string in UTF-8 wire format (for @utf8InCpp).
// Writes int32 byte length, then UTF-8 bytes with a null terminator,
// padded to 4 bytes. An empty string writes length 0 followed by a null byte.
// Use WriteNullString to write a null sentinel (-1 length).
func (p *Parcel) WriteString(
	s string,
) {
	byteLen := len(s)
	p.WriteInt32(int32(byteLen))

	// Write UTF-8 bytes plus null terminator.
	buf := p.grow(byteLen + 1)
	copy(buf[:byteLen], s)
	buf[byteLen] = 0
}

// WriteNullString writes a null string sentinel (-1 length) in UTF-8 wire format.
func (p *Parcel) WriteNullString() {
	p.WriteInt32(-1)
}

// ReadString reads a string in UTF-8 wire format (for @utf8InCpp).
// Reads int32 byte length. If -1, returns empty string.
// Then reads byteLen+1 bytes (including null terminator), padded to 4 bytes.
func (p *Parcel) ReadString() (string, error) {
	byteLen, err := p.ReadInt32()
	if err != nil {
		return "", err
	}

	if byteLen < 0 {
		return "", nil
	}

	// Read byteLen+1 bytes for data plus null terminator.
	b, err := p.read(int(byteLen) + 1)
	if err != nil {
		return "", err
	}

	return string(b[:byteLen]), nil
}
