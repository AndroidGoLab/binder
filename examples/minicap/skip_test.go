package main

import (
	"context"
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
