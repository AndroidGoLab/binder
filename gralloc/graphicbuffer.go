package gralloc

import (
	"encoding/binary"
	"fmt"

	common "github.com/AndroidGoLab/binder/android/hardware/common"
	"github.com/AndroidGoLab/binder/parcel"
)

// GraphicBuffer magic value: "GB01" in little-endian.
const graphicBufferMagic uint32 = 0x47423031

// ReadGraphicBuffer reads a flattened GraphicBuffer from a parcel.
// The wire format follows GraphicBuffer::unflatten():
//
//	int32(flattenedSize) + int32(fdCount) + raw[flattenedSize] + fd objects
func ReadGraphicBuffer(p *parcel.Parcel) (*Buffer, error) {
	flattenedSize, err := p.ReadInt32()
	if err != nil {
		return nil, fmt.Errorf("reading flattenedSize: %w", err)
	}
	fdCount, err := p.ReadInt32()
	if err != nil {
		return nil, fmt.Errorf("reading fdCount: %w", err)
	}

	if flattenedSize < 13*4 {
		return nil, fmt.Errorf("flattened size %d too small (minimum 52)", flattenedSize)
	}

	raw, err := p.Read(int(flattenedSize))
	if err != nil {
		return nil, fmt.Errorf("reading flattened data: %w", err)
	}

	magic := binary.LittleEndian.Uint32(raw[0:4])
	if magic != graphicBufferMagic {
		return nil, fmt.Errorf("bad GraphicBuffer magic: 0x%08x (expected 0x%08x)", magic, graphicBufferMagic)
	}

	width := binary.LittleEndian.Uint32(raw[4:8])
	height := binary.LittleEndian.Uint32(raw[8:12])
	stride := int32(binary.LittleEndian.Uint32(raw[12:16]))
	format := int32(binary.LittleEndian.Uint32(raw[16:20]))
	usageLow := binary.LittleEndian.Uint32(raw[24:28])
	usageHigh := binary.LittleEndian.Uint32(raw[48:52])
	numFds := int32(binary.LittleEndian.Uint32(raw[40:44]))
	numInts := int32(binary.LittleEndian.Uint32(raw[44:48]))

	if numFds != fdCount {
		return nil, fmt.Errorf("fd count mismatch: header=%d, outer=%d", numFds, fdCount)
	}

	// Read native_handle ints from the flattened data (after 13-word header).
	ints := make([]int32, numInts)
	for i := range ints {
		off := 52 + i*4
		if off+4 > len(raw) {
			break
		}
		ints[i] = int32(binary.LittleEndian.Uint32(raw[off : off+4]))
	}

	// Read FDs from parcel (as flat_binder_objects).
	fds := make([]int32, numFds)
	for i := range fds {
		fd, err := p.ReadFileDescriptor()
		if err != nil {
			return nil, fmt.Errorf("reading fd %d: %w", i, err)
		}
		fds[i] = fd
	}

	return &Buffer{
		Handle: common.NativeHandle{
			Fds:  fds,
			Ints: ints,
		},
		Stride: stride,
		Width:  width,
		Height: height,
		Format: format,
		Usage:  uint64(usageHigh)<<32 | uint64(usageLow),
	}, nil
}
