package main

import (
	"context"
	"errors"

	"github.com/AndroidGoLab/binder/igbp"
)

// pendingFrameSkipper releases one obsolete frame if one is queued.
//
// Example: discard a stale frame without waiting for another.
type pendingFrameSkipper interface {
	SkipNext(context.Context) (bool, error)
}

// skipAtMost bounds a skip pass even if the producer continuously refills it.
//
// Example: skip up to the number of allocated buffers.
func skipAtMost(ctx context.Context, limit int, s pendingFrameSkipper) error {
	for i := 0; i < limit; i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		skipped, err := s.SkipNext(ctx)
		if err != nil {
			return err
		}
		if !skipped {
			break
		}
	}
	return nil
}

// queuedFrameSkipper drops stale frames or replaces its current frame with newer ones.
//
// Example: keep the newest queued lease after acquiring one frame.
type queuedFrameSkipper struct {
	producer *igbp.FrameProducer
	current  *igbp.Frame
}

// SkipNext releases a queued frame or the older current frame.
//
// Example: preserve only the latest lease for encoding.
func (s *queuedFrameSkipper) SkipNext(_ context.Context) (bool, error) {
	next, ok := s.producer.TryNext()
	if !ok {
		return false, nil
	}
	old := next
	if s.current != nil {
		old = s.current
	}
	ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
	err := releaseFrame(ctx, old)
	cancel()
	if err != nil {
		if old != next {
			ctx, cancel = context.WithTimeout(context.Background(), cleanupTimeout)
			err = errors.Join(err, releaseFrame(ctx, next))
			cancel()
		}
		return false, err
	}
	if s.current != nil {
		s.current = next
	}
	return true, nil
}
