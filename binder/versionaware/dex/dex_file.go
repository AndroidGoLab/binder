package dex

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
	"unsafe"
)

// DEX header field offsets.
// Per https://source.android.com/docs/core/runtime/dex-format#header-item
const (
	headerSize = 0x70

	offStringIDsSize = 0x38
	offStringIDsOff  = 0x3C
	offTypeIDsSize   = 0x40
	offTypeIDsOff    = 0x44
	offProtoIDsSize  = 0x48
	offProtoIDsOff   = 0x4C
	offFieldIDsSize  = 0x50
	offFieldIDsOff   = 0x54
	offMethodIDsSize = 0x58
	offMethodIDsOff  = 0x5C
	offClassDefsSize = 0x60
	offClassDefsOff  = 0x64
)

// Size of fixed-length structures in the DEX file.
const (
	stringIDItemSize = 4
	typeIDItemSize   = 4
	protoIDItemSize  = 12
	fieldIDItemSize  = 8
	methodIDItemSize = 8
	classDefItemSize = 32
)

// dexFile provides indexed access to a parsed DEX file's sections.
// Strings are read on demand to avoid loading the entire string table.
type dexFile struct {
	data []byte

	stringIDsSize uint32
	stringIDsOff  uint32
	typeIDsSize   uint32
	typeIDsOff    uint32
	protoIDsSize  uint32
	protoIDsOff   uint32
	fieldIDsSize  uint32
	fieldIDsOff   uint32
	methodIDsSize uint32
	methodIDsOff  uint32
	classDefsSize uint32
	classDefsOff  uint32
}

// parseDEXFile validates the header and extracts section offsets.
func parseDEXFile(data []byte) (*dexFile, error) {
	if len(data) < headerSize {
		return nil, fmt.Errorf("DEX data too short: %d bytes (need at least %d)", len(data), headerSize)
	}

	if data[0] != 'd' || data[1] != 'e' || data[2] != 'x' || data[3] != '\n' {
		return nil, fmt.Errorf("bad DEX magic: %q", data[0:8])
	}

	f := &dexFile{
		data:          data,
		stringIDsSize: binary.LittleEndian.Uint32(data[offStringIDsSize:]),
		stringIDsOff:  binary.LittleEndian.Uint32(data[offStringIDsOff:]),
		typeIDsSize:   binary.LittleEndian.Uint32(data[offTypeIDsSize:]),
		typeIDsOff:    binary.LittleEndian.Uint32(data[offTypeIDsOff:]),
		protoIDsSize:  binary.LittleEndian.Uint32(data[offProtoIDsSize:]),
		protoIDsOff:   binary.LittleEndian.Uint32(data[offProtoIDsOff:]),
		fieldIDsSize:  binary.LittleEndian.Uint32(data[offFieldIDsSize:]),
		fieldIDsOff:   binary.LittleEndian.Uint32(data[offFieldIDsOff:]),
		methodIDsSize: binary.LittleEndian.Uint32(data[offMethodIDsSize:]),
		methodIDsOff:  binary.LittleEndian.Uint32(data[offMethodIDsOff:]),
		classDefsSize: binary.LittleEndian.Uint32(data[offClassDefsSize:]),
		classDefsOff:  binary.LittleEndian.Uint32(data[offClassDefsOff:]),
	}

	// Validate that section extents fit within the data to catch
	// truncated or corrupted DEX files early.
	dataLen := uint64(len(data))
	sections := []struct {
		name string
		off  uint32
		size uint32
		item uint32
	}{
		{"string_ids", f.stringIDsOff, f.stringIDsSize, stringIDItemSize},
		{"type_ids", f.typeIDsOff, f.typeIDsSize, typeIDItemSize},
		{"proto_ids", f.protoIDsOff, f.protoIDsSize, protoIDItemSize},
		{"field_ids", f.fieldIDsOff, f.fieldIDsSize, fieldIDItemSize},
		{"method_ids", f.methodIDsOff, f.methodIDsSize, methodIDItemSize},
		{"class_defs", f.classDefsOff, f.classDefsSize, classDefItemSize},
	}
	for _, s := range sections {
		end := uint64(s.off) + uint64(s.size)*uint64(s.item)
		if end > dataLen {
			return nil, fmt.Errorf("DEX %s section extends past file end (off=0x%x, count=%d, item=%d, file=%d)", s.name, s.off, s.size, s.item, dataLen)
		}
	}

	return f, nil
}

// readStringBytes returns the raw MUTF-8 bytes of the string at the
// given string_ids index. The returned slice points into f.data, so it
// must not be modified. This avoids the allocation that string() causes.
func (f *dexFile) readStringBytes(idx uint32) ([]byte, error) {
	if idx >= f.stringIDsSize {
		return nil, fmt.Errorf("string index %d out of range (size=%d)", idx, f.stringIDsSize)
	}

	off := uint64(f.stringIDsOff) + uint64(idx)*stringIDItemSize
	if off+4 > uint64(len(f.data)) {
		return nil, fmt.Errorf("string_id_item at offset 0x%x out of bounds", off)
	}

	dataOff := binary.LittleEndian.Uint32(f.data[off:])
	if dataOff >= uint32(len(f.data)) {
		return nil, fmt.Errorf("string_data_off 0x%x out of bounds", dataOff)
	}

	// Skip the ULEB128-encoded utf16_size.
	pos := dataOff
	_, pos, err := readULEB128(f.data, pos)
	if err != nil {
		return nil, fmt.Errorf("reading string utf16_size at 0x%x: %w", dataOff, err)
	}

	if pos >= uint32(len(f.data)) {
		return nil, fmt.Errorf("string data at offset 0x%x past end of file", pos)
	}

	// Find null terminator using vectorized search.
	nullIdx := bytes.IndexByte(f.data[pos:], 0)
	if nullIdx < 0 {
		return nil, fmt.Errorf("string at offset 0x%x not null-terminated", pos)
	}

	return f.data[pos : pos+uint32(nullIdx)], nil
}


// typeDescriptorHasSuffix checks whether the type descriptor at the given
// type_ids index ends with suffix, without allocating a string.
func (f *dexFile) typeDescriptorHasSuffix(idx uint32, suffix []byte) (bool, error) {
	if idx >= f.typeIDsSize {
		return false, fmt.Errorf("type index %d out of range (size=%d)", idx, f.typeIDsSize)
	}

	off := uint64(f.typeIDsOff) + uint64(idx)*typeIDItemSize
	if off+4 > uint64(len(f.data)) {
		return false, fmt.Errorf("type_id_item at offset 0x%x out of bounds", off)
	}

	descriptorIdx := binary.LittleEndian.Uint32(f.data[off:])
	b, err := f.readStringBytes(descriptorIdx)
	if err != nil {
		return false, err
	}
	return bytes.HasSuffix(b, suffix), nil
}

// readTypeDescriptorBytes returns the raw bytes of the type descriptor
// at the given type_ids index without allocating a string.
func (f *dexFile) readTypeDescriptorBytes(idx uint32) ([]byte, error) {
	if idx >= f.typeIDsSize {
		return nil, fmt.Errorf("type index %d out of range (size=%d)", idx, f.typeIDsSize)
	}

	off := uint64(f.typeIDsOff) + uint64(idx)*typeIDItemSize
	if off+4 > uint64(len(f.data)) {
		return nil, fmt.Errorf("type_id_item at offset 0x%x out of bounds", off)
	}

	descriptorIdx := binary.LittleEndian.Uint32(f.data[off:])
	return f.readStringBytes(descriptorIdx)
}

// readTypeDescriptor returns the type descriptor at the given type_ids
// index as an allocated string. The returned string is safe to retain
// after f.data is reused or freed.
func (f *dexFile) readTypeDescriptor(idx uint32) (string, error) {
	b, err := f.readTypeDescriptorBytes(idx)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// readMethodID parses the method_id_item at the given method_ids index.
// Returns the class type index, proto index, and name string index.
func (f *dexFile) readMethodID(
	idx uint32,
) (classIdx uint16, protoIdx uint16, nameIdx uint32, err error) {
	if idx >= f.methodIDsSize {
		return 0, 0, 0, fmt.Errorf("method index %d out of range (size=%d)", idx, f.methodIDsSize)
	}

	off := uint64(f.methodIDsOff) + uint64(idx)*methodIDItemSize
	if off+methodIDItemSize > uint64(len(f.data)) {
		return 0, 0, 0, fmt.Errorf("method_id_item at offset 0x%x out of bounds", off)
	}

	classIdx = binary.LittleEndian.Uint16(f.data[off:])
	protoIdx = binary.LittleEndian.Uint16(f.data[off+2:])
	nameIdx = binary.LittleEndian.Uint32(f.data[off+4:])
	return classIdx, protoIdx, nameIdx, nil
}

// readProtoParams returns the parameter type indices for the proto_id
// at the given proto_ids index. Returns nil for zero-parameter protos.
func (f *dexFile) readProtoParams(protoIdx uint32) ([]uint32, error) {
	if protoIdx >= f.protoIDsSize {
		return nil, fmt.Errorf("proto index %d out of range (size=%d)", protoIdx, f.protoIDsSize)
	}

	off := uint64(f.protoIDsOff) + uint64(protoIdx)*protoIDItemSize
	if off+protoIDItemSize > uint64(len(f.data)) {
		return nil, fmt.Errorf("proto_id_item at offset 0x%x out of bounds", off)
	}

	// proto_id_item: shorty_idx(u32), return_type_idx(u32), parameters_off(u32)
	paramsOff := binary.LittleEndian.Uint32(f.data[off+8:])
	if paramsOff == 0 {
		return nil, nil
	}

	dataLen := uint32(len(f.data))
	if paramsOff+4 > dataLen {
		return nil, fmt.Errorf("type_list at offset 0x%x out of bounds", paramsOff)
	}

	// type_list: uint32 size, then size * uint16 type_idx entries.
	size := binary.LittleEndian.Uint32(f.data[paramsOff:])
	listEnd := uint64(paramsOff) + 4 + uint64(size)*2
	if listEnd > uint64(dataLen) {
		return nil, fmt.Errorf("type_list entries at offset 0x%x extend past file end", paramsOff)
	}

	typeIndices := make([]uint32, size)
	pos := paramsOff + 4
	for i := uint32(0); i < size; i++ {
		typeIndices[i] = uint32(binary.LittleEndian.Uint16(f.data[pos:]))
		pos += 2
	}
	return typeIndices, nil
}

// stubDescriptorToInterface converts a $Stub class descriptor to the
// AIDL interface's dot-separated name.
//
//	"Landroid/app/IActivityManager$Stub;" -> "android.app.IActivityManager"
func stubDescriptorToInterface(desc string) string {
	// Strip leading 'L' and trailing '$Stub;'.
	s := strings.TrimPrefix(desc, "L")
	s = strings.TrimSuffix(s, "$Stub;")
	return strings.ReplaceAll(s, "/", ".")
}

// stubDescriptorBytesToInterface converts a $Stub class descriptor byte
// slice to the AIDL interface's dot-separated name. Replaces '/' with '.'
// in a single pass and strips the 'L' prefix and '$Stub;' suffix.
//
//	[]byte("Landroid/app/IActivityManager$Stub;") -> "android.app.IActivityManager"
func stubDescriptorBytesToInterface(desc []byte) string {
	// Strip leading 'L'.
	start := 0
	if len(desc) > 0 && desc[0] == 'L' {
		start = 1
	}

	// Strip trailing '$Stub;' (6 bytes).
	end := len(desc)
	if end-start >= len(stubSuffixBytes) && bytes.Equal(desc[end-len(stubSuffixBytes):end], stubSuffixBytes) {
		end -= len(stubSuffixBytes)
	}

	// Build result replacing '/' -> '.' in a single allocation.
	body := desc[start:end]
	if len(body) == 0 {
		return ""
	}
	buf := make([]byte, len(body))
	for i, b := range body {
		if b == '/' {
			buf[i] = '.'
		} else {
			buf[i] = b
		}
	}
	return unsafe.String(&buf[0], len(buf))
}
