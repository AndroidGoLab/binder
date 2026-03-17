//go:build linux

package kernelbinder

import (
	"context"
	"encoding/binary"
	"fmt"
)

// freeBuffer sends BC_FREE_BUFFER to release an mmap'd buffer.
func (d *Driver) freeBuffer(
	ctx context.Context,
	bufferAddr uint64,
) error {
	// Use a fixed-size array so the compiler can stack-allocate it,
	// avoiding a per-call heap allocation.
	var bufArr [freeBufferBufSize]byte
	buf := bufArr[:]
	binary.LittleEndian.PutUint32(buf[0:4], bcFreeBuffer)
	binary.LittleEndian.PutUint64(buf[4:12], bufferAddr)

	err := d.writeCommand(ctx, buf)
	if err != nil {
		return fmt.Errorf("freeBuffer: %w", err)
	}

	return nil
}
