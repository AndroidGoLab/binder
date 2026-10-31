package versionaware

import (
	"context"
	"testing"

	"github.com/AndroidGoLab/binder/binder"
	"github.com/stretchr/testify/require"
)

// TestSurfaceComposerAndroid14R35 verifies the explicitly selected contract.
//
// Example: resolving native SurfaceComposer methods absent from framework JARs.
func TestSurfaceComposerAndroid14R35(t *testing.T) {
	cfg := Options{OptionSurfaceComposerAndroid14R35()}.config()
	tr := &Transport{table: VersionTable{"android.gui.ISurfaceComposer": {"getPhysicalDisplayIds": 2, "other": 3}}, overrides: cfg.TransactionCodes}
	for method, want := range map[string]binder.TransactionCode{"getPhysicalDisplayIds": 6, "captureDisplayById": 26, "other": 3} {
		code, err := tr.ResolveCode(context.Background(), "android.gui.ISurfaceComposer", method)
		require.NoError(t, err)
		require.Equal(t, want, code)
	}
	_, err := tr.ResolveCode(context.Background(), "android.gui.ISurfaceComposer", "missing")
	require.Error(t, err)
}

// TestSurfaceComposerAndroid14R35RejectsOtherAPI checks the profile boundary.
//
// Example: an Android 15 transport must not use the Android 14 contract.
func TestSurfaceComposerAndroid14R35RejectsOtherAPI(t *testing.T) {
	_, err := NewTransport(context.Background(), nil, 35, OptionSurfaceComposerAndroid14R35())
	require.ErrorContains(t, err, "requires API 34")
}
