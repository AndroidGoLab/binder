package gralloc

import (
	"encoding/binary"
	"errors"
	"testing"
)

type testNativeBuffer struct {
	locked      bool
	unlockFails bool
	closed      bool
}

func (n *testNativeBuffer) copyInto([]byte) error {
	n.locked = true
	if n.unlockFails {
		return errors.New("unlock failed")
	}
	n.locked = false
	return nil
}
func (n *testNativeBuffer) finish() error {
	if n.locked && n.unlockFails {
		return errors.New("unlock failed")
	}
	n.locked = false
	return nil
}
func (n *testNativeBuffer) close() error {
	if err := n.finish(); err != nil {
		return err
	}
	n.closed = true
	return nil
}
func (n *testNativeBuffer) flattened() []byte { return nil }
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
