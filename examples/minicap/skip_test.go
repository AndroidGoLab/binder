package main

import (
	"context"
	"github.com/AndroidGoLab/binder/gralloc"
	"github.com/AndroidGoLab/binder/igbp"
	"github.com/AndroidGoLab/binder/parcel"
	"github.com/stretchr/testify/require"
	"testing"
)

// TestSkipBoundedReplenishing verifies a continuously replenished queue cannot
// starve frame delivery and that cancellation stops the pass.
func TestSkipBoundedReplenishing(t *testing.T) {
	s := &replenishingSkipper{}
	require.NoError(t, skipAtMost(context.Background(), 3, s))
	require.Equal(t, 3, s.calls)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, skipAtMost(ctx, 3, s), context.Canceled)
	require.Equal(t, 3, s.calls)
}

// replenishingSkipper simulates a producer that always has another frame.
type replenishingSkipper struct{ calls int }

// SkipNext records a skipped frame and immediately offers another.
func (s *replenishingSkipper) SkipNext(context.Context) (bool, error) { s.calls++; return true, nil }

// TestSkipRetainsOnlyQueuedFrame preserves a static display frame after a client
// reconnects when no newer frame is available.
func TestSkipRetainsOnlyQueuedFrame(t *testing.T) {
	ctx := context.Background()
	producer, err := igbp.NewFrameProducer(34, []*gralloc.Buffer{{Width: 1, Height: 1, Stride: 1, Format: 1}})
	require.NoError(t, err)
	req := parcel.New()
	req.WriteInterfaceToken(igbp.Descriptor)
	req.WriteUint32(1)
	req.WriteUint32(1)
	req.WriteInt32(1)
	req.WriteUint64(0)
	req.WriteBool(false)
	req.SetPosition(0)
	_, err = producer.OnTransaction(ctx, igbp.DequeueBuffer, req)
	require.NoError(t, err)
	raw := make([]byte, 61)
	req = parcel.New()
	req.WriteInterfaceToken(igbp.Descriptor)
	req.WriteInt32(0)
	req.WriteInt32(int32(len(raw)))
	req.WriteInt32(0)
	req.WriteRawBytes(raw)
	req.SetPosition(0)
	_, err = producer.OnTransaction(ctx, igbp.QueueBuffer, req)
	require.NoError(t, err)
	frame, err := producer.Next(ctx)
	require.NoError(t, err)
	skipper := &queuedFrameSkipper{producer: producer, current: frame}
	require.NoError(t, skipAtMost(ctx, captureBufferCount-1, skipper))
	require.Same(t, frame, skipper.current)
	require.NoError(t, frame.Release(ctx))
	require.NoError(t, producer.Close(ctx))
}
