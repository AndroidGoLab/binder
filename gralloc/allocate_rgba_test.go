package gralloc

import (
	"context"
	"runtime"
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

// TestAllocateRGBAPreservesPixelValidation checks the platform-specific validation order.
//
// Example: the native Android allocator checks dimensions without a service manager.
func TestAllocateRGBAPreservesPixelValidation(t *testing.T) {
	_, err := AllocateRGBA(context.Background(), nil, 34, 0, 1, 1)
	if runtime.GOOS == "android" && runtime.GOARCH == "arm64" {
		require.ErrorContains(t, err, "dimensions")
	} else {
		require.ErrorContains(t, err, "invalid service manager or buffer count")
	}
}

// TestAndroid10AllocationRejectsUnknownProfile checks selection before I/O.
//
// Example: an unknown profile does not use the transport.
func TestAndroid10AllocationRejectsUnknownProfile(t *testing.T) {
	_, err := allocateAndroid10Buffers(context.Background(), nil, 1, 1, 1, 1, android10Profile(99))
	require.ErrorContains(t, err, "unsupported")
}
