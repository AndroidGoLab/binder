package dex

import (
	"encoding/binary"
	"fmt"
	"unsafe"
)

// MethodSignatures maps method names to their parameter type descriptor lists.
// Example: {"registerClient": ["Landroid/os/ParcelUuid;", "Landroid/bluetooth/IBluetoothGattCallback;", "Z", "Landroid/content/AttributionSource;"]}
type MethodSignatures map[string][]string

// objectMethodNames contains method names inherited from java.lang.Object
// that should be skipped when extracting proxy method signatures.
var objectMethodNames = map[string]struct{}{
	"hashCode":       {},
	"equals":         {},
	"toString":       {},
	"getClass":       {},
	"notify":         {},
	"notifyAll":      {},
	"wait":           {},
	"finalize":       {},
	"clone":          {},
	"asBinder":       {},
	"getInterfaceDescriptor": {},
}

// extractProxyMethodSignatures reads virtual methods from a $Stub$Proxy
// class_def and returns the method name to parameter type descriptors mapping.
//
// classDefOff is the offset of the class_def_item within data.
func extractProxyMethodSignatures(
	f *dexFile,
	data []byte,
	classDefOff uint32,
) (MethodSignatures, error) {
	classDataOff := binary.LittleEndian.Uint32(data[classDefOff+0x18:])
	if classDataOff == 0 {
		return nil, nil
	}

	dataLen := uint32(len(data))
	if classDataOff >= dataLen {
		return nil, fmt.Errorf("class_data_off 0x%x out of bounds (data len %d)", classDataOff, dataLen)
	}

	// Parse class_data_item header:
	// static_fields_size, instance_fields_size, direct_methods_size, virtual_methods_size
	pos := classDataOff
	staticFieldsSize, pos, err := readULEB128(data, pos)
	if err != nil {
		return nil, fmt.Errorf("reading static_fields_size: %w", err)
	}

	instanceFieldsSize, pos, err := readULEB128(data, pos)
	if err != nil {
		return nil, fmt.Errorf("reading instance_fields_size: %w", err)
	}

	directMethodsSize, pos, err := readULEB128(data, pos)
	if err != nil {
		return nil, fmt.Errorf("reading direct_methods_size: %w", err)
	}

	virtualMethodsSize, pos, err := readULEB128(data, pos)
	if err != nil {
		return nil, fmt.Errorf("reading virtual_methods_size: %w", err)
	}

	if virtualMethodsSize == 0 {
		return nil, nil
	}

	// Skip static fields.
	for i := uint32(0); i < staticFieldsSize; i++ {
		_, pos, err = readULEB128(data, pos) // field_idx_diff
		if err != nil {
			return nil, fmt.Errorf("skipping static_field[%d] idx_diff: %w", i, err)
		}
		_, pos, err = readULEB128(data, pos) // access_flags
		if err != nil {
			return nil, fmt.Errorf("skipping static_field[%d] access_flags: %w", i, err)
		}
	}

	// Skip instance fields.
	for i := uint32(0); i < instanceFieldsSize; i++ {
		_, pos, err = readULEB128(data, pos) // field_idx_diff
		if err != nil {
			return nil, fmt.Errorf("skipping instance_field[%d] idx_diff: %w", i, err)
		}
		_, pos, err = readULEB128(data, pos) // access_flags
		if err != nil {
			return nil, fmt.Errorf("skipping instance_field[%d] access_flags: %w", i, err)
		}
	}

	// Skip direct methods.
	for i := uint32(0); i < directMethodsSize; i++ {
		_, pos, err = readULEB128(data, pos) // method_idx_diff
		if err != nil {
			return nil, fmt.Errorf("skipping direct_method[%d] idx_diff: %w", i, err)
		}
		_, pos, err = readULEB128(data, pos) // access_flags
		if err != nil {
			return nil, fmt.Errorf("skipping direct_method[%d] access_flags: %w", i, err)
		}
		_, pos, err = readULEB128(data, pos) // code_off
		if err != nil {
			return nil, fmt.Errorf("skipping direct_method[%d] code_off: %w", i, err)
		}
	}

	// Iterate virtual methods — these are the proxy methods.
	sigs := MethodSignatures{}
	var methodIdx uint32
	for i := uint32(0); i < virtualMethodsSize; i++ {
		diff, newPos, err := readULEB128(data, pos)
		if err != nil {
			return nil, fmt.Errorf("reading virtual_method[%d] idx_diff: %w", i, err)
		}
		pos = newPos

		_, pos, err = readULEB128(data, pos) // access_flags
		if err != nil {
			return nil, fmt.Errorf("reading virtual_method[%d] access_flags: %w", i, err)
		}

		_, pos, err = readULEB128(data, pos) // code_off
		if err != nil {
			return nil, fmt.Errorf("reading virtual_method[%d] code_off: %w", i, err)
		}

		methodIdx += diff

		_, protoIdx, nameIdx, err := f.readMethodID(methodIdx)
		if err != nil {
			return nil, fmt.Errorf("reading method_id[%d]: %w", methodIdx, err)
		}

		nameBytes, err := f.readStringBytes(nameIdx)
		if err != nil {
			return nil, fmt.Errorf("reading method name for method_id[%d]: %w", methodIdx, err)
		}

		// Skip Object-inherited and binder-internal methods.
		nameStr := unsafe.String(&nameBytes[0], len(nameBytes))
		if _, skip := objectMethodNames[nameStr]; skip {
			continue
		}

		paramTypeIndices, err := f.readProtoParams(uint32(protoIdx))
		if err != nil {
			return nil, fmt.Errorf("reading proto params for method_id[%d]: %w", methodIdx, err)
		}

		paramDescs := make([]string, len(paramTypeIndices))
		for pi, typeIdx := range paramTypeIndices {
			desc, err := f.readTypeDescriptor(typeIdx)
			if err != nil {
				return nil, fmt.Errorf("reading param type[%d] for method %s: %w", pi, nameBytes, err)
			}
			paramDescs[pi] = desc
		}

		// Allocate a proper string for the map key (nameBytes points
		// into f.data which may be reused).
		sigs[string(nameBytes)] = paramDescs
	}

	return sigs, nil
}
