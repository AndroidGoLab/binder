package versionaware

import (
	"context"
	"debug/elf"
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
	tr := &Transport{table: VersionTable{"android.gui.ISurfaceComposer": {"getPhysicalDisplayIds": 2, "other": 3}}, overrides: cfg.TransactionCodes, surfaceComposerAndroid14R35: cfg.SurfaceComposerAndroid14R35}
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

// TestRecognizeSurfaceComposer verifies the verified device contract boundary.
//
// Example: a different library must not enable native capture.
func TestRecognizeSurfaceComposer(t *testing.T) {
	verified := surfaceComposerLibrary{API: 34, Class: elf.ELFCLASS64, Machine: elf.EM_AARCH64, BuildID: "e1a469cf6fa0276c7bb104f53fc1f463", Symbols: map[string]bool{
		"_ZN7android3gui17BpSurfaceComposer18captureDisplayByIdElRKNS0_11CaptureArgsERKNS_2spINS0_22IScreenCaptureListenerEEE": true,
		"_ZN7android3gui17BpSurfaceComposer21getPhysicalDisplayIdsEPNSt3__16vectorIlNS2_9allocatorIlEEEE":                      true,
		"_ZNK7android3gui11CaptureArgs13writeToParcelEPNS_6ParcelE":                                                            true,
	}}
	require.True(t, verified.supported())
	wrongAPI := verified
	wrongAPI.API = 35
	require.False(t, wrongAPI.supported())
	wrongArch := verified
	wrongArch.Machine = elf.EM_X86_64
	require.False(t, wrongArch.supported())
	wrongClass := verified
	wrongClass.Class = elf.ELFCLASS32
	require.False(t, wrongClass.supported())
	wrongBuild := verified
	wrongBuild.BuildID = "unknown"
	require.False(t, wrongBuild.supported())
	for symbol := range verified.Symbols {
		missingSymbol := verified
		missingSymbol.Symbols = make(map[string]bool, len(verified.Symbols))
		for present := range verified.Symbols {
			missingSymbol.Symbols[present] = present != symbol
		}
		require.False(t, missingSymbol.supported(), "missing %s", symbol)
	}
}

// TestUnselectedSurfaceComposerRejectsCachedCodes prevents unsafe transactions.
//
// Example: an old cache cannot select a native contract on a new firmware.
func TestUnselectedSurfaceComposerRejectsCachedCodes(t *testing.T) {
	tr := &Transport{apiLevel: 34, table: VersionTable{"android.gui.ISurfaceComposer": {"getPhysicalDisplayIds": 6, "captureDisplayById": 26, "other": 7}}}
	for _, method := range []string{"captureDisplayById"} {
		_, err := tr.ResolveCode(context.Background(), "android.gui.ISurfaceComposer", method)
		require.ErrorContains(t, err, "unsupported")
		require.ErrorContains(t, err, "API 34")
		require.ErrorContains(t, err, "/system/lib64/libgui.so")
	}
	code, err := tr.ResolveCode(context.Background(), "android.gui.ISurfaceComposer", "getPhysicalDisplayIds")
	require.NoError(t, err)
	require.Equal(t, binder.TransactionCode(6), code)
	code, err = tr.ResolveCode(context.Background(), "android.gui.ISurfaceComposer", "other")
	require.NoError(t, err)
	require.Equal(t, binder.TransactionCode(7), code)
}

// TestSurfaceComposerDefinedSymbols excludes imported functions from evidence.
//
// Example: an undefined capture symbol does not identify a client implementation.
func TestSurfaceComposerDefinedSymbols(t *testing.T) {
	symbols := surfaceComposerDefinedSymbols([]elf.Symbol{{Name: surfaceComposerCaptureSymbol, Section: elf.SHN_UNDEF}, {Name: surfaceComposerIDsSymbol, Section: 1}})
	require.False(t, symbols[surfaceComposerCaptureSymbol])
	require.True(t, symbols[surfaceComposerIDsSymbol])
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
