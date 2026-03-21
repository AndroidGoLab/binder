package dex

import "fmt"

// dexValueType represents a DEX encoded_value type tag.
// Per https://source.android.com/docs/core/runtime/dex-format#value-formats
type dexValueType byte

const (
	valueTypeByte         dexValueType = 0x00
	valueTypeShort        dexValueType = 0x02
	valueTypeChar         dexValueType = 0x03
	valueTypeInt          dexValueType = 0x04
	valueTypeLong         dexValueType = 0x06
	valueTypeFloat        dexValueType = 0x10
	valueTypeDouble       dexValueType = 0x11
	valueTypeMethodType   dexValueType = 0x15
	valueTypeMethodHandle dexValueType = 0x16
	valueTypeString       dexValueType = 0x17
	valueTypeType         dexValueType = 0x18
	valueTypeField        dexValueType = 0x19
	valueTypeMethod       dexValueType = 0x1a
	valueTypeEnum         dexValueType = 0x1b
	valueTypeArray        dexValueType = 0x1c
	valueTypeAnnotation   dexValueType = 0x1d
	valueTypeNull         dexValueType = 0x1e
	valueTypeBoolean      dexValueType = 0x1f
)

// encodedValue holds a decoded DEX encoded_value.
// Only the integer representation is stored, which suffices for
// extracting TRANSACTION_* constants.
type encodedValue struct {
	intVal int64
}

// readEncodedValue decodes a single encoded_value at the given position.
// It returns the decoded value and the new position past the consumed bytes.
func readEncodedValue(
	data []byte,
	pos uint32,
) (encodedValue, uint32, error) {
	if pos >= uint32(len(data)) {
		return encodedValue{}, pos, fmt.Errorf("encoded_value truncated at offset 0x%x", pos)
	}

	byte0 := data[pos]
	pos++
	valueType := dexValueType(byte0 & 0x1F)
	valueArg := byte0 >> 5

	switch valueType {
	case valueTypeNull:
		return encodedValue{}, pos, nil

	case valueTypeBoolean:
		var v int64
		if valueArg != 0 {
			v = 1
		}
		return encodedValue{intVal: v}, pos, nil

	case valueTypeByte:
		// DEX spec requires value_arg == 0 for VALUE_BYTE (always 1 byte).
		if valueArg != 0 {
			return encodedValue{}, pos, fmt.Errorf("VALUE_BYTE has invalid value_arg %d (must be 0) at offset 0x%x", valueArg, pos-1)
		}
		return readSignedEncodedInt(data, pos, 1)

	case valueTypeShort:
		return readSignedEncodedInt(data, pos, uint32(valueArg)+1)

	case valueTypeChar:
		return readUnsignedEncodedInt(data, pos, uint32(valueArg)+1)

	case valueTypeInt:
		return readSignedEncodedInt(data, pos, uint32(valueArg)+1)

	case valueTypeLong:
		return readSignedEncodedInt(data, pos, uint32(valueArg)+1)

	case valueTypeFloat:
		// Right-zero-extended: read value_arg+1 bytes into the high bytes of a 4-byte value.
		// We don't need float precision for transaction codes; store raw bits.
		return readUnsignedEncodedInt(data, pos, uint32(valueArg)+1)

	case valueTypeDouble:
		return readUnsignedEncodedInt(data, pos, uint32(valueArg)+1)

	case valueTypeMethodType, valueTypeMethodHandle,
		valueTypeString, valueTypeType,
		valueTypeField, valueTypeMethod, valueTypeEnum:
		// Index types: unsigned, value_arg+1 bytes.
		return readUnsignedEncodedInt(data, pos, uint32(valueArg)+1)

	case valueTypeArray:
		size, newPos, err := readULEB128(data, pos)
		if err != nil {
			return encodedValue{}, newPos, fmt.Errorf("reading array size: %w", err)
		}
		pos = newPos
		for i := uint32(0); i < size; i++ {
			_, pos, err = readEncodedValue(data, pos)
			if err != nil {
				return encodedValue{}, pos, fmt.Errorf("reading array element %d: %w", i, err)
			}
		}
		return encodedValue{}, pos, nil

	case valueTypeAnnotation:
		// type_idx (uleb128) + size (uleb128) + name/value pairs.
		_, newPos, err := readULEB128(data, pos)
		if err != nil {
			return encodedValue{}, newPos, fmt.Errorf("reading annotation type_idx: %w", err)
		}
		pos = newPos
		annSize, newPos, err := readULEB128(data, pos)
		if err != nil {
			return encodedValue{}, newPos, fmt.Errorf("reading annotation size: %w", err)
		}
		pos = newPos
		for i := uint32(0); i < annSize; i++ {
			_, pos, err = readULEB128(data, pos) // name_idx
			if err != nil {
				return encodedValue{}, pos, fmt.Errorf("reading annotation name %d: %w", i, err)
			}
			_, pos, err = readEncodedValue(data, pos)
			if err != nil {
				return encodedValue{}, pos, fmt.Errorf("reading annotation value %d: %w", i, err)
			}
		}
		return encodedValue{}, pos, nil

	default:
		return encodedValue{}, pos, fmt.Errorf("unknown encoded_value type 0x%02x at offset 0x%x", valueType, pos-1)
	}
}

// readSignedEncodedInt reads size bytes of little-endian data and sign-extends the result.
func readSignedEncodedInt(
	data []byte,
	pos uint32,
	size uint32,
) (encodedValue, uint32, error) {
	if pos+size > uint32(len(data)) {
		return encodedValue{}, pos, fmt.Errorf("signed int truncated at offset 0x%x (need %d bytes)", pos, size)
	}

	var val int64
	for i := uint32(0); i < size; i++ {
		val |= int64(data[pos+i]) << (8 * i)
	}

	// Sign-extend from the topmost byte.
	signBit := int64(1) << (size*8 - 1)
	if val&signBit != 0 {
		val |= ^((signBit << 1) - 1)
	}

	return encodedValue{intVal: val}, pos + size, nil
}

// readUnsignedEncodedInt reads size bytes of little-endian data as an unsigned value.
func readUnsignedEncodedInt(
	data []byte,
	pos uint32,
	size uint32,
) (encodedValue, uint32, error) {
	if pos+size > uint32(len(data)) {
		return encodedValue{}, pos, fmt.Errorf("unsigned int truncated at offset 0x%x (need %d bytes)", pos, size)
	}

	var val int64
	for i := uint32(0); i < size; i++ {
		val |= int64(data[pos+i]) << (8 * i)
	}

	return encodedValue{intVal: val}, pos + size, nil
}
