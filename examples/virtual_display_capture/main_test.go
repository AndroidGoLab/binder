package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/AndroidGoLab/binder/gralloc"
	"github.com/AndroidGoLab/binder/igbp"
	"github.com/AndroidGoLab/binder/parcel"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// cleanupFD arranges to close a test-owned descriptor and check the result.
//
// Example: the write end of a pipe closes after its test.
func cleanupFD(
	t *testing.T,
	fd int,
) {
	t.Helper()
	t.Cleanup( // Close the descriptor after the test finishes.
		//
		// Example: a failed close is reported to the test.
		func() { require.NoError(t, unix.Close(fd)) },
	)
}

// TestPackedRGBA verifies that row padding does not leak into the output.
//
// Example: a two-pixel image may have a three-pixel buffer stride.
func TestPackedRGBA(t *testing.T) {
	got, err := packedRGBA([]byte{1, 2, 3, 4, 5, 6, 7, 8, 99, 99, 99, 99, 9, 10, 11, 12, 13, 14, 15, 16, 99, 99, 99, 99}, 2, 2, 3)
	if err != nil || !bytes.Equal(got, []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}) {
		require.FailNowf(t, "assertion failed", "got %v, %v", got, err)
	}
	if _, err = packedRGBA([]byte{1}, 2, 2, 3); err == nil {
		require.FailNow(t, "short buffer accepted")
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
		require.FailNowf(t, "assertion failed", "slot=%d fd=%d err=%v", slot, fd, err)
	}
	bad := parcel.New()
	bad.WriteInt32(0)
	bad.WriteInt32(4)
	bad.WriteInt32(2)
	bad.WriteInt32(0)
	if _, _, err := readQueueFence(bad); err == nil {
		require.FailNow(t, "invalid fence count accepted")
	}
	noFence := parcel.New()
	noFence.WriteInt32(0)
	noFence.WriteInt32(61)
	noFence.WriteInt32(0)
	noFence.WriteRawBytes(make([]byte, 61))
	if slot, fd, err := readQueueFence(noFence); err != nil || slot != 0 || fd != -1 {
		require.FailNowf(t, "assertion failed", "missing fence: slot=%d fd=%d err=%v", slot, fd, err)
	}
}

// TestCopyFrameRejectsInvalidFence verifies that a failed fence cannot yield pixels.
//
// Example: a closed fence descriptor fails before buffer access.
func TestCopyFrameRejectsInvalidFence(t *testing.T) {
	pixels, err := copyFrame(&gralloc.Buffer{}, 999999)
	require.Nil(t, pixels)
	require.ErrorContains(t, err, "acquire fence")
}

// TestClosedProducerClosesFence checks late callbacks after teardown.
//
// Example: a queued buffer arriving during shutdown must not access its buffer.
func TestClosedProducerClosesFence(t *testing.T) {
	var fds [2]int
	require.NoError(t, unix.Pipe(fds[:]))
	cleanupFD(t, fds[1])
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
		require.NoError(t, err)
	}
	if _, err := unix.FcntlInt(uintptr(fds[0]), unix.F_GETFD, 0); err != unix.EBADF {
		require.FailNowf(t, "assertion failed", "late fence remains open: %v", err)
	}
}

// TestQueueFenceRejectsMalformedDescriptor verifies that a missing FD object leaves the descriptor sentinel at -1.
//
// Example: a missing FD object leaves the descriptor sentinel at -1.
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
		require.FailNowf(t, "assertion failed", "fd=%d err=%v; want error and -1", fd, err)
	}
}

// TestQueueFenceChecksNestedCount verifies that mismatched fence counts reject the parcel and close its FD.
//
// Example: mismatched fence counts reject the parcel and close its FD.
func TestQueueFenceChecksNestedCount(t *testing.T) {
	var fds [2]int
	require.NoError(t, unix.Pipe(fds[:]))
	cleanupFD(t, fds[1])
	p := parcel.New()
	p.WriteInt32(0)
	p.WriteInt32(61)
	p.WriteInt32(1)
	p.WriteRawBytes(make([]byte, 61))
	p.WriteFileDescriptor(int32(fds[0]))
	if _, _, err := readQueueFence(p); err == nil {
		require.FailNow(t, "mismatched nested fence count accepted")
	}
	if _, err := unix.FcntlInt(uintptr(fds[0]), unix.F_GETFD, 0); err != unix.EBADF {
		require.FailNowf(t, "assertion failed", "fence remains open: %v", err)
	}
}

// TestCancelSignalsCaptureFailure verifies that canceling a slot closes its fence and reports failure.
//
// Example: canceling a slot closes its fence and reports failure.
func TestCancelSignalsCaptureFailure(t *testing.T) {
	var fds [2]int
	require.NoError(t, unix.Pipe(fds[:]))
	cleanupFD(t, fds[1])
	p := &producer{ProducerStub: igbp.NewProducerStub(width, height, [4]*gralloc.Buffer{}), Result: make(chan frameResult, 1)}
	data := parcel.New()
	data.WriteInterfaceToken(igbp.Descriptor)
	data.WriteInt32(0)
	data.WriteInt32(4)
	data.WriteInt32(1)
	data.WriteInt32(1)
	data.WriteFileDescriptor(int32(fds[0]))
	if _, err := p.OnTransaction(context.Background(), igbp.CancelBuffer, data); err != nil {
		require.NoError(t, err)
	}
	select {
	case r := <-p.Result:
		if r.Err == nil || r.Pixels != nil {
			require.FailNowf(t, "assertion failed", "result=%+v", r)
		}
	default:
		require.FailNow(t, "cancel did not signal capture failure")
	}
	if _, err := unix.FcntlInt(uintptr(fds[0]), unix.F_GETFD, 0); err != unix.EBADF {
		require.FailNowf(t, "assertion failed", "fence remains open: %v", err)
	}
}

// TestRejectOversizedDimensions verifies that a width above MaxInt32 fails before platform access.
//
// Example: a width above MaxInt32 fails before platform access.
func TestRejectOversizedDimensions(t *testing.T) {
	if ^uint(0)>>32 == 0 {
		t.Skip("int cannot represent oversized dimension")
	}
	_, err := run(context.Background(), int(int64(1)<<31), 1)
	if err == nil || !strings.Contains(err.Error(), "dimensions") {
		require.FailNowf(t, "assertion failed", "got %v; want dimension error", err)
	}
}

// TestRejectedDequeueIncludesTimestamps verifies that a rejected dequeue includes requested frame timestamps.
//
// Example: a rejected dequeue includes requested frame timestamps.
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
	require.NoError(t, err)
	if _, err := reply.Read(24); err != nil {
		require.NoError(t, err)
	}
	size, err := reply.ReadInt32()
	if err != nil || size != 28 {
		require.FailNowf(t, "assertion failed", "timestamps size=%d err=%v", size, err)
	}
	if _, err := reply.Read(32); err != nil {
		require.NoError(t, err)
	}
	status, err := reply.ReadInt32()
	if err != nil || status != -11 {
		require.FailNowf(t, "assertion failed", "status=%d err=%v", status, err)
	}
}

// TestMalformedDequeueDoesNotConsumeSlot verifies that a truncated dequeue leaves the slot available.
//
// Example: a truncated dequeue leaves the slot available.
func TestMalformedDequeueDoesNotConsumeSlot(t *testing.T) {
	p := &producer{ProducerStub: igbp.NewProducerStub(width, height, [4]*gralloc.Buffer{})}
	malformed := parcel.New()
	malformed.WriteInterfaceToken(igbp.Descriptor)
	if _, err := p.OnTransaction(context.Background(), igbp.DequeueBuffer, malformed); err == nil {
		require.FailNow(t, "malformed dequeue accepted")
	}
	if p.Dequeued.Load() {
		require.FailNow(t, "malformed dequeue consumed slot")
	}
}

// TestRejectsWrongInterfaceToken verifies that an unrelated Binder token cannot reserve the slot.
//
// Example: an unrelated Binder token cannot reserve the slot.
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
		require.FailNow(t, "wrong token accepted")
	}
	if p.Dequeued.Load() {
		require.FailNow(t, "wrong token consumed slot")
	}
}

// TestRequestInvalidSlot verifies that a request for slot 999 is rejected.
//
// Example: a request for slot 999 is rejected.
func TestRequestInvalidSlot(t *testing.T) {
	p := &producer{ProducerStub: igbp.NewProducerStub(width, height, [4]*gralloc.Buffer{})}
	data := parcel.New()
	data.WriteInterfaceToken(igbp.Descriptor)
	data.WriteInt32(999)
	if _, err := p.OnTransaction(context.Background(), igbp.RequestBuffer, data); err == nil {
		require.FailNow(t, "invalid slot accepted")
	}
}

// TestCloseFDsOnPartialAllocation verifies that a descriptor read before truncated allocation data is closed.
//
// Example: a descriptor read before truncated allocation data is closed.
func TestCloseFDsOnPartialAllocation(t *testing.T) {
	var fds [2]int
	require.NoError(t, unix.Pipe(fds[:]))
	cleanupFD(t, fds[1])
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
		require.FailNow(t, "truncated allocation accepted")
	}
	require.NoError(t, closeFDs(owned))
	if _, err := unix.FcntlInt(uintptr(fds[0]), unix.F_GETFD, 0); err != unix.EBADF {
		require.FailNowf(t, "assertion failed", "fd still open: %v", err)
	}
}

// TestMalformedLateFencePreservesStdin verifies that a missing fence object does not close descriptor zero.
//
// Example: a missing fence object does not close descriptor zero.
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
			require.NoError(t, err)
		}
		if _, err := unix.FcntlInt(0, unix.F_GETFD, 0); err != nil {
			require.FailNowf(t, "assertion failed", "stdin was closed: %v", err)
		}
		return
	}
	input, err := os.Open(os.DevNull)
	require.NoError(t, err)
	t.Cleanup( // Close the subprocess input after the child exits.
		//
		// Example: report a failure to release the test file.
		func() { require.NoError(t, input.Close()) },
	)
	cmd := exec.Command(os.Args[0], "-test.run=^TestMalformedLateFencePreservesStdin$")
	cmd.Env = append(os.Environ(), "BINDER_TEST_STDIN_CHILD=1")
	cmd.Stdin = input
	if output, err := cmd.CombinedOutput(); err != nil {
		require.FailNowf(t, "assertion failed", "child: %v\n%s", err, output)
	}
}

// TestCancelWithoutFenceSignalsFailure verifies that canceling without a fence still unblocks capture.
//
// Example: canceling without a fence still unblocks capture.
func TestCancelWithoutFenceSignalsFailure(t *testing.T) {
	p := &producer{ProducerStub: igbp.NewProducerStub(width, height, [4]*gralloc.Buffer{}), Result: make(chan frameResult, 1)}
	data := parcel.New()
	data.WriteInterfaceToken(igbp.Descriptor)
	data.WriteInt32(0)
	data.WriteInt32(4)
	data.WriteInt32(0)
	data.WriteInt32(0)
	if _, err := p.OnTransaction(context.Background(), igbp.CancelBuffer, data); err != nil {
		require.NoError(t, err)
	}
	select {
	case r := <-p.Result:
		if r.Err == nil || r.Pixels != nil {
			require.FailNowf(t, "assertion failed", "result=%+v", r)
		}
	default:
		require.FailNow(t, "cancel did not signal failure")
	}
}

// TestSuccessfulDequeueIncludesTimestamps verifies that a successful dequeue returns requested timestamps.
//
// Example: a successful dequeue returns requested timestamps.
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
	require.NoError(t, err)
	if _, err := reply.Read(24); err != nil {
		require.NoError(t, err)
	}
	size, err := reply.ReadInt32()
	if err != nil || size != 28 {
		require.FailNowf(t, "assertion failed", "size=%d err=%v", size, err)
	}
	if _, err := reply.Read(32); err != nil {
		require.NoError(t, err)
	}
	status, err := reply.ReadInt32()
	if err != nil || status != int32(igbp.BufferNeedsRealloc) {
		require.FailNowf(t, "assertion failed", "status=%d err=%v", status, err)
	}
}

// TestWaitFenceRetriesInterruptedPoll verifies that an interrupted poll retries with remaining time.
//
// Example: an interrupted poll retries with remaining time.
func TestWaitFenceRetriesInterruptedPoll(t *testing.T) {
	calls := 0
	poll := // Interrupt the first poll and signal the second.
		//
		// Example: a signal interrupts a waiting Binder client.
		func(
			fds []unix.PollFd,
			timeout int,
		) (int, error) {
			calls++
			if calls == 1 {
				return 0, unix.EINTR
			}
			if timeout <= 0 || timeout > fenceTimeoutMS {
				require.FailNowf(t, "assertion failed", "invalid remaining timeout %d", timeout)
			}
			fds[0].Revents = unix.POLLIN
			return 1, nil
		}
	if err := waitFence(1, 5*time.Second, poll); err != nil || calls != 2 {
		require.FailNowf(t, "assertion failed", "calls=%d err=%v", calls, err)
	}
}

// TestWaitFenceRejectsSignaledError verifies that POLLIN with POLLERR is rejected.
//
// Example: POLLIN with POLLERR is rejected.
func TestWaitFenceRejectsSignaledError(t *testing.T) {
	poll := // Simulate an errored fence that also reports readability.
		//
		// Example: POLLIN and POLLERR arrive together.
		func(
			fds []unix.PollFd,
			timeout int,
		) (int, error) {
			fds[0].Revents = unix.POLLIN | unix.POLLERR
			return 1, nil
		}
	if err := waitFence(1, time.Second, poll); err == nil {
		require.FailNow(t, "signaled error accepted")
	}
}

// TestSyncBufferRetries verifies retryable ioctl errors for START and END.
//
// Example: EINTR followed by EAGAIN is retried until success.
func TestSyncBufferRetries(t *testing.T) {
	for _, flags := range []uint64{dmaBufSyncRead, dmaBufSyncRead | dmaBufSyncEnd} {
		calls := 0
		ioctl := // Return two retryable failures before success.
			//
			// Example: EINTR and EAGAIN do not change the requested sync flags.
			func(
				fd int,
				got *uint64,
			) error {
				require.Equal(t, flags, *got)
				calls++
				if calls == 1 {
					return unix.EINTR
				}
				if calls == 2 {
					return unix.EAGAIN
				}
				return nil
			}
		require.NoError(t, syncBuffer(42, flags, ioctl))
		require.Equal(t, 3, calls)
	}
}

// TestSyncBufferTerminalError verifies nonretryable ioctl errors survive.
//
// Example: EBADF is returned after one attempt.
func TestSyncBufferTerminalError(t *testing.T) {
	calls := 0
	ioctl := // Return a nonretryable failure.
		//
		// Example: EBADF stops the retry loop.
		func(
			fd int,
			flags *uint64,
		) error {
			calls++
			return unix.EBADF
		}
	require.ErrorIs(t, syncBuffer(42, dmaBufSyncRead, ioctl), unix.EBADF)
	require.Equal(t, 1, calls)
}
