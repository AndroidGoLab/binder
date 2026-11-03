package gralloc

import (
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
)

type nativeBuffer interface {
	copyInto([]byte) error
	finish() error
	close() error
	flattened() []byte
}

// FlattenedGraphicBuffer returns the original serialized metadata, when available.
// The buffer's descriptors must remain open while the result is sent.
func (b *Buffer) FlattenedGraphicBuffer() ([]byte, bool) {
	if b == nil || b.native == nil {
		return nil, false
	}
	return b.native.flattened(), true
}

// FinishCPUAccess completes any outstanding CPU access before buffer reuse.
func (b *Buffer) FinishCPUAccess() error {
	if b == nil || b.native == nil {
		return nil
	}
	return b.native.finish()
}

// Close releases the buffer after its producer and all leases have stopped.
func (b *Buffer) Close() error {
	if b == nil {
		return nil
	}
	if b.native != nil {
		if err := b.native.close(); err != nil {
			return err
		}
		b.Handle.Fds = nil
		return nil
	}
	var errs []error
	for _, fd := range b.Handle.Fds {
		if fd >= 0 {
			if err := unix.Close(int(fd)); err != nil {
				errs = append(errs, fmt.Errorf("close buffer descriptor: %w", err))
			}
		}
	}
	b.Handle.Fds = nil
	return errors.Join(errs...)
}
