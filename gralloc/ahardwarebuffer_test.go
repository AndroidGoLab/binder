package gralloc

import (
	"encoding/binary"
	"errors"
	"testing"
)

// testNativeBuffer simulates failed unlock for shared-owner tests.
//
// Example: keep a buffer locked until a later retry succeeds.
type testNativeBuffer struct {
	locked      bool
	unlockFails bool
	closed      bool
}

// copyInto simulates a CPU read followed by unlock.
func (n *testNativeBuffer) copyInto([]byte) error {
	n.locked = true
	if n.unlockFails {
		return errors.New("unlock failed")
	}
	n.locked = false
	return nil
}

// finish simulates retrying an outstanding unlock.
func (n *testNativeBuffer) finish() error {
	if n.locked && n.unlockFails {
		return errors.New("unlock failed")
	}
	n.locked = false
	return nil
}

// close refuses to release a buffer whose unlock still fails.
func (n *testNativeBuffer) close() error {
	if err := n.finish(); err != nil {
		return err
	}
	n.closed = true
	return nil
}

// flattened supplies empty test metadata.
func (n *testNativeBuffer) flattened() []byte { return nil }

// TestNativeBufferOwnership verifies failed unlock blocks release across copies.
func TestNativeBufferOwnership(t *testing.T) {
	native := &testNativeBuffer{unlockFails: true}
	b := &Buffer{native: native}
	copyOfBuffer := *b
	if err := CopyRGBAInto(b, make([]byte, 4)); err == nil {
		t.Fatal("failed unlock must be returned")
	}
	if err := copyOfBuffer.FinishCPUAccess(); err == nil {
		t.Fatal("failed unlock must prevent release")
	}
	if err := b.Close(); err == nil || native.closed {
		t.Fatal("locked allocation was closed")
	}
	native.unlockFails = false
	if err := copyOfBuffer.FinishCPUAccess(); err != nil {
		t.Fatal(err)
	}
	if err := b.Close(); err != nil || !native.closed {
		t.Fatal("unlocked allocation not closed", err)
	}
}

// TestFlattenedMetadataValidation checks native metadata counts and payload.
func TestFlattenedMetadataValidation(t *testing.T) {
	d := ahbMetadata{width: 16, height: 8, layers: 1, format: 1, usage: 0xb03, stride: 32}
	raw := make([]byte, 56)
	words := []uint32{0x47423031, 16, 8, 32, 1, 1, 0xb03, 123, 456, 7, 1, 1, 0, 999}
	for i, v := range words {
		binary.LittleEndian.PutUint32(raw[i*4:], v)
	}
	ints, err := validateAHBMetadata(raw, d, 1)
	if err != nil || len(ints) != 1 || ints[0] != 999 {
		t.Fatal(ints, err)
	}
	binary.LittleEndian.PutUint32(raw[40:], 2)
	if _, err := validateAHBMetadata(raw, d, 1); err == nil {
		t.Fatal("accepted mismatched fd count")
	}
}
