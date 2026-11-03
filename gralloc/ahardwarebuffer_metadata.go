package gralloc

import (
	"encoding/binary"
	"errors"
)

// ahbMetadata describes the expected native buffer geometry and usage.
//
// Example: validate socket metadata against AHardwareBuffer_describe.
type ahbMetadata struct {
	width, height, layers, format, stride uint32
	usage                                 uint64
}

// validateAHBMetadata checks the flattened header and extracts opaque integers.
//
// Example: reject a transport whose descriptor count disagrees with the header.
func validateAHBMetadata(raw []byte, d ahbMetadata, fdCount int) ([]int32, error) {
	if len(raw) < 52 {
		return nil, errors.New("short AHardwareBuffer metadata")
	}
	u := binary.LittleEndian.Uint32
	if u(raw) != 0x47423031 || u(raw[4:]) != d.width || u(raw[8:]) != d.height || u(raw[12:]) != d.stride || u(raw[16:]) != d.format || u(raw[20:]) != d.layers || (uint64(u(raw[24:]))|uint64(u(raw[48:]))<<32) != d.usage || uint64(u(raw[40:])) != uint64(fdCount) || uint64(u(raw[44:]))*4 != uint64(len(raw)-52) {
		return nil, errors.New("invalid AHardwareBuffer flattened metadata")
	}
	ints := make([]int32, int(u(raw[44:])))
	for i := range ints {
		ints[i] = int32(u(raw[52+i*4:]))
	}
	return ints, nil
}
