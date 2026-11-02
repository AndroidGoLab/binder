//go:build linux

package kernelbinder

import (
	"encoding/binary"
	"fmt"

	"golang.org/x/sys/unix"
)

// binder_buffer_object offsets (64-bit):
//
//	[0:4]   type (uint32)
//	[4:8]   flags (uint32)
//	[8:16]  buffer (binder_uintptr_t / __u64)
//	[16:24] length (binder_size_t / __u64)
//	[24:32] parent (binder_size_t / __u64)
//	[32:40] parent_offset (binder_size_t / __u64)
const binderBufferObjectSize = 40

// binder_fd_array_object offsets (64-bit):
//
//	[0:4]   type (uint32)
//	[4:8]   pad (uint32)
//	[8:16]  num_fds (binder_size_t / __u64)
//	[16:24] parent (binder_size_t / __u64)
//	[24:32] parent_offset (binder_size_t / __u64)
const binderFDArrayObjectSize = 32

// calcScatterGatherBuffersSize scans the parcel objects for
// BINDER_TYPE_PTR entries and returns the total size of their
// referenced buffers (8-byte aligned). Returns 0 if no PTR objects exist.
func calcScatterGatherBuffersSize(
	data []byte,
	objects []uint64,
) uint64 {
	var total uint64
	for _, objOffset := range objects {
		if objOffset+binderBufferObjectSize > uint64(len(data)) {
			continue
		}
		objType := binderObjectType(binary.LittleEndian.Uint32(data[objOffset:]))
		if objType != binderTypePTR {
			continue
		}
		bufLen := binary.LittleEndian.Uint64(data[objOffset+16:])
		// Align each buffer to 8 bytes (kernel alignment for SG buffers).
		total += (bufLen + 7) &^ 7
	}
	return total
}

// resolveScatterGather scans the reply data's offsets for BINDER_TYPE_PTR
// objects, copies each referenced buffer from the mmap'd region, and
// appends the data to replyData. The buffer pointers in the PTR objects
// are patched to record the offset within the extended replyData where
// each buffer's content was placed.
//
// For BINDER_TYPE_FDA (fd array) objects, the referenced file descriptors
// in the parent buffer are dup'd before they are closed by freeBuffer.
// The kernel closes FDA fds during binder_transaction_buffer_release.
//
// Must be called BEFORE freeBuffer, since the mmap'd data is released
// by freeBuffer.
func (d *Driver) resolveScatterGather(
	replyData []byte,
	offsetsAddr uint64,
	offsetsSize uint64,
) (result []byte, fdOffsets []int, err error) {
	if offsetsSize == 0 {
		return replyData, nil, nil
	}
	if offsetsSize%8 != 0 {
		return replyData, nil, fmt.Errorf("resolveScatterGather: invalid offsets size %d", offsetsSize)
	}

	numOffsets := int(offsetsSize / 8)
	offsetsBuf, err := d.copyFromMapped(offsetsAddr, offsetsSize)
	if err != nil {
		return replyData, nil, fmt.Errorf("resolveScatterGather: copying offsets: %w", err)
	}

	// First pass: check if any BINDER_TYPE_PTR objects exist.
	hasPTR := false
	for i := range numOffsets {
		objOffset := binary.LittleEndian.Uint64(offsetsBuf[i*8:])
		if objOffset > uint64(len(replyData)) || uint64(len(replyData))-objOffset < 4 {
			continue
		}
		objType := binderObjectType(binary.LittleEndian.Uint32(replyData[objOffset:]))
		if objType == binderTypePTR {
			hasPTR = true
			break
		}
	}

	if !hasPTR {
		return replyData, nil, nil
	}

	// Second pass: copy each PTR buffer from mmap and append to replyData.
	// Track resolved buffer locations for FDA processing.
	extended := make([]byte, len(replyData), len(replyData)+4096)
	copy(extended, replyData)

	// ptrBufferOffsets maps objects-array index to the offset in extended
	// where the PTR buffer data was placed.
	type bufferLocation struct{ offset, length uint64 }
	ptrBufferOffsets := make(map[int]bufferLocation)
	var duplicated []int
	defer func() {
		if err != nil {
			for _, fd := range duplicated {
				_ = unix.Close(fd)
			}
		}
	}()

	for i := range numOffsets {
		objOffset := binary.LittleEndian.Uint64(offsetsBuf[i*8:])
		if objOffset > uint64(len(replyData)) || uint64(len(replyData))-objOffset < 4 {
			continue
		}

		objType := binderObjectType(binary.LittleEndian.Uint32(extended[objOffset:]))

		switch objType {
		case binderTypePTR:
			if uint64(len(replyData))-objOffset < binderBufferObjectSize {
				continue
			}

			bufPtr := binary.LittleEndian.Uint64(extended[objOffset+8:])
			bufLen := binary.LittleEndian.Uint64(extended[objOffset+16:])

			if bufLen == 0 {
				ptrBufferOffsets[i] = bufferLocation{uint64(len(extended)), 0}
				continue
			}

			bufData, copyErr := d.copyFromMapped(bufPtr, bufLen)
			if copyErr != nil {
				return replyData, nil, fmt.Errorf("resolveScatterGather: copying buffer %d (ptr=0x%x len=%d): %w",
					i, bufPtr, bufLen, copyErr)
			}

			newOffset := uint64(len(extended))
			extended = append(extended, bufData...)

			// Align to 8 bytes.
			for len(extended)%8 != 0 {
				extended = append(extended, 0)
			}

			// Patch the buffer pointer.
			binary.LittleEndian.PutUint64(extended[objOffset+8:], newOffset)
			ptrBufferOffsets[i] = bufferLocation{newOffset, bufLen}

		case binderTypeFDA:
			if uint64(len(replyData))-objOffset < binderFDArrayObjectSize {
				continue
			}

			numFds := binary.LittleEndian.Uint64(extended[objOffset+8:])
			parentIdx := binary.LittleEndian.Uint64(extended[objOffset+16:])
			parentOff := binary.LittleEndian.Uint64(extended[objOffset+24:])

			// Find the parent buffer's data in our extended array.
			parent, ok := ptrBufferOffsets[int(parentIdx)]
			if parentIdx >= uint64(numOffsets) {
				ok = false
			}
			if !ok {
				return replyData, nil, fmt.Errorf("resolveScatterGather: invalid FDA parent %d", parentIdx)
			}

			if parentOff > parent.length || numFds > (parent.length-parentOff)/4 {
				return replyData, nil, fmt.Errorf("resolveScatterGather: FDA exceeds parent buffer")
			}
			fdOffset := parent.offset + parentOff
			for j := uint64(0); j < numFds; j++ {
				fdPos := fdOffset + j*4
				if fdPos+4 > uint64(len(extended)) {
					return replyData, nil, fmt.Errorf("resolveScatterGather: FDA exceeds buffer")
				}
				oldFD := int(int32(binary.LittleEndian.Uint32(extended[fdPos:])))

				// Dup the fd before freeBuffer closes it.
				// The kernel calls close() on FDA fds during
				// binder_transaction_buffer_release.
				newFD, dupErr := unix.Dup(oldFD)
				if dupErr != nil {
					return replyData, nil, fmt.Errorf("resolveScatterGather: duplicate fd: %w", dupErr)
				}
				duplicated = append(duplicated, newFD)
				fdOffsets = append(fdOffsets, int(fdPos))

				// Replace the fd in the buffer data with the dup'd fd.
				binary.LittleEndian.PutUint32(extended[fdPos:], uint32(int32(newFD)))
			}
		}
	}

	return extended, fdOffsets, nil
}
