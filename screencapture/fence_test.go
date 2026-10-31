package screencapture

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// TestWaitFence handles a signaled fence and cancellation without leaking descriptors.
//
// Example: a ready fence succeeds and a canceled wait returns the context error.
func TestWaitFence(t *testing.T) {
	fds := []int{0, 0}
	require.NoError(t, unix.Pipe(fds))
	defer unix.Close(fds[0])
	defer unix.Close(fds[1])
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, waitFence(ctx, int32(fds[0])), context.Canceled)
	_, err := unix.Write(fds[1], []byte{1})
	require.NoError(t, err)
	require.NoError(t, waitFence(context.Background(), int32(fds[0])))
}
