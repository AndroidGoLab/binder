//go:build e2e

package e2e

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xaionaro-go/binder/binder"
	genApp "github.com/xaionaro-go/binder/android/app"
	genContent "github.com/xaionaro-go/binder/android/content"
	genGui "github.com/xaionaro-go/binder/android/gui"
	genNet "github.com/xaionaro-go/binder/android/net"
	genOs "github.com/xaionaro-go/binder/android/os"
	"github.com/xaionaro-go/binder/servicemanager"
)

// --- ServiceManager generated proxy (via handle 0) ---

func TestGenProxy_ServiceManager_IsDeclared(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)

	smBinder := binder.NewProxyBinder(driver, binder.DefaultCallerIdentity(), 0)
	smProxy := genOs.NewServiceManagerProxy(smBinder)

	// SurfaceFlinger is a native (non-AIDL) service, so isDeclared returns
	// false on most devices. We only verify the RPC round-trip succeeds.
	declared, err := smProxy.IsDeclared(ctx, "SurfaceFlinger")
	requireOrSkip(t, err)
	t.Logf("IsDeclared(SurfaceFlinger): %v", declared)

	// Looking up non-existent services may trigger SELinux denial.
	notDeclared, err := smProxy.IsDeclared(ctx, "definitely.does.not.exist.99999")
	if err != nil {
		t.Logf("IsDeclared(non-existent) returned error (SELinux): %v", err)
	} else {
		assert.False(t, notDeclared, "non-existent service should not be declared")
	}
}

// --- PowerManager ---

func TestGenProxy_PowerManager_IsPowerSaveMode(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "power")

	proxy := genOs.NewPowerManagerProxy(svc)
	result, err := proxy.IsPowerSaveMode(ctx)
	requireOrSkip(t, err)
	t.Logf("IsPowerSaveMode: %v", result)
}

func TestGenProxy_PowerManager_IsInteractive(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "power")

	proxy := genOs.NewPowerManagerProxy(svc)
	result, err := proxy.IsInteractive(ctx)
	requireOrSkip(t, err)
	t.Logf("IsInteractive: %v", result)
}

func TestGenProxy_PowerManager_IsDeviceIdleMode(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "power")

	proxy := genOs.NewPowerManagerProxy(svc)
	result, err := proxy.IsDeviceIdleMode(ctx)
	requireOrSkip(t, err)
	t.Logf("IsDeviceIdleMode: %v", result)
}

func TestGenProxy_PowerManager_IsLightDeviceIdleMode(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "power")

	proxy := genOs.NewPowerManagerProxy(svc)
	result, err := proxy.IsLightDeviceIdleMode(ctx)
	requireOrSkip(t, err)
	t.Logf("IsLightDeviceIdleMode: %v", result)
}

func TestGenProxy_PowerManager_IsBatterySaverSupported(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "power")

	proxy := genOs.NewPowerManagerProxy(svc)
	result, err := proxy.IsBatterySaverSupported(ctx)
	requireOrSkip(t, err)
	t.Logf("IsBatterySaverSupported: %v", result)
}

func TestGenProxy_PowerManager_IsWakeLockLevelSupported(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "power")

	proxy := genOs.NewPowerManagerProxy(svc)
	// PARTIAL_WAKE_LOCK = 1
	result, err := proxy.IsWakeLockLevelSupported(ctx, 1)
	requireOrSkip(t, err)
	assert.True(t, result, "PARTIAL_WAKE_LOCK should be supported")
	t.Logf("IsWakeLockLevelSupported(PARTIAL_WAKE_LOCK): %v", result)
}

func TestGenProxy_PowerManager_AreAutoPowerSaveModesEnabled(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "power")

	proxy := genOs.NewPowerManagerProxy(svc)
	result, err := proxy.AreAutoPowerSaveModesEnabled(ctx)
	requireOrSkip(t, err)
	t.Logf("AreAutoPowerSaveModesEnabled: %v", result)
}

// --- ActivityManager ---

func TestGenProxy_ActivityManager_IsUserAMonkey(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "activity")

	proxy := genApp.NewActivityManagerProxy(svc)
	result, err := proxy.IsUserAMonkey(ctx)
	requireOrSkip(t, err)
	assert.False(t, result, "should not be a monkey in test")
	t.Logf("IsUserAMonkey: %v", result)
}

func TestGenProxy_ActivityManager_GetProcessLimit(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "activity")

	proxy := genApp.NewActivityManagerProxy(svc)
	result, err := proxy.GetProcessLimit(ctx)
	requireOrSkip(t, err)
	assert.GreaterOrEqual(t, result, int32(-1), "process limit should be >= -1")
	t.Logf("GetProcessLimit: %d", result)
}

func TestGenProxy_ActivityManager_IsAppFreezerSupported(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "activity")

	proxy := genApp.NewActivityManagerProxy(svc)
	result, err := proxy.IsAppFreezerSupported(ctx)
	requireOrSkip(t, err)
	t.Logf("IsAppFreezerSupported: %v", result)
}

func TestGenProxy_ActivityManager_CheckPermission(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "activity")

	proxy := genApp.NewActivityManagerProxy(svc)

	// 0 = PERMISSION_GRANTED, -1 = PERMISSION_DENIED.
	result, err := proxy.CheckPermission(ctx, "android.permission.INTERNET", int32(os.Getpid()), 0)
	requireOrSkip(t, err)
	assert.Equal(t, int32(0), result, "root should have INTERNET permission")
	t.Logf("CheckPermission(INTERNET, pid=%d, uid=0): %d", os.Getpid(), result)
}

func TestGenProxy_ActivityManager_GetCurrentUserId(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "activity")

	proxy := genApp.NewActivityManagerProxy(svc)
	result, err := proxy.GetCurrentUserId(ctx)
	requireOrSkip(t, err)
	assert.GreaterOrEqual(t, result, int32(0), "user ID should be non-negative")
	t.Logf("GetCurrentUserId: %d", result)
}

// --- SurfaceComposer (SurfaceFlingerAIDL) ---

func TestGenProxy_SurfaceComposer_GetBootDisplayModeSupport(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "SurfaceFlingerAIDL")

	proxy := genGui.NewSurfaceComposerProxy(svc)
	result, err := proxy.GetBootDisplayModeSupport(ctx)
	requireOrSkip(t, err)
	t.Logf("GetBootDisplayModeSupport: %v", result)
}

func TestGenProxy_SurfaceComposer_GetPhysicalDisplayIds(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "SurfaceFlingerAIDL")

	proxy := genGui.NewSurfaceComposerProxy(svc)
	ids, err := proxy.GetPhysicalDisplayIds(ctx)
	requireOrSkip(t, err)
	require.NotEmpty(t, ids, "expected at least one physical display")
	assert.NotZero(t, ids[0], "first display ID should be non-zero")
	t.Logf("GetPhysicalDisplayIds: %v", ids)
}

func TestGenProxy_SurfaceComposer_GetStaticDisplayInfo(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "SurfaceFlingerAIDL")

	proxy := genGui.NewSurfaceComposerProxy(svc)

	ids, err := proxy.GetPhysicalDisplayIds(ctx)
	requireOrSkip(t, err)
	require.NotEmpty(t, ids)

	// GetStaticDisplayInfo takes a display ID and returns a parcelable struct.
	// This exercises the typed proxy with both input args and structured return.
	//
	// Known issue: the density reads as 0 because SurfaceFlinger prepends a
	// stability marker (int32) before the standard parcelable header, but the
	// generated UnmarshalParcel does not skip it. The codegen needs to emit a
	// stability read for @VintfStability parcelables. We verify the RPC
	// round-trip succeeds without transport errors.
	info, err := proxy.GetStaticDisplayInfo(ctx, ids[0])
	requireOrSkip(t, err)
	t.Logf("GetStaticDisplayInfo(displayId=%d): connectionType=%d, density=%f, secure=%v",
		ids[0], info.ConnectionType, info.Density, info.Secure)
}

func TestGenProxy_SurfaceComposer_GetHdrOutputConversionSupport(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "SurfaceFlingerAIDL")

	proxy := genGui.NewSurfaceComposerProxy(svc)
	result, err := proxy.GetHdrOutputConversionSupport(ctx)
	requireOrSkip(t, err)
	t.Logf("GetHdrOutputConversionSupport: %v", result)
}

// --- ThermalService ---

func TestGenProxy_ThermalService_GetCurrentThermalStatus(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "thermalservice")

	proxy := genOs.NewThermalServiceProxy(svc)
	result, err := proxy.GetCurrentThermalStatus(ctx)
	requireOrSkip(t, err)
	assert.GreaterOrEqual(t, result, int32(0), "thermal status should be non-negative")
	t.Logf("GetCurrentThermalStatus: %d", result)
}

func TestGenProxy_ThermalService_GetThermalHeadroom(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "thermalservice")

	proxy := genOs.NewThermalServiceProxy(svc)
	result, err := proxy.GetThermalHeadroom(ctx, 10)
	requireOrSkip(t, err)
	assert.NotZero(t, result, "thermal headroom should be non-zero")
	t.Logf("GetThermalHeadroom(10s): %f", result)
}

// --- VibratorManagerService ---

func TestGenProxy_VibratorManager_GetVibratorIds(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "vibrator_manager")

	proxy := genOs.NewVibratorManagerServiceProxy(svc)
	ids, err := proxy.GetVibratorIds(ctx)
	requireOrSkip(t, err)
	assert.NotNil(t, ids, "vibrator IDs slice should not be nil")
	t.Logf("GetVibratorIds: %v (count: %d)", ids, len(ids))
}

// TestGenProxy_VibratorManager_GetCapabilities was removed because the
// GetCapabilities method no longer exists in the multi-version-aware
// IVibratorManagerService interface (it was present only in API 36).

// --- DeviceIdleController ---

func TestGenProxy_DeviceIdleController_GetFullPowerWhitelist(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "deviceidle")

	proxy := genOs.NewDeviceIdleControllerProxy(svc)
	apps, err := proxy.GetFullPowerWhitelist(ctx)
	requireOrSkip(t, err)
	require.NotEmpty(t, apps, "expected at least one whitelisted app")
	t.Logf("GetFullPowerWhitelist: %d apps", len(apps))
	for i, app := range apps {
		if i < 5 {
			t.Logf("  [%d] %s", i, app)
		}
	}
}

func TestGenProxy_DeviceIdleController_IsPowerSaveWhitelistApp(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "deviceidle")

	proxy := genOs.NewDeviceIdleControllerProxy(svc)
	result, err := proxy.IsPowerSaveWhitelistApp(ctx, "com.android.shell")
	requireOrSkip(t, err)
	assert.True(t, result, "com.android.shell should be whitelisted")
	t.Logf("IsPowerSaveWhitelistApp(com.android.shell): %v", result)
}

// --- NetworkPolicyManager ---

func TestGenProxy_NetworkPolicyManager_GetRestrictBackground(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "netpolicy")

	proxy := genNet.NewNetworkPolicyManagerProxy(svc)
	result, err := proxy.GetRestrictBackground(ctx)
	requireOrSkip(t, err)
	t.Logf("GetRestrictBackground: %v", result)
}

// --- Clipboard ---

func TestGenProxy_Clipboard_HasClipboardText(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "clipboard")

	proxy := genContent.NewClipboardProxy(svc)
	result, err := proxy.HasClipboardText(ctx, 0)
	requireOrSkip(t, err)
	t.Logf("HasClipboardText: %v", result)
}

// --- Cross-proxy: verify AsBinder() returns the original IBinder ---

func TestGenProxy_AsBinder(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "power")

	proxy := genOs.NewPowerManagerProxy(svc)
	assert.Equal(t, svc.Handle(), proxy.AsBinder().Handle(),
		"AsBinder().Handle() should match the original binder handle")
	t.Logf("power handle: %d, proxy.AsBinder().Handle(): %d", svc.Handle(), proxy.AsBinder().Handle())
}

// --- Multi-service summary: call one method on each of 10 distinct services ---

func TestGenProxy_MultiService(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)

	type serviceTest struct {
		serviceName string
		description string
		testFunc    func(t *testing.T, svc binder.IBinder)
	}

	tests := []serviceTest{
		{
			serviceName: "",
			description: "ServiceManager.IsDeclared",
			testFunc: func(t *testing.T, _ binder.IBinder) {
				smBinder := binder.NewProxyBinder(driver, binder.DefaultCallerIdentity(), 0)
				proxy := genOs.NewServiceManagerProxy(smBinder)
				// SurfaceFlinger is a native (non-AIDL) service, so isDeclared
				// may return false. We only verify the RPC round-trip succeeds.
				declared, err := proxy.IsDeclared(ctx, "SurfaceFlinger")
				requireOrSkip(t, err)
				t.Logf("  IsDeclared(SurfaceFlinger): %v", declared)
			},
		},
		{
			serviceName: "power",
			description: "PowerManager.IsPowerSaveMode",
			testFunc: func(t *testing.T, svc binder.IBinder) {
				proxy := genOs.NewPowerManagerProxy(svc)
				val, err := proxy.IsPowerSaveMode(ctx)
				requireOrSkip(t, err)
				t.Logf("  IsPowerSaveMode: %v", val)
			},
		},
		{
			serviceName: "activity",
			description: "ActivityManager.IsUserAMonkey",
			testFunc: func(t *testing.T, svc binder.IBinder) {
				proxy := genApp.NewActivityManagerProxy(svc)
				val, err := proxy.IsUserAMonkey(ctx)
				requireOrSkip(t, err)
				t.Logf("  IsUserAMonkey: %v", val)
			},
		},
		{
			serviceName: "SurfaceFlingerAIDL",
			description: "SurfaceComposer.GetGpuContextPriority",
			testFunc: func(t *testing.T, svc binder.IBinder) {
				proxy := genGui.NewSurfaceComposerProxy(svc)
				val, err := proxy.GetGpuContextPriority(ctx)
				requireOrSkip(t, err)
				t.Logf("  GetGpuContextPriority: %d", val)
			},
		},
		{
			serviceName: "thermalservice",
			description: "ThermalService.GetCurrentThermalStatus",
			testFunc: func(t *testing.T, svc binder.IBinder) {
				proxy := genOs.NewThermalServiceProxy(svc)
				val, err := proxy.GetCurrentThermalStatus(ctx)
				requireOrSkip(t, err)
				t.Logf("  GetCurrentThermalStatus: %d", val)
			},
		},
		{
			serviceName: "vibrator_manager",
			description: "VibratorManagerService.GetVibratorIds",
			testFunc: func(t *testing.T, svc binder.IBinder) {
				proxy := genOs.NewVibratorManagerServiceProxy(svc)
				val, err := proxy.GetVibratorIds(ctx)
				requireOrSkip(t, err)
				t.Logf("  GetVibratorIds: %v", val)
			},
		},
		{
			serviceName: "deviceidle",
			description: "DeviceIdleController.GetFullPowerWhitelist",
			testFunc: func(t *testing.T, svc binder.IBinder) {
				proxy := genOs.NewDeviceIdleControllerProxy(svc)
				val, err := proxy.GetFullPowerWhitelist(ctx)
				requireOrSkip(t, err)
				t.Logf("  whitelist apps: %d", len(val))
			},
		},
		{
			serviceName: "netpolicy",
			description: "NetworkPolicyManager.GetRestrictBackground",
			testFunc: func(t *testing.T, svc binder.IBinder) {
				proxy := genNet.NewNetworkPolicyManagerProxy(svc)
				val, err := proxy.GetRestrictBackground(ctx)
				requireOrSkip(t, err)
				t.Logf("  GetRestrictBackground: %v", val)
			},
		},
		{
			serviceName: "clipboard",
			description: "Clipboard.HasClipboardText",
			testFunc: func(t *testing.T, svc binder.IBinder) {
				proxy := genContent.NewClipboardProxy(svc)
				val, err := proxy.HasClipboardText(ctx, 0)
				requireOrSkip(t, err)
				t.Logf("  HasClipboardText: %v", val)
			},
		},
		{
			serviceName: "performance_hint",
			description: "HintManager.GetHintSessionPreferredRate",
			testFunc: func(t *testing.T, svc binder.IBinder) {
				proxy := genOs.NewHintManagerProxy(svc)
				val, err := proxy.GetHintSessionPreferredRate(ctx)
				requireOrSkip(t, err)
				t.Logf("  GetHintSessionPreferredRate: %d", val)
			},
		},
	}

	sm := servicemanager.New(driver)

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			var svc binder.IBinder
			if tt.serviceName != "" {
				var err error
				svc, err = sm.GetService(ctx, servicemanager.ServiceName(tt.serviceName))
				requireOrSkip(t, err)
				require.NotNil(t, svc)
			}
			tt.testFunc(t, svc)
		})
	}
}
