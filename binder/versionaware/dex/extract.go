package dex

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
)

const transactionFieldPrefix = "TRANSACTION_"

var (
	stubSuffixBytes             = []byte("$Stub;")
	transactionFieldPrefixBytes = []byte(transactionFieldPrefix)
)

// ExtractFromJAR opens a JAR (ZIP) file, finds all DEX files inside,
// and extracts TRANSACTION_* constants from all $Stub inner classes.
// Returns a map of fully qualified interface name to TransactionCodes.
//
// Example key: "android.app.IActivityManager"
func ExtractFromJAR(path string) (map[string]TransactionCodes, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("opening JAR %q: %w", path, err)
	}
	defer zr.Close()

	// Quick check: skip JARs with no .dex entries to avoid
	// buffer allocation for the ~36 small JARs that are pure
	// Java bytecode.
	hasDEX := false
	for _, zf := range zr.File {
		if strings.HasSuffix(zf.Name, ".dex") {
			hasDEX = true
			break
		}
	}
	if !hasDEX {
		return nil, nil
	}

	result := map[string]TransactionCodes{}

	// Reuse a single buffer across DEX entries to reduce allocations.
	// The buffer grows to the largest DEX file size seen.
	var buf []byte

	for _, zf := range zr.File {
		if !strings.HasSuffix(zf.Name, ".dex") {
			continue
		}

		rc, err := zf.Open()
		if err != nil {
			return nil, fmt.Errorf("opening %q in JAR: %w", zf.Name, err)
		}

		needed := int(zf.UncompressedSize64)
		if needed > len(buf) {
			buf = make([]byte, needed)
		}
		data := buf[:needed]
		n, err := readFull(rc, data)
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("reading %q from JAR: %w", zf.Name, err)
		}
		data = data[:n]

		dexResult, err := ExtractFromDEX(data)
		if err != nil {
			return nil, fmt.Errorf("parsing %q: %w", zf.Name, err)
		}

		for iface, codes := range dexResult {
			existing, ok := result[iface]
			if !ok {
				result[iface] = codes
				continue
			}
			// Merge (later DEX files can contribute additional classes).
			for method, code := range codes {
				existing[method] = code
			}
		}
	}

	return result, nil
}

// readFull reads from r until EOF or the buffer is full.
// Returns the number of bytes read.
func readFull(r io.Reader, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := r.Read(buf[total:])
		total += n
		if err != nil {
			if errors.Is(err, io.EOF) {
				return total, nil
			}
			return total, err
		}
	}
	return total, nil
}

// ExtractFromDEX parses a single DEX file and extracts TRANSACTION_*
// constants from all $Stub classes.
// Returns a map of fully qualified interface name to TransactionCodes.
func ExtractFromDEX(data []byte) (map[string]TransactionCodes, error) {
	f, err := parseDEXFile(data)
	if err != nil {
		return nil, err
	}

	result := map[string]TransactionCodes{}

	for ci := uint32(0); ci < f.classDefsSize; ci++ {
		off := f.classDefsOff + ci*classDefItemSize
		if off+classDefItemSize > uint32(len(data)) {
			return nil, fmt.Errorf("class_def[%d] at offset 0x%x out of bounds", ci, off)
		}

		classIdx := binary.LittleEndian.Uint32(data[off:])

		// Check suffix on raw bytes to avoid allocating a string
		// for every class descriptor (the vast majority are not $Stub).
		isStub, err := f.typeDescriptorHasSuffix(classIdx, stubSuffixBytes)
		if err != nil {
			return nil, fmt.Errorf("class_def[%d]: reading descriptor: %w", ci, err)
		}
		if !isStub {
			continue
		}

		codes, err := extractStubTransactions(f, data, off)
		if err != nil {
			descBytes, _ := f.readTypeDescriptorBytes(classIdx)
			return nil, fmt.Errorf("class %s: %w", descBytes, err)
		}
		if len(codes) == 0 {
			continue
		}

		// Only allocate a string for the interface name of matching
		// $Stub classes (a small fraction of total classes).
		descBytes, err := f.readTypeDescriptorBytes(classIdx)
		if err != nil {
			return nil, fmt.Errorf("class_def[%d]: reading descriptor: %w", ci, err)
		}
		ifaceName := stubDescriptorBytesToInterface(descBytes)
		result[ifaceName] = codes
	}

	return result, nil
}

// ExtractDescriptorFromJAR extracts transaction codes for a single
// AIDL interface from a JAR file. The descriptor uses dot notation
// (e.g., "android.app.IActivityManager"). Returns nil, nil if the
// descriptor is not found in this JAR.
func ExtractDescriptorFromJAR(
	path string,
	descriptor string,
) (TransactionCodes, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("opening JAR %q: %w", path, err)
	}
	defer zr.Close()

	stubDesc := interfaceToStubDescriptor(descriptor)

	var buf []byte
	for _, zf := range zr.File {
		if !strings.HasSuffix(zf.Name, ".dex") {
			continue
		}

		rc, err := zf.Open()
		if err != nil {
			return nil, fmt.Errorf("opening %q in JAR: %w", zf.Name, err)
		}

		needed := int(zf.UncompressedSize64)
		if needed > len(buf) {
			buf = make([]byte, needed)
		}
		data := buf[:needed]
		n, err := readFull(rc, data)
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("reading %q from JAR: %w", zf.Name, err)
		}
		data = data[:n]

		codes, err := extractDescriptorFromDEX(data, stubDesc)
		if err != nil {
			return nil, fmt.Errorf("parsing %q: %w", zf.Name, err)
		}
		if codes != nil {
			return codes, nil
		}
	}

	return nil, nil
}

// extractDescriptorFromDEX extracts transaction codes for a single
// $Stub class matching stubDesc from DEX data. Returns nil, nil if
// the descriptor is not found.
func extractDescriptorFromDEX(
	data []byte,
	stubDesc []byte,
) (TransactionCodes, error) {
	f, err := parseDEXFile(data)
	if err != nil {
		return nil, err
	}

	for ci := uint32(0); ci < f.classDefsSize; ci++ {
		off := f.classDefsOff + ci*classDefItemSize
		if off+classDefItemSize > uint32(len(data)) {
			return nil, fmt.Errorf("class_def[%d] at offset 0x%x out of bounds", ci, off)
		}

		classIdx := binary.LittleEndian.Uint32(data[off:])

		descBytes, err := f.readTypeDescriptorBytes(classIdx)
		if err != nil {
			return nil, fmt.Errorf("class_def[%d]: reading descriptor: %w", ci, err)
		}
		if !bytes.Equal(descBytes, stubDesc) {
			continue
		}

		codes, err := extractStubTransactions(f, data, off)
		if err != nil {
			return nil, fmt.Errorf("class %s: %w", descBytes, err)
		}
		return codes, nil
	}

	return nil, nil
}

// interfaceToStubDescriptor converts a dot-notation interface name to
// a DEX $Stub class descriptor byte slice.
//
//	"android.app.IActivityManager" → []byte("Landroid/app/IActivityManager$Stub;")
func interfaceToStubDescriptor(descriptor string) []byte {
	// L + descriptor_with_slashes + $Stub;
	const suffix = "$Stub;"
	buf := make([]byte, 1+len(descriptor)+len(suffix))
	buf[0] = 'L'
	for i := range len(descriptor) {
		if descriptor[i] == '.' {
			buf[1+i] = '/'
		} else {
			buf[1+i] = descriptor[i]
		}
	}
	copy(buf[1+len(descriptor):], suffix)
	return buf
}

// extractStubTransactions reads the static fields and their initial
// values from a single $Stub class definition, returning only
// TRANSACTION_* integer constants.
//
// Field indices and static values are decoded in parallel to avoid
// allocating intermediate slices: field_idx_diff values are decoded
// from class_data_item while encoded_value entries are decoded from
// encoded_array_item, both in lock-step.
func extractStubTransactions(
	f *dexFile,
	data []byte,
	classDefOff uint32,
) (TransactionCodes, error) {
	classDataOff := binary.LittleEndian.Uint32(data[classDefOff+0x18:])
	staticValuesOff := binary.LittleEndian.Uint32(data[classDefOff+0x1C:])

	// A class with no class_data or no static initializer values
	// cannot have TRANSACTION_* constants.
	if classDataOff == 0 || staticValuesOff == 0 {
		return nil, nil
	}

	dataLen := uint32(len(data))
	if classDataOff >= dataLen {
		return nil, fmt.Errorf("class_data_off 0x%x out of bounds (data len %d)", classDataOff, dataLen)
	}
	if staticValuesOff >= dataLen {
		return nil, fmt.Errorf("static_values_off 0x%x out of bounds (data len %d)", staticValuesOff, dataLen)
	}

	// Parse class_data_item header: static_fields_size.
	pos := classDataOff
	staticFieldsSize, pos, err := readULEB128(data, pos)
	if err != nil {
		return nil, fmt.Errorf("reading static_fields_size: %w", err)
	}
	if staticFieldsSize == 0 {
		return nil, nil
	}

	// Skip remaining size fields (instance_fields, direct_methods, virtual_methods).
	for i := 0; i < 3; i++ {
		_, pos, err = readULEB128(data, pos)
		if err != nil {
			return nil, fmt.Errorf("reading size field %d: %w", i+1, err)
		}
	}

	// Parse encoded_array_item header: array size.
	valSize, valPos, err := readULEB128(data, staticValuesOff)
	if err != nil {
		return nil, fmt.Errorf("reading encoded_array size: %w", err)
	}

	// Iterate static fields and values in lock-step, decoding values
	// inline to avoid allocating a separate slice.
	codes := TransactionCodes{}
	var fieldIdx uint32
	for i := uint32(0); i < staticFieldsSize; i++ {
		diff, newPos, err := readULEB128(data, pos)
		if err != nil {
			return nil, fmt.Errorf("reading static_field[%d] idx_diff: %w", i, err)
		}
		pos = newPos

		_, pos, err = readULEB128(data, pos) // access_flags
		if err != nil {
			return nil, fmt.Errorf("reading static_field[%d] access_flags: %w", i, err)
		}

		fieldIdx += diff

		fid, err := f.readFieldID(fieldIdx)
		if err != nil {
			return nil, fmt.Errorf("reading field_id[%d]: %w", fieldIdx, err)
		}

		// Check prefix on raw bytes to avoid string allocation for
		// non-TRANSACTION_ fields (the majority of static fields).
		nameBytes, err := f.readStringBytes(fid.nameIdx)
		if err != nil {
			return nil, fmt.Errorf("reading field name for field_id[%d]: %w", fieldIdx, err)
		}

		if !bytes.HasPrefix(nameBytes, transactionFieldPrefixBytes) {
			// Still advance the value position to stay in sync.
			if i < valSize {
				_, valPos, err = readEncodedValue(data, valPos)
				if err != nil {
					return nil, fmt.Errorf("skipping encoded_value[%d]: %w", i, err)
				}
			}
			continue
		}

		if i >= valSize {
			// The encoded_array_item may have fewer entries than
			// static fields; remaining fields are zero-initialized.
			continue
		}

		val, newValPos, err := readEncodedValue(data, valPos)
		if err != nil {
			return nil, fmt.Errorf("reading encoded_value[%d]: %w", i, err)
		}
		valPos = newValPos

		// Only allocate a string for matching TRANSACTION_ field names.
		// This allocation is necessary because the string is used as
		// a map key that outlives the DEX data slice.
		methodName := string(nameBytes[len(transactionFieldPrefixBytes):])
		codes[methodName] = uint32(val.intVal)
	}

	return codes, nil
}

