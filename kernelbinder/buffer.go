//go:build linux

package kernelbinder

import (
	"context"
	"encoding/binary"
	"fmt"

	"github.com/facebookincubator/go-belt/tool/logger"
)

// freeBuffer sends BC_FREE_BUFFER to release an mmap'd buffer.
func (d *Driver) freeBuffer(
	ctx context.Context,
	bufferAddr uint64,
) (_err error) {
	logger.Tracef(ctx, "freeBuffer")
	defer func() { logger.Tracef(ctx, "/freeBuffer: %v", _err) }()

	// BC_FREE_BUFFER: uint32 command + uint64 pointer
	buf := make([]byte, 4+8)
	binary.LittleEndian.PutUint32(buf[0:4], bcFreeBuffer)
	binary.LittleEndian.PutUint64(buf[4:12], bufferAddr)

	err := d.writeCommand(ctx, buf)
	if err != nil {
		return fmt.Errorf("freeBuffer: %w", err)
	}

	return nil
}
