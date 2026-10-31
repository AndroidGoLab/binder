package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/AndroidGoLab/binder/gralloc"
	"github.com/AndroidGoLab/binder/igbp"
	"github.com/AndroidGoLab/binder/parcel"
	"golang.org/x/sys/unix"
)

// TestPackedRGBA verifies that row padding does not leak into the output.
//
// Example: a two-pixel image may have a three-pixel buffer stride.
func TestPackedRGBA(t *testing.T) {
	got, err := packedRGBA([]byte{1, 2, 3, 4, 5, 6, 7, 8, 99, 99, 99, 99, 9, 10, 11, 12, 13, 14, 15, 16, 99, 99, 99, 99}, 2, 2, 3)
	if err != nil || !bytes.Equal(got, []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}) {
		t.Fatalf("got %v, %v", got, err)
	}
	if _, err = packedRGBA([]byte{1}, 2, 2, 3); err == nil {
		t.Fatal("short buffer accepted")
	}
}

// TestQueueFence checks both valid and malformed flattenable fence data.
//
// Example: a queued frame can carry one synchronization fence.
func TestQueueFence(t *testing.T) {
	p := parcel.New()
	p.WriteInt32(0)
	p.WriteInt32(4)
	p.WriteInt32(1)
	p.WriteInt32(0)
	p.WriteFileDescriptor(42)
	slot, fd, err := readQueueFence(p)
	if err != nil || slot != 0 || fd != 42 {
		t.Fatalf("slot=%d fd=%d err=%v", slot, fd, err)
	}
	bad := parcel.New()
	bad.WriteInt32(0)
	bad.WriteInt32(4)
	bad.WriteInt32(2)
	bad.WriteInt32(0)
	if _, _, err := readQueueFence(bad); err == nil {
		t.Fatal("invalid fence count accepted")
	}
	noFence := parcel.New()
	noFence.WriteInt32(0)
	noFence.WriteInt32(4)
	noFence.WriteInt32(0)
	noFence.WriteInt32(0)
	if slot, fd, err := readQueueFence(noFence); err != nil || slot != 0 || fd != -1 {
		t.Fatalf("missing fence: slot=%d fd=%d err=%v", slot, fd, err)
	}
}

// TestCopyFrameRejectsInvalidFence verifies that a failed fence cannot yield pixels.
//
// Example: a closed fence descriptor fails before buffer access.
func TestCopyFrameRejectsInvalidFence(t *testing.T) {
	if pixels, err := copyFrame(&gralloc.Buffer{}, 999999); err == nil || pixels != nil {
		t.Fatalf("got pixels=%v error=%v", pixels, err)
	}
}

// TestClosedProducerClosesFence checks late callbacks after teardown.
//
// Example: a queued buffer arriving during shutdown must not access its buffer.
func TestClosedProducerClosesFence(t *testing.T) {
	var fds [2]int
	if err := unix.Pipe(fds[:]); err != nil {
		t.Fatal(err)
	}
	defer unix.Close(fds[1])
	p := &producer{ProducerStub: igbp.NewProducerStub(width, height, [4]*gralloc.Buffer{}), Closed: true}
	data := parcel.New()
	data.WriteInterfaceToken(igbp.Descriptor)
	data.WriteInt32(0)
	data.WriteInt32(4)
	data.WriteInt32(1)
	data.WriteInt32(0)
	data.WriteFileDescriptor(int32(fds[0]))
	if _, err := p.OnTransaction(context.Background(), igbp.QueueBuffer, data); err != nil {
		t.Fatal(err)
	}
	if _, err := unix.FcntlInt(uintptr(fds[0]), unix.F_GETFD, 0); err != unix.EBADF {
		t.Fatalf("late fence remains open: %v", err)
	}
}
