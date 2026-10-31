package gralloc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// observedCancelContext reports when the fence waiter registers cancellation.
//
// Example: a test can cancel only after the wake pipe is connected.
type observedCancelContext struct {
	context.Context
	registered chan struct{}
}

// Value prevents context from bypassing the observable AfterFunc method.
//
// Example: registration is forwarded through AfterFunc below.
func (*observedCancelContext) Value(key any) any { return nil }

// AfterFunc registers a cancellation callback and announces the registration.
//
// Example: a test waits until cancellation can wake the poll.
func (c *observedCancelContext) AfterFunc(f func()) func() bool {
	stop := context.AfterFunc(c.Context, f)
	close(c.registered)
	return stop
}

// TestWaitFence checks success, invalid descriptors and caller ownership.
//
// Example: a signaled fence succeeds without being closed by the wait.
func TestWaitFence(t *testing.T) {
	fds := []int{0, 0}
	require.NoError(t, unix.Pipe(fds))
	defer unix.Close(fds[0])
	defer unix.Close(fds[1])
	require.NoError(t, WaitFence(context.Background(), -1))
	require.Error(t, WaitFence(context.Background(), -2))
	_, err := unix.Write(fds[1], []byte{1})
	require.NoError(t, err)
	require.NoError(t, WaitFence(context.Background(), int32(fds[0])))
	_, err = unix.FcntlInt(uintptr(fds[0]), unix.F_GETFD, 0)
	require.NoError(t, err)
}

// TestWaitFenceCancellation checks that cancellation stops a pending wait.
//
// Example: cancel an unsignaled fence without closing the caller's descriptor.
func TestWaitFenceCancellation(t *testing.T) {
	fds := []int{0, 0}
	require.NoError(t, unix.Pipe(fds))
	defer unix.Close(fds[0])
	defer unix.Close(fds[1])
	base, cancel := context.WithCancel(context.Background())
	ctx := &observedCancelContext{Context: base, registered: make(chan struct{})}
	result := make(chan error, 1)
	go // Wait on an unsignaled descriptor until canceled.
	//
	// Example: a caller can stop waiting without signaling the fence.
	func() { result <- WaitFence(ctx, int32(fds[0])) }()
	<-ctx.registered
	cancel()
	require.ErrorIs(t, <-result, context.Canceled)
	_, err := unix.FcntlInt(uintptr(fds[0]), unix.F_GETFD, 0)
	require.NoError(t, err)
}

// TestWaitFenceHangup checks that a closed writer is not mistaken for a signal.
//
// Example: a broken fence yields an error.
func TestWaitFenceHangup(t *testing.T) {
	fds := []int{0, 0}
	require.NoError(t, unix.Pipe(fds))
	defer unix.Close(fds[0])
	require.NoError(t, unix.Close(fds[1]))
	require.Error(t, WaitFence(context.Background(), int32(fds[0])))
}

// TestWaitFenceClosedFD rejects an invalid descriptor.
//
// Example: a previously closed fence must not report successful completion.
func TestWaitFenceClosedFD(t *testing.T) {
	fds := []int{0, 0}
	require.NoError(t, unix.Pipe(fds))
	defer unix.Close(fds[1])
	require.NoError(t, unix.Close(fds[0]))
	require.Error(t, WaitFence(context.Background(), int32(fds[0])))
}
