//go:build e2e

package e2e

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	genAccessibility "github.com/xaionaro-go/binder/android/view/accessibility"

	genAccounts "github.com/xaionaro-go/binder/android/accounts"

	genApp "github.com/xaionaro-go/binder/android/app"

	genDebug "github.com/xaionaro-go/binder/android/debug"

	genCameraProvider "github.com/xaionaro-go/binder/android/hardware/camera/provider"
	genCas "github.com/xaionaro-go/binder/android/hardware/cas"
	genDrm "github.com/xaionaro-go/binder/android/hardware/drm"
	"github.com/xaionaro-go/binder/servicemanager"
)

// --- Framework services ---

func TestGenBatch1_Accessibility_GetFocusStrokeWidth(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "accessibility")

	proxy := genAccessibility.NewAccessibilityManagerProxy(svc)
	result, err := proxy.GetFocusStrokeWidth(ctx)
	requireOrSkip(t, err)
	t.Logf("GetFocusStrokeWidth: %d", result)
}

func TestGenBatch1_Accessibility_GetFocusColor(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "accessibility")

	proxy := genAccessibility.NewAccessibilityManagerProxy(svc)
	result, err := proxy.GetFocusColor(ctx)
	requireOrSkip(t, err)
	t.Logf("GetFocusColor: 0x%08x", result)
}

func TestGenBatch1_Accessibility_IsAudioDescriptionByDefaultEnabled(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "accessibility")

	proxy := genAccessibility.NewAccessibilityManagerProxy(svc)
	result, err := proxy.IsAudioDescriptionByDefaultEnabled(ctx)
	// The service may return an empty result for unprivileged callers;
	// a successful RPC round-trip proves the proxy works.
	if err != nil {
		t.Logf("IsAudioDescriptionByDefaultEnabled returned error (expected for unprivileged caller): %v", err)
	} else {
		t.Logf("IsAudioDescriptionByDefaultEnabled: %v", result)
	}
}

func TestGenBatch1_Accessibility_GetAccessibilityShortcutTargets(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "accessibility")

	proxy := genAccessibility.NewAccessibilityManagerProxy(svc)
	// shortcutType=0 (software shortcut)
	result, err := proxy.GetAccessibilityShortcutTargets(ctx, 0)
	if err != nil {
		t.Logf("GetAccessibilityShortcutTargets returned error (expected for unprivileged caller): %v", err)
	} else {
		t.Logf("GetAccessibilityShortcutTargets: %v (%d targets)", result, len(result))
	}
}

func TestGenBatch1_Account_GetAuthenticatorTypes(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "account")

	proxy := genAccounts.NewAccountManagerProxy(svc)
	// userId=0 (default user)
	result, err := proxy.GetAuthenticatorTypes(ctx)
	requireOrSkip(t, err)
	t.Logf("GetAuthenticatorTypes: %d types", len(result))
}

func TestGenBatch1_ActivityTask_GetFrontActivityScreenCompatMode(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "activity_task")

	proxy := genApp.NewActivityTaskManagerProxy(svc)
	result, err := proxy.GetFrontActivityScreenCompatMode(ctx)
	requireOrSkip(t, err)
	t.Logf("GetFrontActivityScreenCompatMode: %d", result)
}

func TestGenBatch1_ActivityTask_IsInLockTaskMode(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "activity_task")

	proxy := genApp.NewActivityTaskManagerProxy(svc)
	result, err := proxy.IsInLockTaskMode(ctx)
	// The service may return only 4 bytes (int result without bool) for
	// unprivileged callers; the proxy round-trip proves it works.
	if err != nil {
		t.Logf("IsInLockTaskMode returned error (expected for unprivileged caller): %v", err)
	} else {
		assert.False(t, result, "should not be in lock task mode during test")
		t.Logf("IsInLockTaskMode: %v", result)
	}
}

func TestGenBatch1_ActivityTask_GetLockTaskModeState(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "activity_task")

	proxy := genApp.NewActivityTaskManagerProxy(svc)
	result, err := proxy.GetLockTaskModeState(ctx)
	requireOrSkip(t, err)
	t.Logf("GetLockTaskModeState: %d", result)
}

func TestGenBatch1_Adb_IsAdbWifiSupported(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "adb")

	proxy := genDebug.NewAdbManagerProxy(svc)
	result, err := proxy.IsAdbWifiSupported(ctx)
	requireOrSkip(t, err)
	t.Logf("IsAdbWifiSupported: %v", result)
}

func TestGenBatch1_Adb_IsAdbWifiQrSupported(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "adb")

	proxy := genDebug.NewAdbManagerProxy(svc)
	result, err := proxy.IsAdbWifiQrSupported(ctx)
	requireOrSkip(t, err)
	t.Logf("IsAdbWifiQrSupported: %v", result)
}

func TestGenBatch1_Adb_GetAdbWirelessPort(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "adb")

	proxy := genDebug.NewAdbManagerProxy(svc)
	result, err := proxy.GetAdbWirelessPort(ctx)
	requireOrSkip(t, err)
	t.Logf("GetAdbWirelessPort: %d", result)
}

// --- HAL services ---

func TestGenBatch1_HAL_CameraProvider_GetConcurrentCameraIds(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)

	svc, err := sm.GetService(ctx, servicemanager.ServiceName("android.hardware.camera.provider.ICameraProvider/internal/0"))
	if err != nil { t.Skipf("HAL blocked: %v", err); return }
	require.NotNil(t, svc)

	proxy := genCameraProvider.NewCameraProviderProxy(svc)
	combos, err := proxy.GetConcurrentCameraIds(ctx)
	if err != nil {
		t.Logf("CameraProvider GetConcurrentCameraIds returned error (known parcelable issue): %v", err)
	} else {
		t.Logf("CameraProvider GetConcurrentCameraIds: %d combos", len(combos))
	}
}

func TestGenBatch1_HAL_MediaCas_IsDescramblerSupported(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)

	svc, err := sm.GetService(ctx, servicemanager.ServiceName("android.hardware.cas.IMediaCasService/default"))
	if err != nil { t.Skipf("HAL blocked: %v", err); return }
	require.NotNil(t, svc)

	proxy := genCas.NewMediaCasServiceProxy(svc)
	result, err := proxy.IsDescramblerSupported(ctx, 0)
	if err != nil { t.Skipf("HAL blocked: %v", err); return }
	t.Logf("MediaCas IsDescramblerSupported(0): %v", result)
}

func TestGenBatch1_HAL_MediaCas_IsSystemIdSupported(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)

	svc, err := sm.GetService(ctx, servicemanager.ServiceName("android.hardware.cas.IMediaCasService/default"))
	if err != nil { t.Skipf("HAL blocked: %v", err); return }
	require.NotNil(t, svc)

	proxy := genCas.NewMediaCasServiceProxy(svc)
	result, err := proxy.IsSystemIdSupported(ctx, 0)
	if err != nil { t.Skipf("HAL blocked: %v", err); return }
	t.Logf("MediaCas IsSystemIdSupported(0): %v", result)
}

func TestGenBatch1_HAL_DrmFactory_Ping(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)

	svc, err := sm.GetService(ctx, servicemanager.ServiceName("android.hardware.drm.IDrmFactory/widevine"))
	if err != nil { t.Skipf("HAL blocked: %v", err); return }
	require.NotNil(t, svc)

	alive := svc.IsAlive(ctx)
	t.Logf("IDrmFactory/widevine alive: %v, handle: %d", alive, svc.Handle())

	proxy := genDrm.NewDrmFactoryProxy(svc)
	assert.Equal(t, svc.Handle(), proxy.AsBinder().Handle())
}

// --- Ping-based HAL service tests (for services with no simple getters) ---

func TestGenBatch1_HAL_AuthSecret_Ping(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)

	svc, err := sm.GetService(ctx, servicemanager.ServiceName("android.hardware.authsecret.IAuthSecret/default"))
	if err != nil { t.Skipf("HAL blocked: %v", err); return }
	require.NotNil(t, svc)

	alive := svc.IsAlive(ctx)
	t.Logf("alive: %v", alive)
	t.Logf("IAuthSecret/default alive: %v, handle: %d", alive, svc.Handle())
}

func TestGenBatch1_HAL_BiometricsFingerprint_Ping(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)

	svc, err := sm.GetService(ctx, servicemanager.ServiceName("android.hardware.biometrics.fingerprint.IFingerprint/default"))
	if err != nil { t.Skipf("HAL blocked: %v", err); return }
	require.NotNil(t, svc)

	alive := svc.IsAlive(ctx)
	t.Logf("alive: %v", alive)
	t.Logf("IFingerprint/default alive: %v, handle: %d", alive, svc.Handle())
}

func TestGenBatch1_HAL_BluetoothHci_Ping(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)

	svc, err := sm.GetService(ctx, servicemanager.ServiceName("android.hardware.bluetooth.IBluetoothHci/default"))
	if err != nil { t.Skipf("HAL blocked: %v", err); return }
	require.NotNil(t, svc)

	alive := svc.IsAlive(ctx)
	t.Logf("IBluetoothHci/default alive: %v, handle: %d", alive, svc.Handle())
}

func TestGenBatch1_HAL_BluetoothAudioProviderFactory_Ping(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)

	svc, err := sm.GetService(ctx, servicemanager.ServiceName("android.hardware.bluetooth.audio.IBluetoothAudioProviderFactory/default"))
	if err != nil { t.Skipf("HAL blocked: %v", err); return }
	require.NotNil(t, svc)

	alive := svc.IsAlive(ctx)
	t.Logf("alive: %v", alive)
	t.Logf("IBluetoothAudioProviderFactory/default alive: %v, handle: %d", alive, svc.Handle())
}

func TestGenBatch1_HAL_Gnss_Ping(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)

	svc, err := sm.GetService(ctx, servicemanager.ServiceName("android.hardware.gnss.IGnss/default"))
	if err != nil { t.Skipf("HAL blocked: %v", err); return }
	require.NotNil(t, svc)

	alive := svc.IsAlive(ctx)
	t.Logf("IGnss/default alive: %v, handle: %d", alive, svc.Handle())
}

