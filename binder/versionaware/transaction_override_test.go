package versionaware

import (
	"context"
	"testing"

	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/binder/versionaware/dex"
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

// TestSurfaceComposerR35SignatureOverridesExtracted checks the selected contract.
//
// Example: an unrelated extracted signature cannot change capture serialization.
func TestSurfaceComposerR35SignatureOverridesExtracted(t *testing.T) {
	tr := &Transport{surfaceComposerAndroid14R35: true, signaturesLoaded: true, signatures: map[string]dex.MethodSignatures{
		"android.gui.ISurfaceComposer": {"captureDisplayById": {"J", "Landroid/gui/IScreenCaptureListener;"}, "getPhysicalDisplayIds": {"I"}, "other": {"I"}},
	}}
	require.Equal(t, []string{"J", "Landroid/gui/CaptureArgs;", "Landroid/gui/IScreenCaptureListener;"}, tr.ResolveMethodSignature(context.Background(), "android.gui.ISurfaceComposer", "captureDisplayById"))
	require.Empty(t, tr.ResolveMethodSignature(context.Background(), "android.gui.ISurfaceComposer", "getPhysicalDisplayIds"))
	require.Equal(t, []string{"I"}, tr.ResolveMethodSignature(context.Background(), "android.gui.ISurfaceComposer", "other"))
}

// TestSurfaceComposerAndroid14R35RejectsOtherAPI checks the profile boundary.
//
// Example: an Android 15 transport must not use the Android 14 contract.
func TestSurfaceComposerAndroid14R35RejectsOtherAPI(t *testing.T) {
	_, err := NewTransport(context.Background(), nil, 35, OptionSurfaceComposerAndroid14R35())
	require.ErrorContains(t, err, "requires API 34")
}

// TestSurfaceComposerAndroid14R35Capability requires explicit profile selection.
//
// Example: a capture client can reject unknown native wire contracts.
func TestSurfaceComposerAndroid14R35Capability(t *testing.T) {
	cfg := Options{OptionSurfaceComposerAndroid14R35()}.config()
	require.True(t, (&Transport{surfaceComposerAndroid14R35: cfg.SurfaceComposerAndroid14R35}).UsesSurfaceComposerAndroid14R35())
	require.False(t, (&Transport{}).UsesSurfaceComposerAndroid14R35())
	require.False(t, (&Transport{overrides: VersionTable{"android.gui.ISurfaceComposer": {"getPhysicalDisplayIds": 6, "captureDisplayById": 26}}}).UsesSurfaceComposerAndroid14R35())
}
