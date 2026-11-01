package gralloc

import (
	"encoding/binary"
	"errors"
	"fmt"

	common "github.com/AndroidGoLab/binder/android/hardware/common"
	"github.com/AndroidGoLab/binder/parcel"
	"golang.org/x/sys/unix"
)

// graphicBufferMagic identifies the flattened GraphicBuffer wire format.
//
// Example: reject a flattened buffer with a different magic value.
const graphicBufferMagic uint32 = 0x47423031

// ReadGraphicBuffer reads a flattened GraphicBuffer from a parcel.
// The wire format follows GraphicBuffer::unflatten():
//
//	int32(flattenedSize) + int32(fdCount) + raw[flattenedSize] + fd objects
//
// Received descriptors are transferred to the caller; descriptors in a
// borrowed parcel remain borrowed.
//
// Example: decode a GraphicBuffer while preserving its parcel's FD semantics.
func ReadGraphicBuffer(p *parcel.Parcel) (*Buffer, error) {
	return readGraphicBuffer(p, false)
}

// ReadOwnedGraphicBuffer returns a buffer whose descriptors the caller owns.
// Received descriptors transfer from the parcel; borrowed descriptors are
// duplicated with close-on-exec.
//
// Example: decode a capture result that can outlive its source parcel.
func ReadOwnedGraphicBuffer(p *parcel.Parcel) (*Buffer, error) {
	return readGraphicBuffer(p, true)
}

// readGraphicBuffer decodes a native buffer with the requested FD ownership.
//
// Example: an owned decode duplicates borrowed FDs before returning.
func readGraphicBuffer(
	p *parcel.Parcel,
	owned bool,
) (*Buffer, error) {
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

	if numFds < 0 || numInts < 0 || numFds != fdCount {
		return nil, fmt.Errorf("fd count mismatch: header=%d, outer=%d", numFds, fdCount)
	}
	if int64(numInts)*4 != int64(len(raw)-52) {
		return nil, fmt.Errorf("native handle ints %d do not fit flattened size %d", numInts, len(raw))
	}

	// Read native_handle ints from the flattened data (after 13-word header).
	ints := make([]int32, numInts)
	for i := range ints {
		off := 52 + i*4
		ints[i] = int32(binary.LittleEndian.Uint32(raw[off : off+4]))
	}

	// Read FDs from parcel (as flat_binder_objects).
	fds := make([]int32, numFds)
	var acquired []int32
	for i := range fds {
		var fd int32
		var transferred bool
		if owned {
			fd, err = p.ReadOwnedFileDescriptor()
			transferred = err == nil
		} else {
			fd, transferred, err = p.ReadFileDescriptorWithOwnership()
		}
		if err != nil {
			return nil, errors.Join(fmt.Errorf("reading fd %d: %w", i, err), closeGraphicBufferFDs(acquired))
		}
		fds[i] = fd
		if transferred {
			acquired = append(acquired, fd)
		}
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

// closeGraphicBufferFDs releases descriptors acquired during a failed decode.
//
// Example: preserve every close error when a later descriptor is missing.
func closeGraphicBufferFDs(fds []int32) error {
	var errs []error
	for _, fd := range fds {
		if err := unix.Close(int(fd)); err != nil {
			errs = append(errs, fmt.Errorf("closing partially read fd: %w", err))
		}
	}
	return errors.Join(errs...)
}
