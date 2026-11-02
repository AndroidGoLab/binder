package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestWriteFrame emits exactly the supplied pixels and propagates short writes.
func TestWriteFrame(t *testing.T) {
	var out bytes.Buffer
	require.NoError(t, writeFrame(&out, []byte{1, 2, 3, 4}))
	require.Equal(t, []byte{1, 2, 3, 4}, out.Bytes())
	require.Error(t, writeFrame(shortWriter{}, []byte{1, 2, 3, 4}))
}

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }

// TestReleaseFrameRespectsCancellation verifies an unsignaled fence can be
// abandoned without hanging the command.
func TestReleaseFrameRespectsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, releaseFrame(ctx, waitingFrame{}), context.Canceled)
}

// waitingFrame simulates a frame whose acquire fence never signals.
type waitingFrame struct{}

// Release waits until the caller's context ends.
func (waitingFrame) Release(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }
