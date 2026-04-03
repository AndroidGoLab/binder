//go:build e2e || e2e_root

package e2e

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	genGui "github.com/AndroidGoLab/binder/android/gui"
	genDisplay "github.com/AndroidGoLab/binder/android/hardware/display"
	"github.com/AndroidGoLab/binder/android/graphics"
	genView "github.com/AndroidGoLab/binder/android/view"
	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/binder/versionaware"
	"github.com/AndroidGoLab/binder/kernelbinder"
	"github.com/AndroidGoLab/binder/servicemanager"
)

// --- shared driver for window_surface tests ---
// These tests make 60+ binder calls. Using a separate fd for each
// exhausts per-process binder resources after 300+ earlier tests.
var (
	windowSurfaceDriver    *versionaware.Transport
	windowSurfaceOnce      sync.Once
	windowSurfaceErr       error
)

func windowSurfaceOpenBinder(t *testing.T) *versionaware.Transport {
	t.Helper()
	windowSurfaceOnce.Do(func() {
		ctx := context.Background()
		drv, err := kernelbinder.Open(ctx, binder.WithMapSize(128*1024))
		if err != nil {
			windowSurfaceErr = err
			return
		}
		tr, err := versionaware.NewTransport(ctx, drv, 0)
		if err != nil {
			windowSurfaceErr = err
			return
		}
		windowSurfaceDriver = tr
	})
	require.NoError(t, windowSurfaceErr, "failed to open window_surface shared binder")
	return windowSurfaceDriver
}

// --- helpers ---

func getDisplayManager(
	ctx context.Context,
	t *testing.T,
	driver *versionaware.Transport,
) *genDisplay.DisplayManagerProxy {
	t.Helper()
	svc := getService(ctx, t, driver, "display")
	return genDisplay.NewDisplayManagerProxy(svc)
}

func getWindowManager(
	ctx context.Context,
	t *testing.T,
	driver *versionaware.Transport,
) *genView.WindowManagerProxy {
	t.Helper()
	svc := getService(ctx, t, driver, "window")
	return genView.NewWindowManagerProxy(svc)
}

func getSurfaceComposerProxy(
	ctx context.Context,
	t *testing.T,
	driver *versionaware.Transport,
) *genGui.SurfaceComposerProxy {
	t.Helper()
	sm := servicemanager.New(driver)
	svc, err := sm.GetService(ctx, servicemanager.ServiceName("SurfaceFlingerAIDL"))
	require.NoError(t, err, "GetService(SurfaceFlingerAIDL) failed")
	require.NotNil(t, svc)
	return genGui.NewSurfaceComposerProxy(svc)
}

// --- 1. Virtual display via IDisplayManager ---

func TestWindowSurface_DisplayManager_GetDisplayIds(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	dm := getDisplayManager(ctx, t, driver)

	ids, err := dm.GetDisplayIds(ctx, false)
	requireOrSkip(t, err)
	require.NotEmpty(t, ids, "device must have at least one display")
	assert.Equal(t, int32(0), ids[0], "default display ID should be 0")
	t.Logf("display IDs: %v", ids)

	// Fetch with disabled displays included to exercise the boolean parameter.
	allIds, err := dm.GetDisplayIds(ctx, true)
	requireOrSkip(t, err)
	assert.GreaterOrEqual(t, len(allIds), len(ids),
		"including disabled displays should return at least as many IDs")
	t.Logf("display IDs (including disabled): %v", allIds)
}

func TestWindowSurface_DisplayManager_GetDisplayInfo(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	dm := getDisplayManager(ctx, t, driver)

	// GetDisplayInfo returns interface{} (untyped parcelable) in the generated
	// proxy, but successfully round-tripping through the binder proves the
	// parameter marshaling and reply status parsing work.
	info, err := dm.GetDisplayInfo(ctx, 0)
	requireOrSkip(t, err)
	// The generated proxy returns nil for the result because the parcelable
	// type is not generated (interface{}). A nil result with no error confirms
	// the transaction completed and the status was parsed.
	_ = info
	t.Logf("GetDisplayInfo(0) completed without error")
}

func TestWindowSurface_DisplayManager_StableDisplaySize(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	dm := getDisplayManager(ctx, t, driver)

	_, err := dm.GetStableDisplaySize(ctx)
	requireOrSkip(t, err)
	// graphics.Point is an empty generated struct (no X/Y fields), so we
	// cannot inspect the values. A nil-error confirms the transaction succeeded.
	t.Logf("GetStableDisplaySize completed without error")
}

func TestWindowSurface_DisplayManager_Brightness(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	dm := getDisplayManager(ctx, t, driver)

	brightness, err := dm.GetBrightness(ctx, 0)
	requireOrSkip(t, err)
	// Brightness is a float in [0,1] or -1 (auto). Any finite value is valid.
	t.Logf("display 0 brightness: %f", brightness)
}

func TestWindowSurface_DisplayManager_GetUserDisabledHdrTypes(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	dm := getDisplayManager(ctx, t, driver)

	types, err := dm.GetUserDisabledHdrTypes(ctx)
	requireOrSkip(t, err)
	t.Logf("user disabled HDR types: %v (count=%d)", types, len(types))
}

func TestWindowSurface_DisplayManager_AreUserDisabledHdrTypesAllowed(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	dm := getDisplayManager(ctx, t, driver)

	allowed, err := dm.AreUserDisabledHdrTypesAllowed(ctx)
	requireOrSkip(t, err)
	t.Logf("areUserDisabledHdrTypesAllowed: %v", allowed)
}

func TestWindowSurface_DisplayManager_RefreshRateSwitchingType(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	dm := getDisplayManager(ctx, t, driver)

	switchType, err := dm.GetRefreshRateSwitchingType(ctx)
	requireOrSkip(t, err)
	t.Logf("refresh rate switching type: %d", switchType)
}

func TestWindowSurface_DisplayManager_PreferredWideGamutColorSpaceId(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	dm := getDisplayManager(ctx, t, driver)

	colorSpaceId, err := dm.GetPreferredWideGamutColorSpaceId(ctx)
	requireOrSkip(t, err)
	t.Logf("preferred wide-gamut color space ID: %d", colorSpaceId)
}

func TestWindowSurface_DisplayManager_ShouldAlwaysRespectAppRequestedMode(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	dm := getDisplayManager(ctx, t, driver)

	respect, err := dm.ShouldAlwaysRespectAppRequestedMode(ctx)
	requireOrSkip(t, err)
	t.Logf("shouldAlwaysRespectAppRequestedMode: %v", respect)
}

func TestWindowSurface_DisplayManager_IsUidPresentOnDisplay(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	dm := getDisplayManager(ctx, t, driver)

	// uid 0 (root) should be present on the default display.
	present, err := dm.IsUidPresentOnDisplay(ctx, 0, 0)
	requireOrSkip(t, err)
	t.Logf("isUidPresentOnDisplay(uid=0, display=0): %v", present)
}

func TestWindowSurface_DisplayManager_IsMinimalPostProcessingRequested(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	dm := getDisplayManager(ctx, t, driver)

	minPP, err := dm.IsMinimalPostProcessingRequested(ctx, 0)
	requireOrSkip(t, err)
	t.Logf("isMinimalPostProcessingRequested(display=0): %v", minPP)
}

func TestWindowSurface_DisplayManager_GetDisplayDecorationSupport(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	dm := getDisplayManager(ctx, t, driver)

	// This exercises nested parcelable unmarshaling: DisplayDecorationSupport
	// contains Format + AlphaInterpretation.
	decor, err := dm.GetDisplayDecorationSupport(ctx, 0)
	requireOrSkip(t, err)
	t.Logf("DisplayDecorationSupport(display=0): format=%d, alphaInterpretation=%d",
		decor.Format, decor.AlphaInterpretation)
}

func TestWindowSurface_DisplayManager_GetOverlaySupport(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	dm := getDisplayManager(ctx, t, driver)

	// OverlayProperties contains an array of SupportedBufferCombinations
	// parcelables, exercising nested array-of-parcelable unmarshaling.
	overlay, err := dm.GetOverlaySupport(ctx)
	requireOrSkip(t, err)
	t.Logf("OverlaySupport: %+v", overlay)
}

func TestWindowSurface_DisplayManager_GetSupportedHdrOutputTypes(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	dm := getDisplayManager(ctx, t, driver)

	types, err := dm.GetSupportedHdrOutputTypes(ctx)
	requireOrSkip(t, err)
	t.Logf("supported HDR output types: %v (count=%d)", types, len(types))
}

// --- 2. Window management via IWindowManager ---

func TestWindowSurface_WindowManager_GetInitialDisplaySize(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	wm := getWindowManager(ctx, t, driver)

	// The generated proxy passes graphics.Point by value (not pointer), so the
	// caller's struct is not populated. The transaction still completes
	// successfully, which validates the marshaling round-trip. The by-value
	// signature is a codegen limitation for AIDL "out" parameters.
	var size graphics.Point
	err := wm.GetInitialDisplaySize(ctx, 0, size)
	requireOrSkip(t, err)
	t.Logf("GetInitialDisplaySize completed without error")
}

func TestWindowSurface_WindowManager_GetBaseDisplaySize(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	wm := getWindowManager(ctx, t, driver)

	// Same by-value limitation as GetInitialDisplaySize.
	var size graphics.Point
	err := wm.GetBaseDisplaySize(ctx, 0, size)
	requireOrSkip(t, err)
	t.Logf("GetBaseDisplaySize completed without error")
}

func TestWindowSurface_WindowManager_GetInitialDisplayDensity(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	wm := getWindowManager(ctx, t, driver)

	density, err := wm.GetInitialDisplayDensity(ctx, 0)
	requireOrSkip(t, err)
	assert.Greater(t, density, int32(0), "initial display density should be > 0")
	t.Logf("initial display density: %d dpi", density)
}

func TestWindowSurface_WindowManager_GetBaseDisplayDensity(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	wm := getWindowManager(ctx, t, driver)

	density, err := wm.GetBaseDisplayDensity(ctx, 0)
	requireOrSkip(t, err)
	assert.Greater(t, density, int32(0), "base display density should be > 0")
	t.Logf("base display density: %d dpi", density)
}

// --- 3. Window layout parameters via IWindowManager ---

func TestWindowSurface_WindowManager_GetAnimationScales(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	wm := getWindowManager(ctx, t, driver)

	// GetAnimationScales returns a float32 array (3 values: window, transition, animator).
	scales, err := wm.GetAnimationScales(ctx)
	requireOrSkip(t, err)
	require.NotEmpty(t, scales, "animation scales should not be empty")
	for i, s := range scales {
		assert.GreaterOrEqual(t, s, float32(0), "scale %d should be >= 0", i)
	}
	t.Logf("animation scales: %v", scales)
}

func TestWindowSurface_WindowManager_GetDefaultDisplayRotation(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	wm := getWindowManager(ctx, t, driver)

	rotation, err := wm.GetDefaultDisplayRotation(ctx)
	requireOrSkip(t, err)
	// Rotation values: 0=ROTATION_0, 1=ROTATION_90, 2=ROTATION_180, 3=ROTATION_270.
	assert.GreaterOrEqual(t, rotation, int32(0))
	assert.LessOrEqual(t, rotation, int32(3))
	t.Logf("default display rotation: %d", rotation)
}

func TestWindowSurface_WindowManager_IsKeyguardLocked(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	wm := getWindowManager(ctx, t, driver)

	locked, err := wm.IsKeyguardLocked(ctx)
	requireOrSkip(t, err)
	t.Logf("isKeyguardLocked: %v", locked)
}

func TestWindowSurface_WindowManager_IsKeyguardSecure(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	wm := getWindowManager(ctx, t, driver)

	secure, err := wm.IsKeyguardSecure(ctx)
	requireOrSkip(t, err)
	t.Logf("isKeyguardSecure: %v", secure)
}

func TestWindowSurface_WindowManager_HasNavigationBar(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	wm := getWindowManager(ctx, t, driver)

	hasNav, err := wm.HasNavigationBar(ctx, 0)
	requireOrSkip(t, err)
	t.Logf("hasNavigationBar(display=0): %v", hasNav)
}

func TestWindowSurface_WindowManager_IsSafeModeEnabled(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	wm := getWindowManager(ctx, t, driver)

	safeMode, err := wm.IsSafeModeEnabled(ctx)
	requireOrSkip(t, err)
	assert.False(t, safeMode, "safe mode should not be enabled during tests")
	t.Logf("isSafeModeEnabled: %v", safeMode)
}

func TestWindowSurface_WindowManager_IsRotationFrozen(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	wm := getWindowManager(ctx, t, driver)

	frozen, err := wm.IsRotationFrozen(ctx)
	requireOrSkip(t, err)
	t.Logf("isRotationFrozen: %v", frozen)
}

func TestWindowSurface_WindowManager_GetWindowingMode(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	wm := getWindowManager(ctx, t, driver)

	mode, err := wm.GetWindowingMode(ctx, 0)
	requireOrSkip(t, err)
	t.Logf("getWindowingMode(display=0): %d", mode)
}

func TestWindowSurface_WindowManager_GetDisplayImePolicy(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	wm := getWindowManager(ctx, t, driver)

	policy, err := wm.GetDisplayImePolicy(ctx, 0)
	requireOrSkip(t, err)
	t.Logf("getDisplayImePolicy(display=0): %d", policy)
}

func TestWindowSurface_WindowManager_ShouldShowSystemDecors(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	wm := getWindowManager(ctx, t, driver)

	show, err := wm.ShouldShowSystemDecors(ctx, 0)
	requireOrSkip(t, err)
	t.Logf("shouldShowSystemDecors(display=0): %v", show)
}

func TestWindowSurface_WindowManager_GetPreferredOptionsPanelGravity(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	wm := getWindowManager(ctx, t, driver)

	gravity, err := wm.GetPreferredOptionsPanelGravity(ctx, 0)
	requireOrSkip(t, err)
	t.Logf("getPreferredOptionsPanelGravity(display=0): %d", gravity)
}

func TestWindowSurface_WindowManager_GetSupportedDisplayHashAlgorithms(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	wm := getWindowManager(ctx, t, driver)

	// Returns a string array, exercising string16[] deserialization.
	algorithms, err := wm.GetSupportedDisplayHashAlgorithms(ctx)
	// Server-side NPE in WindowManager on some ROMs (e.g. GrapheneOS):
	// "Attempt to invoke virtual method 'int android.os.Bundle.size()' on a null object reference"
	if err != nil && strings.Contains(err.Error(), "exception NullPointer") {
		t.Skipf("server-side NPE (not a library bug): %v", err)
	}
	requireOrSkip(t, err)
	t.Logf("supported display hash algorithms: %v (count=%d)", algorithms, len(algorithms))
}

func TestWindowSurface_WindowManager_IsTaskSnapshotSupported(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	wm := getWindowManager(ctx, t, driver)

	supported, err := wm.IsTaskSnapshotSupported(ctx)
	requireOrSkip(t, err)
	t.Logf("isTaskSnapshotSupported: %v", supported)
}

func TestWindowSurface_WindowManager_GetImeDisplayId(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	wm := getWindowManager(ctx, t, driver)

	imeDisplay, err := wm.GetImeDisplayId(ctx)
	requireOrSkip(t, err)
	t.Logf("getImeDisplayId: %d", imeDisplay)
}

func TestWindowSurface_WindowManager_GetDockedStackSide(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	wm := getWindowManager(ctx, t, driver)

	side, err := wm.GetDockedStackSide(ctx)
	requireOrSkip(t, err)
	t.Logf("getDockedStackSide: %d", side)
}

func TestWindowSurface_WindowManager_GetCurrentAnimatorScale(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	wm := getWindowManager(ctx, t, driver)

	scale, err := wm.GetCurrentAnimatorScale(ctx)
	requireOrSkip(t, err)
	assert.GreaterOrEqual(t, scale, float32(0), "animator scale should be >= 0")
	t.Logf("getCurrentAnimatorScale: %f", scale)
}

func TestWindowSurface_WindowManager_GetLetterboxBackgroundColorInArgb(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	wm := getWindowManager(ctx, t, driver)

	color, err := wm.GetLetterboxBackgroundColorInArgb(ctx)
	requireOrSkip(t, err)
	t.Logf("getLetterboxBackgroundColorInArgb: 0x%08X", uint32(color))
}

func TestWindowSurface_WindowManager_IsLetterboxBackgroundMultiColored(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	wm := getWindowManager(ctx, t, driver)

	multi, err := wm.IsLetterboxBackgroundMultiColored(ctx)
	requireOrSkip(t, err)
	t.Logf("isLetterboxBackgroundMultiColored: %v", multi)
}

// --- 4. SurfaceComposer layer creation ---

func TestWindowSurface_SurfaceComposer_CreateConnection(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	sf := getSurfaceComposerProxy(ctx, t, driver)

	// CreateConnection returns an ISurfaceComposerClient binder, which can
	// then be used to create surfaces (layers).
	client, err := sf.CreateConnection(ctx)
	requireOrSkip(t, err)
	require.NotNil(t, client, "SurfaceComposerClient should not be nil")
	t.Logf("created SurfaceComposerClient")

	// Verify the returned client binder is alive.
	clientBinder := client.AsBinder()
	require.NotNil(t, clientBinder, "client binder should not be nil")
	t.Logf("SurfaceComposerClient handle: %d", clientBinder.Handle())
}

func TestWindowSurface_SurfaceComposer_CreateSurfaceLayer(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	sf := getSurfaceComposerProxy(ctx, t, driver)

	client, err := sf.CreateConnection(ctx)
	requireOrSkip(t, err)
	require.NotNil(t, client)

	// CreateSurface with eFXSurfaceEffect flag creates a color/effect layer
	// (no buffer needed). This exercises CreateSurfaceResult parcelable
	// unmarshaling, which contains a binder handle, int32 layerId,
	// string layerName, and int32 transformHint.
	//
	// Uses the generated proxy which correctly serializes all parameters
	// including LayerMetadata. WriteBinderToParcel handles nil parent by
	// writing a null binder.
	result, err := client.CreateSurface(
		ctx,
		"e2e-test-layer",
		genGui.ISurfaceComposerClientEFXSurfaceEffect,
		nil, // null parent
		genGui.LayerMetadata{},
	)
	if err != nil && strings.Contains(err.Error(), "kernel status error: -61") {
		// createSurface requires ACCESS_SURFACE_FLINGER (signature-level,
		// not grantable to shell). The binder round-trip succeeded (the
		// service received and rejected the call). Log and pass.
		t.Logf("createSurface denied (ACCESS_SURFACE_FLINGER required): %v", err)
		return
	}
	requireOrSkip(t, err)
	assert.NotZero(t, result.LayerId, "layer ID should be non-zero")
	assert.NotEmpty(t, result.LayerName, "layer name should not be empty")
	t.Logf("created surface: layerId=%d, name=%q, transformHint=%d",
		result.LayerId, result.LayerName, result.TransformHint)
}

func TestWindowSurface_SurfaceComposer_CreateContainerLayer(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	sf := getSurfaceComposerProxy(ctx, t, driver)

	client, err := sf.CreateConnection(ctx)
	requireOrSkip(t, err)
	require.NotNil(t, client)

	// Container layers are metadata-only (no buffers, no effects).
	// Uses the generated proxy which correctly serializes all parameters
	// including LayerMetadata.
	result, err := client.CreateSurface(
		ctx,
		"e2e-test-container",
		genGui.ISurfaceComposerClientEFXSurfaceContainer,
		nil, // null parent
		genGui.LayerMetadata{},
	)
	if err != nil && strings.Contains(err.Error(), "kernel status error: -61") {
		// createSurface requires ACCESS_SURFACE_FLINGER (signature-level,
		// not grantable to shell). The binder round-trip succeeded.
		t.Logf("createSurface denied (ACCESS_SURFACE_FLINGER required): %v", err)
		return
	}
	requireOrSkip(t, err)
	assert.NotZero(t, result.LayerId, "container layer ID should be non-zero")
	t.Logf("created container layer: layerId=%d, name=%q",
		result.LayerId, result.LayerName)
}

func TestWindowSurface_SurfaceComposer_MirrorDisplay(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	sf := getSurfaceComposerProxy(ctx, t, driver)

	// Get a physical display ID first.
	physIds, err := sf.GetPhysicalDisplayIds(ctx)
	requireOrSkip(t, err)
	require.NotEmpty(t, physIds, "need at least one physical display")

	client, err := sf.CreateConnection(ctx)
	requireOrSkip(t, err)
	require.NotNil(t, client)

	// MirrorDisplay exercises CreateSurfaceResult parcelable with a
	// display-backed mirror layer.
	result, err := client.MirrorDisplay(ctx, physIds[0])
	requireOrSkip(t, err)
	assert.NotZero(t, result.LayerId, "mirror layer ID should be non-zero")
	t.Logf("mirror display layer: layerId=%d, name=%q, transformHint=%d",
		result.LayerId, result.LayerName, result.TransformHint)
}

func TestWindowSurface_SurfaceComposer_GetSupportedFrameTimestamps(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	sf := getSurfaceComposerProxy(ctx, t, driver)

	// Returns an enum array (FrameEvent[]), exercising typed-enum array
	// deserialization.
	timestamps, err := sf.GetSupportedFrameTimestamps(ctx)
	requireOrSkip(t, err)
	require.NotEmpty(t, timestamps, "should support at least one frame timestamp type")
	t.Logf("supported frame timestamps: %v (count=%d)", timestamps, len(timestamps))
}

func TestWindowSurface_SurfaceComposer_GetSchedulingPolicy(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	sf := getSurfaceComposerProxy(ctx, t, driver)

	// SchedulingPolicy is a parcelable with policy + priority.
	policy, err := sf.GetSchedulingPolicy(ctx)
	requireOrSkip(t, err)
	t.Logf("SurfaceFlinger scheduling policy: policy=%d, priority=%d",
		policy.Policy, policy.Priority)
}

func TestWindowSurface_SurfaceComposer_GetMaxAcquiredBufferCount(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	sf := getSurfaceComposerProxy(ctx, t, driver)

	maxBuf, err := sf.GetMaxAcquiredBufferCount(ctx)
	requireOrSkip(t, err)
	assert.Greater(t, maxBuf, int32(0), "max acquired buffer count should be > 0")
	t.Logf("max acquired buffer count: %d", maxBuf)
}

func TestWindowSurface_SurfaceComposer_GetGpuContextPriority(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	sf := getSurfaceComposerProxy(ctx, t, driver)

	gpuPriority, err := sf.GetGpuContextPriority(ctx)
	if err != nil && strings.Contains(err.Error(), "read beyond end") {
		t.Skipf("method not implemented on this device: %v", err)
	}
	requireOrSkip(t, err)
	t.Logf("GPU context priority: %d", gpuPriority)
}

func TestWindowSurface_SurfaceComposer_GetProtectedContentSupport(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	sf := getSurfaceComposerProxy(ctx, t, driver)

	supported, err := sf.GetProtectedContentSupport(ctx)
	requireOrSkip(t, err)
	t.Logf("protected content support: %v", supported)
}

func TestWindowSurface_SurfaceComposer_GetBootDisplayModeSupport(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	sf := getSurfaceComposerProxy(ctx, t, driver)

	supported, err := sf.GetBootDisplayModeSupport(ctx)
	requireOrSkip(t, err)
	t.Logf("boot display mode support: %v", supported)
}

func TestWindowSurface_SurfaceComposer_GetHdrOutputConversionSupport(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	sf := getSurfaceComposerProxy(ctx, t, driver)

	supported, err := sf.GetHdrOutputConversionSupport(ctx)
	requireOrSkip(t, err)
	t.Logf("HDR output conversion support: %v", supported)
}

func TestWindowSurface_SurfaceComposer_GetOverlaySupport(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	sf := getSurfaceComposerProxy(ctx, t, driver)

	// OverlayProperties contains a nested array of parcelables, testing
	// recursive parcelable unmarshaling.
	overlay, err := sf.GetOverlaySupport(ctx)
	requireOrSkip(t, err)
	t.Logf("SurfaceComposer overlay: %d combinations, mixedColorSpaces=%v",
		len(overlay.Combinations), overlay.SupportMixedColorSpaces)
}

// --- 5. Display configuration details ---

func TestWindowSurface_SurfaceComposer_GetPhysicalDisplayToken(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	sf := getSurfaceComposerProxy(ctx, t, driver)

	physIds, err := sf.GetPhysicalDisplayIds(ctx)
	requireOrSkip(t, err)
	require.NotEmpty(t, physIds)

	token, err := sf.GetPhysicalDisplayToken(ctx, physIds[0])
	requireOrSkip(t, err)
	require.NotNil(t, token, "display token should not be nil")
	t.Logf("physical display token handle: %d (for display ID %d)",
		token.Handle(), physIds[0])
}

func TestWindowSurface_SurfaceComposer_GetStaticDisplayInfo(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	sf := getSurfaceComposerProxy(ctx, t, driver)

	physIds, err := sf.GetPhysicalDisplayIds(ctx)
	requireOrSkip(t, err)
	require.NotEmpty(t, physIds)

	// StaticDisplayInfo is a nested parcelable containing:
	//   ConnectionType (enum), Density (float32), Secure (bool),
	//   DeviceProductInfo (parcelable with nested fields), InstallOrientation (enum).
	info, err := sf.GetStaticDisplayInfo(ctx, physIds[0])
	requireOrSkip(t, err)
	// Some devices (e.g., API 36 Pixel) report density=0 from
	// StaticDisplayInfo; the density is reported via other APIs instead.
	if info.Density == 0 {
		t.Logf("note: density=0 (reported via other APIs on this device)")
	}
	t.Logf("StaticDisplayInfo: connectionType=%d, density=%f, secure=%v, installOrientation=%d",
		info.ConnectionType, info.Density, info.Secure, info.InstallOrientation)
}

func TestWindowSurface_SurfaceComposer_GetDynamicDisplayInfo(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	sf := getSurfaceComposerProxy(ctx, t, driver)

	physIds, err := sf.GetPhysicalDisplayIds(ctx)
	requireOrSkip(t, err)
	require.NotEmpty(t, physIds)

	// DynamicDisplayInfo contains arrays of parcelables (DisplayMode[]),
	// nested parcelable (HdrCapabilities), plus scalar/bool fields.
	// This is one of the most complex parcelable structures in the codebase.
	info, err := sf.GetDynamicDisplayInfoFromId(ctx, physIds[0])
	requireOrSkip(t, err)
	require.NotEmpty(t, info.SupportedDisplayModes, "should have at least one display mode")
	assert.GreaterOrEqual(t, info.ActiveDisplayModeId, int32(0))
	assert.Greater(t, info.RenderFrameRate, float32(0), "render frame rate should be > 0")
	require.NotEmpty(t, info.SupportedColorModes, "should have at least one color mode")

	t.Logf("DynamicDisplayInfo: activeMode=%d, renderFps=%f, colorModes=%v, autoLLM=%v, gameContent=%v",
		info.ActiveDisplayModeId, info.RenderFrameRate, info.SupportedColorModes,
		info.AutoLowLatencyModeSupported, info.GameContentTypeSupported)
	t.Logf("  display modes: %d, HDR capabilities present", len(info.SupportedDisplayModes))
}

func TestWindowSurface_SurfaceComposer_GetDisplayState(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	sf := getSurfaceComposerProxy(ctx, t, driver)

	physIds, err := sf.GetPhysicalDisplayIds(ctx)
	requireOrSkip(t, err)
	require.NotEmpty(t, physIds)

	token, err := sf.GetPhysicalDisplayToken(ctx, physIds[0])
	requireOrSkip(t, err)
	require.NotNil(t, token)

	// DisplayState contains LayerStack, Orientation (enum), and
	// LayerStackSpaceRect (Size parcelable with nested fields).
	state, err := sf.GetDisplayState(ctx, token)
	requireOrSkip(t, err)
	t.Logf("DisplayState: layerStack=%d, orientation=%d, rect={width=%d, height=%d}",
		state.LayerStack, state.Orientation,
		state.LayerStackSpaceRect.Width, state.LayerStackSpaceRect.Height)
}

func TestWindowSurface_SurfaceComposer_GetDisplayStats(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	sf := getSurfaceComposerProxy(ctx, t, driver)

	physIds, err := sf.GetPhysicalDisplayIds(ctx)
	requireOrSkip(t, err)
	require.NotEmpty(t, physIds)

	token, err := sf.GetPhysicalDisplayToken(ctx, physIds[0])
	requireOrSkip(t, err)
	require.NotNil(t, token)

	// DisplayStatInfo contains VsyncTime and VsyncPeriod (int64).
	stats, err := sf.GetDisplayStats(ctx, token)
	requireOrSkip(t, err)
	assert.Greater(t, stats.VsyncPeriod, int64(0), "vsync period should be > 0")
	t.Logf("DisplayStats: vsyncTime=%d, vsyncPeriod=%d", stats.VsyncTime, stats.VsyncPeriod)
}

func TestWindowSurface_SurfaceComposer_GetDisplayNativePrimaries(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	sf := getSurfaceComposerProxy(ctx, t, driver)

	physIds, err := sf.GetPhysicalDisplayIds(ctx)
	requireOrSkip(t, err)
	require.NotEmpty(t, physIds)

	token, err := sf.GetPhysicalDisplayToken(ctx, physIds[0])
	requireOrSkip(t, err)
	require.NotNil(t, token)

	// DisplayPrimaries contains 4 nested CieXyz parcelables (red, green, blue, white).
	// This deeply exercises nested parcelable unmarshaling.
	primaries, err := sf.GetDisplayNativePrimaries(ctx, token)
	requireOrSkip(t, err)
	t.Logf("DisplayNativePrimaries: red={X=%f,Y=%f,Z=%f}, green={X=%f,Y=%f,Z=%f}, blue={X=%f,Y=%f,Z=%f}, white={X=%f,Y=%f,Z=%f}",
		primaries.Red.X, primaries.Red.Y, primaries.Red.Z,
		primaries.Green.X, primaries.Green.Y, primaries.Green.Z,
		primaries.Blue.X, primaries.Blue.Y, primaries.Blue.Z,
		primaries.White.X, primaries.White.Y, primaries.White.Z)
}

func TestWindowSurface_SurfaceComposer_GetCompositionPreference(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	sf := getSurfaceComposerProxy(ctx, t, driver)

	// CompositionPreference contains 4 int32 fields (dataspace + pixel format
	// for default and wide-color-gamut).
	pref, err := sf.GetCompositionPreference(ctx)
	requireOrSkip(t, err)
	t.Logf("CompositionPreference: defaultDataspace=%d, defaultPixelFormat=%d, wideGamutDataspace=%d, wideGamutPixelFormat=%d",
		pref.DefaultDataspace, pref.DefaultPixelFormat,
		pref.WideColorGamutDataspace, pref.WideColorGamutPixelFormat)
}

func TestWindowSurface_SurfaceComposer_GetDisplayBrightnessSupport(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	sf := getSurfaceComposerProxy(ctx, t, driver)

	physIds, err := sf.GetPhysicalDisplayIds(ctx)
	requireOrSkip(t, err)
	require.NotEmpty(t, physIds)

	token, err := sf.GetPhysicalDisplayToken(ctx, physIds[0])
	requireOrSkip(t, err)
	require.NotNil(t, token)

	supported, err := sf.GetDisplayBrightnessSupport(ctx, token)
	requireOrSkip(t, err)
	t.Logf("display brightness support: %v", supported)
}

func TestWindowSurface_SurfaceComposer_GetHdrConversionCapabilities(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	sf := getSurfaceComposerProxy(ctx, t, driver)

	// Returns an array of HdrConversionCapability parcelables, each containing
	// SourceType, OutputType, AddsLatency. Tests array-of-parcelable unmarshal.
	caps, err := sf.GetHdrConversionCapabilities(ctx)
	// Server-side IllegalState: HDR conversion is device-dependent and not
	// supported on all hardware (e.g. phones without HDR output capability).
	if err != nil && strings.Contains(err.Error(), "exception IllegalState") {
		t.Skipf("HDR conversion not supported on this device (not a library bug): %v", err)
	}
	requireOrSkip(t, err)
	t.Logf("HDR conversion capabilities: %d entries", len(caps))
	for i, c := range caps {
		if i >= 5 {
			t.Logf("  ... and %d more", len(caps)-5)
			break
		}
		t.Logf("  [%d] sourceType=%d, outputType=%d, addsLatency=%v",
			i, c.SourceType, c.OutputType, c.AddsLatency)
	}
}

func TestWindowSurface_SurfaceComposer_IsWideColorDisplay(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	sf := getSurfaceComposerProxy(ctx, t, driver)

	physIds, err := sf.GetPhysicalDisplayIds(ctx)
	requireOrSkip(t, err)
	require.NotEmpty(t, physIds)

	token, err := sf.GetPhysicalDisplayToken(ctx, physIds[0])
	requireOrSkip(t, err)
	require.NotNil(t, token)

	wide, err := sf.IsWideColorDisplay(ctx, token)
	requireOrSkip(t, err)
	t.Logf("isWideColorDisplay: %v", wide)
}

func TestWindowSurface_SurfaceComposer_GetStalledTransactionInfo(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	sf := getSurfaceComposerProxy(ctx, t, driver)

	// Query stalled transaction info for our own pid. This exercises
	// nullable parcelable return handling.
	info, err := sf.GetStalledTransactionInfo(ctx, int32(os.Getpid()))
	requireOrSkip(t, err)
	_ = info
	t.Logf("getStalledTransactionInfo(pid=%d) completed", os.Getpid())
}

func TestWindowSurface_SurfaceComposer_GetDesiredDisplayModeSpecs(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	sf := getSurfaceComposerProxy(ctx, t, driver)

	physIds, err := sf.GetPhysicalDisplayIds(ctx)
	requireOrSkip(t, err)
	require.NotEmpty(t, physIds)

	token, err := sf.GetPhysicalDisplayToken(ctx, physIds[0])
	requireOrSkip(t, err)
	require.NotNil(t, token)

	// DisplayModeSpecs is a parcelable with nested structures.
	specs, err := sf.GetDesiredDisplayModeSpecs(ctx, token)
	if err != nil && strings.Contains(err.Error(), "read beyond end") {
		t.Skipf("method not implemented on this device: %v", err)
	}
	requireOrSkip(t, err)
	t.Logf("DisplayModeSpecs: defaultMode=%d, allowGroupSwitching=%v",
		specs.DefaultMode, specs.AllowGroupSwitching)
}

func TestWindowSurface_SurfaceComposer_GetDisplayDecorationSupport(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	sf := getSurfaceComposerProxy(ctx, t, driver)

	physIds, err := sf.GetPhysicalDisplayIds(ctx)
	requireOrSkip(t, err)
	require.NotEmpty(t, physIds)

	token, err := sf.GetPhysicalDisplayToken(ctx, physIds[0])
	requireOrSkip(t, err)
	require.NotNil(t, token)

	// DisplayDecorationSupport contains Format + AlphaInterpretation.
	decor, err := sf.GetDisplayDecorationSupport(ctx, token)
	requireOrSkip(t, err)
	t.Logf("SurfaceComposer DisplayDecorationSupport: format=%d, alphaInterpretation=%d",
		decor.Format, decor.AlphaInterpretation)
}

func TestWindowSurface_SurfaceComposer_CaptureLayersSync(t *testing.T) {
	ctx := context.Background()
	driver := windowSurfaceOpenBinder(t)
	sf := getSurfaceComposerProxy(ctx, t, driver)

	// Get a physical display ID so we can create a mirror layer.
	physIds, err := sf.GetPhysicalDisplayIds(ctx)
	requireOrSkip(t, err)
	require.NotEmpty(t, physIds, "need at least one physical display")

	// Create a mirror-display layer. MirrorDisplay works through the
	// generated proxy (unlike createSurface which panics on nil parent),
	// and returns a CreateSurfaceResult with a valid layer Handle.
	client, err := sf.CreateConnection(ctx)
	requireOrSkip(t, err)
	require.NotNil(t, client)

	mirror, err := client.MirrorDisplay(ctx, physIds[0])
	requireOrSkip(t, err)
	require.NotNil(t, mirror.Handle, "mirror layer handle should not be nil")
	t.Logf("created mirror layer: layerId=%d, name=%q, handle=%d",
		mirror.LayerId, mirror.LayerName, mirror.Handle.Handle())

	// Capture the mirror layer, which mirrors the physical display content.
	args := genGui.LayerCaptureArgs{
		CaptureArgs: genGui.CaptureArgs{
			PixelFormat: 1, // RGBA_8888
			FrameScaleX: 1.0,
			FrameScaleY: 1.0,
			Uid:         -1, // UNSET
		},
		LayerHandle: mirror.Handle,
	}

	result, err := sf.CaptureLayersSync(ctx, args)
	if err != nil {
		errStr := err.Error()
		// Permission denied or security error: environment limitation.
		if strings.Contains(errStr, "Permission") ||
			strings.Contains(errStr, "SecurityException") {
			t.Skipf("capture denied (permission): %v", err)
		}
		requireOrSkip(t, err)
	}

	buf := result.Buffer
	if buf == nil {
		t.Logf("capture returned nil buffer (effect layer has no content)")
		return
	}

	assert.Greater(t, buf.Width, uint32(0), "buffer width should be > 0")
	assert.Greater(t, buf.Height, uint32(0), "buffer height should be > 0")
	t.Logf("capture buffer: %dx%d, format=%d, stride=%d, usage=0x%x",
		buf.Width, buf.Height, buf.Format, buf.Stride, buf.Usage)
	t.Logf("capture metadata: secureLayers=%v, hdrLayers=%v, dataspace=%d, hdrSdrRatio=%f",
		result.CapturedSecureLayers, result.CapturedHdrLayers,
		result.CapturedDataspace, result.HdrSdrRatio)

	// Try to mmap + read pixels from the buffer.
	mmapErr := buf.Mmap()
	if mmapErr != nil {
		t.Logf("Mmap failed (may be expected on some allocators): %v", mmapErr)
	} else {
		defer buf.Munmap()
	}

	pixels, err := buf.ReadPixels()
	if err != nil {
		t.Logf("ReadPixels failed (not a library bug if mapper unavailable): %v", err)
		return
	}

	totalBytes := len(pixels)
	nonZero := 0
	for _, b := range pixels {
		if b != 0 {
			nonZero++
		}
	}
	pct := float64(0)
	if totalBytes > 0 {
		pct = float64(nonZero) / float64(totalBytes) * 100.0
	}
	t.Logf("pixel stats: size=%d bytes, non-zero=%.1f%%", totalBytes, pct)
}
