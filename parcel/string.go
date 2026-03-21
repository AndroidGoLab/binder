package parcel

import (
	"encoding/binary"
	"fmt"
	"math"
	"unicode/utf16"
)

const (
	// maxStringChars is the maximum number of UTF-16 code units (or bytes for
	// UTF-8 strings) allowed when reading strings from a parcel. The binder
	// transaction limit is 1MB, so a string cannot validly contain more than
	// 512K UTF-16 chars (1MB / 2 bytes per char). This guards against OOM
	// from malicious or corrupted parcels.
	maxStringChars = 512 * 1024
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

	// Silently drop strings exceeding int32 max UTF-16 code units.
	// In practice, Go strings cannot reach this size on any real system.
	if charCount > math.MaxInt32 {
		return
	}

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
		// Null string on the wire (charCount == -1). Returns empty string
		// because Go strings cannot represent null. Use ReadNullableString16
		// to distinguish null from empty.
		return "", nil
	}

	if int(charCount) > maxStringChars {
		return "", fmt.Errorf("parcel: ReadString16 charCount %d exceeds limit %d", charCount, maxStringChars)
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

// ReadNullableString16 reads a string in UTF-16LE wire format,
// distinguishing null from empty. Returns nil for null strings
// (charCount == -1) and a pointer to the string value otherwise.
func (p *Parcel) ReadNullableString16() (*string, error) {
	charCount, err := p.ReadInt32()
	if err != nil {
		return nil, err
	}

	if charCount < 0 {
		return nil, nil
	}

	if int(charCount) > maxStringChars {
		return nil, fmt.Errorf("parcel: ReadNullableString16 charCount %d exceeds limit %d", charCount, maxStringChars)
	}

	// Read (charCount+1)*2 bytes for data plus null terminator.
	dataBytes := (int(charCount) + 1) * 2
	b, err := p.read(dataBytes)
	if err != nil {
		return nil, err
	}

	units := make([]uint16, charCount)
	for i := range units {
		units[i] = binary.LittleEndian.Uint16(b[i*2:])
	}

	s := string(utf16.Decode(units))
	return &s, nil
}

// WriteString writes a string in UTF-8 wire format (for @utf8InCpp).
// Writes int32 byte length, then UTF-8 bytes with a null terminator,
// padded to 4 bytes. An empty string writes length 0 followed by a null byte.
// Use WriteNullString to write a null sentinel (-1 length).
func (p *Parcel) WriteString(
	s string,
) {
	byteLen := len(s)

	// Silently drop strings exceeding int32 max bytes.
	// In practice, Go strings cannot reach this size on any real system.
	if byteLen > math.MaxInt32 {
		return
	}

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
		// Null string on the wire (byteLen < 0). Returns empty string
		// because Go strings cannot represent null. Use ReadNullableString
		// to distinguish null from empty.
		return "", nil
	}

	// UTF-8 uses at most 4 bytes per character.
	if int(byteLen) > maxStringChars*4 {
		return "", fmt.Errorf("parcel: ReadString byteLen %d exceeds limit %d", byteLen, maxStringChars*4)
	}

	// Read byteLen+1 bytes for data plus null terminator.
	b, err := p.read(int(byteLen) + 1)
	if err != nil {
		return "", err
	}

	return string(b[:byteLen]), nil
}

// ReadNullableString reads a string in UTF-8 wire format,
// distinguishing null from empty. Returns nil for null strings
// (byteLen < 0) and a pointer to the string value otherwise.
func (p *Parcel) ReadNullableString() (*string, error) {
	byteLen, err := p.ReadInt32()
	if err != nil {
		return nil, err
	}

	if byteLen < 0 {
		return nil, nil
	}

	// UTF-8 uses at most 4 bytes per character.
	if int(byteLen) > maxStringChars*4 {
		return nil, fmt.Errorf("parcel: ReadNullableString byteLen %d exceeds limit %d", byteLen, maxStringChars*4)
	}

	// Read byteLen+1 bytes for data plus null terminator.
	b, err := p.read(int(byteLen) + 1)
	if err != nil {
		return nil, err
	}

	s := string(b[:byteLen])
	return &s, nil
}
