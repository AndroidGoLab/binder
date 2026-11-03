package gralloc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAllocateRGBARejectsInvalidAndroid10Input checks validation before opening hwbinder.
//
// Example: a zero width is rejected without accessing a device.
func TestAllocateRGBARejectsInvalidAndroid10Input(t *testing.T) {
	for _, tc := range []struct {
		width, height int32
		count         int
		message       string
	}{
		{0, 1, 1, "dimensions"},
		{1, -1, 1, "dimensions"},
		{1, 1, 0, "count"},
		{1, 1, 65, "count"},
	} {
		_, err := AllocateRGBA(context.Background(), nil, 29, tc.width, tc.height, tc.count)
		require.ErrorContains(t, err, tc.message)
	}
}

// TestAllocateRGBAPreservesPixelValidation checks the API 34 validation order.
//
// Example: a missing service manager is reported before invalid dimensions.
func TestAllocateRGBAPreservesPixelValidation(t *testing.T) {
	_, err := AllocateRGBA(context.Background(), nil, 34, 0, 1, 1)
	require.ErrorContains(t, err, "invalid service manager or buffer count")
}
