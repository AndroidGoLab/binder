package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
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
	p.WriteInt32(61)
	p.WriteInt32(1)
	payload := make([]byte, 61)
	payload[45] = 1
	p.WriteRawBytes(payload)
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
	noFence.WriteInt32(61)
	noFence.WriteInt32(0)
	noFence.WriteRawBytes(make([]byte, 61))
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
	data.WriteInt32(61)
	data.WriteInt32(1)
	payload := make([]byte, 61)
	payload[45] = 1
	data.WriteRawBytes(payload)
	data.WriteFileDescriptor(int32(fds[0]))
	if _, err := p.OnTransaction(context.Background(), igbp.QueueBuffer, data); err != nil {
		t.Fatal(err)
	}
	if _, err := unix.FcntlInt(uintptr(fds[0]), unix.F_GETFD, 0); err != unix.EBADF {
		t.Fatalf("late fence remains open: %v", err)
	}
}

func TestQueueFenceRejectsMalformedDescriptor(t *testing.T) {
	p := parcel.New()
	p.WriteInt32(0)
	p.WriteInt32(61)
	p.WriteInt32(1)
	payload := make([]byte, 61)
	payload[45] = 1
	p.WriteRawBytes(payload)
	_, fd, err := readQueueFence(p)
	if err == nil || fd != -1 {
		t.Fatalf("fd=%d err=%v; want error and -1", fd, err)
	}
}

func TestQueueFenceChecksNestedCount(t *testing.T) {
	var fds [2]int
	if err := unix.Pipe(fds[:]); err != nil {
		t.Fatal(err)
	}
	defer unix.Close(fds[1])
	p := parcel.New()
	p.WriteInt32(0)
	p.WriteInt32(61)
	p.WriteInt32(1)
	p.WriteRawBytes(make([]byte, 61))
	p.WriteFileDescriptor(int32(fds[0]))
	if _, _, err := readQueueFence(p); err == nil {
		t.Fatal("mismatched nested fence count accepted")
	}
	if _, err := unix.FcntlInt(uintptr(fds[0]), unix.F_GETFD, 0); err != unix.EBADF {
		t.Fatalf("fence remains open: %v", err)
	}
}

func TestCancelSignalsCaptureFailure(t *testing.T) {
	var fds [2]int
	if err := unix.Pipe(fds[:]); err != nil {
		t.Fatal(err)
	}
	defer unix.Close(fds[1])
	p := &producer{ProducerStub: igbp.NewProducerStub(width, height, [4]*gralloc.Buffer{}), Result: make(chan frameResult, 1)}
	data := parcel.New()
	data.WriteInterfaceToken(igbp.Descriptor)
	data.WriteInt32(0)
	data.WriteInt32(4)
	data.WriteInt32(1)
	data.WriteInt32(1)
	data.WriteFileDescriptor(int32(fds[0]))
	if _, err := p.OnTransaction(context.Background(), igbp.CancelBuffer, data); err != nil {
		t.Fatal(err)
	}
	select {
	case r := <-p.Result:
		if r.Err == nil || r.Pixels != nil {
			t.Fatalf("result=%+v", r)
		}
	default:
		t.Fatal("cancel did not signal capture failure")
	}
	if _, err := unix.FcntlInt(uintptr(fds[0]), unix.F_GETFD, 0); err != unix.EBADF {
		t.Fatalf("fence remains open: %v", err)
	}
}

func TestRejectOversizedDimensions(t *testing.T) {
	if ^uint(0)>>32 == 0 {
		t.Skip("int cannot represent oversized dimension")
	}
	_, err := run(context.Background(), int(int64(1)<<31), 1)
	if err == nil || !strings.Contains(err.Error(), "dimensions") {
		t.Fatalf("got %v; want dimension error", err)
	}
}

func TestRejectedDequeueIncludesTimestamps(t *testing.T) {
	p := &producer{ProducerStub: igbp.NewProducerStub(width, height, [4]*gralloc.Buffer{})}
	p.Dequeued.Store(true)
	data := parcel.New()
	data.WriteInterfaceToken(igbp.Descriptor)
	data.WriteUint32(width)
	data.WriteUint32(height)
	data.WriteInt32(rgba8888)
	data.WriteUint64(pixelUsage)
	data.WriteBool(true)
	reply, err := p.OnTransaction(context.Background(), igbp.DequeueBuffer, data)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reply.Read(24); err != nil {
		t.Fatal(err)
	}
	size, err := reply.ReadInt32()
	if err != nil || size != 28 {
		t.Fatalf("timestamps size=%d err=%v", size, err)
	}
	if _, err := reply.Read(32); err != nil {
		t.Fatal(err)
	}
	status, err := reply.ReadInt32()
	if err != nil || status != -11 {
		t.Fatalf("status=%d err=%v", status, err)
	}
}

func TestMalformedDequeueDoesNotConsumeSlot(t *testing.T) {
	p := &producer{ProducerStub: igbp.NewProducerStub(width, height, [4]*gralloc.Buffer{})}
	malformed := parcel.New()
	malformed.WriteInterfaceToken(igbp.Descriptor)
	if _, err := p.OnTransaction(context.Background(), igbp.DequeueBuffer, malformed); err == nil {
		t.Fatal("malformed dequeue accepted")
	}
	if p.Dequeued.Load() {
		t.Fatal("malformed dequeue consumed slot")
	}
}

func TestRejectsWrongInterfaceToken(t *testing.T) {
	p := &producer{ProducerStub: igbp.NewProducerStub(width, height, [4]*gralloc.Buffer{})}
	data := parcel.New()
	data.WriteInterfaceToken("wrong")
	data.WriteUint32(width)
	data.WriteUint32(height)
	data.WriteInt32(rgba8888)
	data.WriteUint64(pixelUsage)
	data.WriteBool(false)
	if _, err := p.OnTransaction(context.Background(), igbp.DequeueBuffer, data); err == nil {
		t.Fatal("wrong token accepted")
	}
	if p.Dequeued.Load() {
		t.Fatal("wrong token consumed slot")
	}
}

func TestRequestInvalidSlot(t *testing.T) {
	p := &producer{ProducerStub: igbp.NewProducerStub(width, height, [4]*gralloc.Buffer{})}
	data := parcel.New()
	data.WriteInterfaceToken(igbp.Descriptor)
	data.WriteInt32(999)
	if _, err := p.OnTransaction(context.Background(), igbp.RequestBuffer, data); err == nil {
		t.Fatal("invalid slot accepted")
	}
}

func TestCloseFDsOnPartialAllocation(t *testing.T) {
	var fds [2]int
	if err := unix.Pipe(fds[:]); err != nil {
		t.Fatal(err)
	}
	defer unix.Close(fds[1])
	p := parcel.New()
	outer := parcel.WriteParcelableHeader(p)
	p.WriteInt32(width)
	p.WriteInt32(1)
	p.WriteInt32(1)
	handle := parcel.WriteParcelableHeader(p)
	p.WriteInt32(1)
	p.WriteInt32(1)
	p.WriteInt32(0)
	p.WriteFileDescriptor(int32(fds[0]))
	parcel.WriteParcelableFooter(p, handle)
	parcel.WriteParcelableFooter(p, outer)
	var owned []int32
	if _, err := readAllocation(p, &owned); err == nil {
		t.Fatal("truncated allocation accepted")
	}
	if err := closeFDs(owned); err != nil {
		t.Fatal(err)
	}
	if _, err := unix.FcntlInt(uintptr(fds[0]), unix.F_GETFD, 0); err != unix.EBADF {
		t.Fatalf("fd still open: %v", err)
	}
}

func TestMalformedLateFencePreservesStdin(t *testing.T) {
	if os.Getenv("BINDER_TEST_STDIN_CHILD") == "1" {
		p := &producer{ProducerStub: igbp.NewProducerStub(width, height, [4]*gralloc.Buffer{}), Closed: true}
		data := parcel.New()
		data.WriteInterfaceToken(igbp.Descriptor)
		data.WriteInt32(0)
		data.WriteInt32(queueMinimumSize)
		data.WriteInt32(1)
		payload := make([]byte, queueMinimumSize)
		payload[queueFenceOffset] = 1
		data.WriteRawBytes(payload)
		if _, err := p.OnTransaction(context.Background(), igbp.QueueBuffer, data); err != nil {
			t.Fatal(err)
		}
		if _, err := unix.FcntlInt(0, unix.F_GETFD, 0); err != nil {
			t.Fatalf("stdin was closed: %v", err)
		}
		return
	}
	input, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	cmd := exec.Command(os.Args[0], "-test.run=^TestMalformedLateFencePreservesStdin$")
	cmd.Env = append(os.Environ(), "BINDER_TEST_STDIN_CHILD=1")
	cmd.Stdin = input
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("child: %v\n%s", err, output)
	}
}

func TestCancelWithoutFenceSignalsFailure(t *testing.T) {
	p := &producer{ProducerStub: igbp.NewProducerStub(width, height, [4]*gralloc.Buffer{}), Result: make(chan frameResult, 1)}
	data := parcel.New()
	data.WriteInterfaceToken(igbp.Descriptor)
	data.WriteInt32(0)
	data.WriteInt32(4)
	data.WriteInt32(0)
	data.WriteInt32(0)
	if _, err := p.OnTransaction(context.Background(), igbp.CancelBuffer, data); err != nil {
		t.Fatal(err)
	}
	select {
	case r := <-p.Result:
		if r.Err == nil || r.Pixels != nil {
			t.Fatalf("result=%+v", r)
		}
	default:
		t.Fatal("cancel did not signal failure")
	}
}

func TestSuccessfulDequeueIncludesTimestamps(t *testing.T) {
	p := &producer{ProducerStub: igbp.NewProducerStub(width, height, [4]*gralloc.Buffer{})}
	data := parcel.New()
	data.WriteInterfaceToken(igbp.Descriptor)
	data.WriteUint32(width)
	data.WriteUint32(height)
	data.WriteInt32(rgba8888)
	data.WriteUint64(pixelUsage)
	data.WriteBool(true)
	reply, err := p.OnTransaction(context.Background(), igbp.DequeueBuffer, data)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reply.Read(24); err != nil {
		t.Fatal(err)
	}
	size, err := reply.ReadInt32()
	if err != nil || size != 28 {
		t.Fatalf("size=%d err=%v", size, err)
	}
	if _, err := reply.Read(32); err != nil {
		t.Fatal(err)
	}
	status, err := reply.ReadInt32()
	if err != nil || status != int32(igbp.BufferNeedsRealloc) {
		t.Fatalf("status=%d err=%v", status, err)
	}
}
