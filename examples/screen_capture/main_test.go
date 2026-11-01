package main

import (
	"context"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRunRejectsInvalidScale checks values that cannot be used for capture.
//
// Example: zero or infinite scale fails before any Binder operation.
func TestRunRejectsInvalidScale(t *testing.T) {
	for _, scale := range []float32{0, -1, float32(math.NaN()), float32(math.Inf(1))} {
		err := run(context.Background(), scale, "unused.rgba")
		require.ErrorContains(t, err, "invalid display scale")
	}
}
