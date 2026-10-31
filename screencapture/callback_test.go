package screencapture

import (
	"context"
	"testing"

	"github.com/AndroidGoLab/binder/android/hardware/common"
	"github.com/AndroidGoLab/binder/gralloc"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// TestAbandonedCaptureClosesLateFrame verifies that a timed out request cannot leak a later frame.
//
// Example: a buffer delivered after cancellation is closed immediately.
func TestAbandonedCaptureClosesLateFrame(t *testing.T) {
	fds := []int{0, 0}
	require.NoError(t, unix.Pipe(fds))
	defer unix.Close(fds[1])
	callback := newCaptureCallback()
	callback.abandon()
	callback.complete(&nativeFrame{Buffer: &gralloc.Buffer{Handle: common.NativeHandle{Fds: []int32{int32(fds[0])}}}}, nil)
	_, err := unix.FcntlInt(uintptr(fds[0]), unix.F_GETFD, 0)
	require.ErrorIs(t, err, unix.EBADF)
}

// TestAbandonDrainsCompletedFrame covers completion racing with cancellation.
//
// Example: a queued frame is released if its caller stops waiting.
func TestAbandonDrainsCompletedFrame(t *testing.T) {
	fds := []int{0, 0}
	require.NoError(t, unix.Pipe(fds))
	defer unix.Close(fds[1])
	callback := newCaptureCallback()
	callback.complete(&nativeFrame{Buffer: &gralloc.Buffer{Handle: common.NativeHandle{Fds: []int32{int32(fds[0])}}}}, nil)
	callback.abandon()
	_, err := unix.FcntlInt(uintptr(fds[0]), unix.F_GETFD, 0)
	require.ErrorIs(t, err, unix.EBADF)
}

// TestFinishOnlyReleasesOneSlot protects against a callback racing with a transport failure.
//
// Example: completing a callback twice does not release the next request.
func TestFinishOnlyReleasesOneSlot(t *testing.T) {
	c := &Capture{pending: make(chan struct{}, 1)}
	require.NoError(t, c.begin())
	cb := newCaptureCallback()
	cb.owner = c
	cb.finish(context.Background())
	require.NoError(t, c.begin())
	cb.finish(context.Background())
	require.ErrorIs(t, c.begin(), ErrCapturePending)
	c.end()
}
