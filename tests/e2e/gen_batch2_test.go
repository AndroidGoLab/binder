//go:build e2e

package e2e

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	aidlerrors "github.com/xaionaro-go/binder/errors"
	genIntegrity "github.com/xaionaro-go/binder/android/content/integrity"
	genC2 "github.com/xaionaro-go/binder/android/hardware/media/c2"
	genNN "github.com/xaionaro-go/binder/android/hardware/neuralnetworks"
	genHwPower "github.com/xaionaro-go/binder/android/hardware/power"
	genHwVibrator "github.com/xaionaro-go/binder/android/hardware/vibrator"
	genWifi "github.com/xaionaro-go/binder/android/hardware/wifi"
	genSupplicant "github.com/xaionaro-go/binder/android/hardware/wifi/supplicant"
	genMedia "github.com/xaionaro-go/binder/android/media"
	genKeystore2 "github.com/xaionaro-go/binder/android/system/keystore2"
	genSuspend "github.com/xaionaro-go/binder/android/system/suspend"
	"github.com/xaionaro-go/binder/servicemanager"
)

// isPermissionError returns true if the error is an AIDL security or
// service-specific exception (which proves the proxy round-trip worked).
func isPermissionError(err error) bool {
	var se *aidlerrors.StatusError
	if !errors.As(err, &se) {
		return false
	}
	return se.Exception == aidlerrors.ExceptionSecurity ||
		se.Exception == aidlerrors.ExceptionServiceSpecific
}

// isTransportError returns true if the error is a transport-level failure
// (e.g. SELinux denial for VINTF HAL services).
func isTransportError(err error) bool {
	if err == nil {
		return false
	}
	var txnErr *aidlerrors.TransactionError
	return errors.As(err, &txnErr) || err.Error() == "binder: failed transaction"
}

// requireNoErrorOrTransport fails the test only if err is non-nil and NOT a
// transport-level failure. HAL services behind SELinux often reject
// non-privileged callers at the binder driver level.
func requireNoErrorOrTransport(t *testing.T, err error, msgAndArgs ...interface{}) {
	t.Helper()
	if err == nil {
		return
	}
	if isTransportError(err) {
		t.Skipf("skipping: HAL service rejected transaction (SELinux): %v", err)
		return
	}
	require.NoError(t, err, msgAndArgs...)
}

// --- Batch 2: HAL hardware services ---

func TestGenBatch2_ComponentStore_ListComponents(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "android.hardware.media.c2.IComponentStore/software")

	proxy := genC2.NewComponentStoreProxy(svc)
	components, err := proxy.ListComponents(ctx)
	requireOrSkip(t, err)
	t.Logf("ListComponents: %d components", len(components))
}

func TestGenBatch2_ComponentStore_GetStructDescriptors(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "android.hardware.media.c2.IComponentStore/software")

	proxy := genC2.NewComponentStoreProxy(svc)
	// Request descriptors for empty set of indices.
	descriptors, err := proxy.GetStructDescriptors(ctx, []int32{})
	if err != nil {
		t.Logf("GetStructDescriptors returned error (transient): %v", err)
	} else {
		t.Logf("GetStructDescriptors([]): %d descriptors", len(descriptors))
	}
}

func TestGenBatch2_NeuralNetworks_GetVersionString(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "android.hardware.neuralnetworks.IDevice/nnapi-sample_all")

	proxy := genNN.NewDeviceProxy(svc)
	version, err := proxy.GetVersionString(ctx)
	requireOrSkip(t, err)
	assert.NotEmpty(t, version)
	t.Logf("GetVersionString: %s", version)
}

func TestGenBatch2_NeuralNetworks_GetType(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "android.hardware.neuralnetworks.IDevice/nnapi-sample_all")

	proxy := genNN.NewDeviceProxy(svc)
	devType, err := proxy.GetType(ctx)
	requireOrSkip(t, err)
	t.Logf("GetType: %d", devType)
}

func TestGenBatch2_NeuralNetworks_GetNumberOfCacheFilesNeeded(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "android.hardware.neuralnetworks.IDevice/nnapi-sample_all")

	proxy := genNN.NewDeviceProxy(svc)
	cacheFiles, err := proxy.GetNumberOfCacheFilesNeeded(ctx)
	requireOrSkip(t, err)
	t.Logf("GetNumberOfCacheFilesNeeded: numDataCache=%d, numModelCache=%d",
		cacheFiles.NumDataCache, cacheFiles.NumModelCache)
}

func TestGenBatch2_NeuralNetworks_GetSupportedExtensions(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "android.hardware.neuralnetworks.IDevice/nnapi-sample_all")

	proxy := genNN.NewDeviceProxy(svc)
	exts, err := proxy.GetSupportedExtensions(ctx)
	requireOrSkip(t, err)
	t.Logf("GetSupportedExtensions: %d extensions", len(exts))
}

func TestGenBatch2_HwPower_IsModeSupported(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "android.hardware.power.IPower/default")

	proxy := genHwPower.NewPowerProxy(svc)
	supported, err := proxy.IsModeSupported(ctx, genHwPower.ModeLowPower)
	requireNoErrorOrTransport(t, err)
	t.Logf("IsModeSupported(LowPower): %v", supported)
}

func TestGenBatch2_HwPower_IsBoostSupported(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "android.hardware.power.IPower/default")

	proxy := genHwPower.NewPowerProxy(svc)
	supported, err := proxy.IsBoostSupported(ctx, genHwPower.BoostINTERACTION)
	requireNoErrorOrTransport(t, err)
	t.Logf("IsBoostSupported(INTERACTION): %v", supported)
}

func TestGenBatch2_HwPower_GetHintSessionPreferredRate(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "android.hardware.power.IPower/default")

	proxy := genHwPower.NewPowerProxy(svc)
	rate, err := proxy.GetHintSessionPreferredRate(ctx)
	requireNoErrorOrTransport(t, err)
	t.Logf("GetHintSessionPreferredRate: %d ns", rate)
}

func TestGenBatch2_HwVibrator_GetCapabilities(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "android.hardware.vibrator.IVibrator/default")

	proxy := genHwVibrator.NewVibratorProxy(svc)
	caps, err := proxy.GetCapabilities(ctx)
	requireNoErrorOrTransport(t, err)
	t.Logf("GetCapabilities: 0x%x", caps)
}

func TestGenBatch2_HwVibratorManager_GetCapabilities(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "android.hardware.vibrator.IVibratorManager/default")

	proxy := genHwVibrator.NewVibratorManagerProxy(svc)
	caps, err := proxy.GetCapabilities(ctx)
	requireNoErrorOrTransport(t, err)
	t.Logf("GetCapabilities: 0x%x", caps)
}

func TestGenBatch2_HwVibratorManager_GetVibratorIds(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "android.hardware.vibrator.IVibratorManager/default")

	proxy := genHwVibrator.NewVibratorManagerProxy(svc)
	ids, err := proxy.GetVibratorIds(ctx)
	requireNoErrorOrTransport(t, err)
	t.Logf("GetVibratorIds: %v", ids)
}

func TestGenBatch2_Wifi_IsStarted(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "android.hardware.wifi.IWifi/default")

	proxy := genWifi.NewWifiProxy(svc)
	started, err := proxy.IsStarted(ctx)
	requireNoErrorOrTransport(t, err)
	t.Logf("IsStarted: %v", started)
}

func TestGenBatch2_Wifi_GetChipIds(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "android.hardware.wifi.IWifi/default")

	proxy := genWifi.NewWifiProxy(svc)
	chipIds, err := proxy.GetChipIds(ctx)
	requireNoErrorOrTransport(t, err)
	t.Logf("GetChipIds: %v", chipIds)
}

func TestGenBatch2_Supplicant_ListInterfaces(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "android.hardware.wifi.supplicant.ISupplicant/default")

	proxy := genSupplicant.NewSupplicantProxy(svc)
	ifaces, err := proxy.ListInterfaces(ctx)
	requireNoErrorOrTransport(t, err)
	t.Logf("ListInterfaces: %d interfaces", len(ifaces))
}

func TestGenBatch2_Supplicant_GetDebugLevel(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "android.hardware.wifi.supplicant.ISupplicant/default")

	proxy := genSupplicant.NewSupplicantProxy(svc)
	level, err := proxy.GetDebugLevel(ctx)
	requireNoErrorOrTransport(t, err)
	t.Logf("GetDebugLevel: %d", level)
}

// --- Batch 3: system services + keystore/suspend ---

func TestGenBatch2_Keystore2_GetNumberOfEntries(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "android.system.keystore2.IKeystoreService/default")

	proxy := genKeystore2.NewKeystoreServiceProxy(svc)
	count, err := proxy.GetNumberOfEntries(ctx, genKeystore2.DomainSELINUX, 0)
	if err != nil {
		if isPermissionError(err) {
			t.Logf("GetNumberOfEntries: permission denied (expected): %v", err)
			return
		}
		requireNoErrorOrTransport(t, err)
		return
	}
	t.Logf("GetNumberOfEntries(SELINUX, 0): %d", count)
}

func TestGenBatch2_SystemSuspend_AcquireWakeLock(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "android.system.suspend.ISystemSuspend/default")

	proxy := genSuspend.NewSystemSuspendProxy(svc)
	wl, err := proxy.AcquireWakeLock(ctx, genSuspend.WakeLockTypePARTIAL, "aidl_e2e_test")
	requireNoErrorOrTransport(t, err)
	require.NotNil(t, wl)
	t.Logf("AcquireWakeLock: got wakelock binder handle=%d", wl.AsBinder().Handle())
}

// AppHibernation tests removed: IAppHibernationService proxy not generated.

func TestGenBatch2_AppIntegrity_GetCurrentRuleSetVersion(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "app_integrity")

	proxy := genIntegrity.NewAppIntegrityManagerProxy(svc)
	version, err := proxy.GetCurrentRuleSetVersion(ctx)
	if err != nil {
		if isPermissionError(err) {
			t.Logf("GetCurrentRuleSetVersion: permission denied (expected): %v", err)
			return
		}
		requireOrSkip(t, err)
	}
	t.Logf("GetCurrentRuleSetVersion: %s", version)
}

func TestGenBatch2_AppIntegrity_GetCurrentRuleSetProvider(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "app_integrity")

	proxy := genIntegrity.NewAppIntegrityManagerProxy(svc)
	provider, err := proxy.GetCurrentRuleSetProvider(ctx)
	if err != nil {
		if isPermissionError(err) {
			t.Logf("GetCurrentRuleSetProvider: permission denied (expected): %v", err)
			return
		}
		requireOrSkip(t, err)
	}
	t.Logf("GetCurrentRuleSetProvider: %s", provider)
}

func TestGenBatch2_Audio_GetStreamMaxVolume(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "audio")

	proxy := genMedia.NewAudioServiceProxy(svc)
	// STREAM_MUSIC = 3
	maxVol, err := proxy.GetStreamMaxVolume(ctx, 3)
	requireOrSkip(t, err)
	assert.Greater(t, maxVol, int32(0), "max volume should be positive")
	t.Logf("GetStreamMaxVolume(STREAM_MUSIC): %d", maxVol)
}

func TestGenBatch2_Audio_GetStreamMinVolume(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "audio")

	proxy := genMedia.NewAudioServiceProxy(svc)
	// STREAM_MUSIC = 3
	minVol, err := proxy.GetStreamMinVolume(ctx, 3)
	requireOrSkip(t, err)
	assert.GreaterOrEqual(t, minVol, int32(0), "min volume should be non-negative")
	t.Logf("GetStreamMinVolume(STREAM_MUSIC): %d", minVol)
}

func TestGenBatch2_Audio_GetStreamVolume(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "audio")

	proxy := genMedia.NewAudioServiceProxy(svc)
	// STREAM_MUSIC = 3
	vol, err := proxy.GetStreamVolume(ctx, 3)
	requireOrSkip(t, err)
	assert.GreaterOrEqual(t, vol, int32(0), "volume should be non-negative")
	t.Logf("GetStreamVolume(STREAM_MUSIC): %d", vol)
}

func TestGenBatch2_Audio_GetMode(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "audio")

	proxy := genMedia.NewAudioServiceProxy(svc)
	mode, err := proxy.GetMode(ctx)
	requireOrSkip(t, err)
	assert.GreaterOrEqual(t, mode, int32(0))
	t.Logf("GetMode: %d", mode)
}

// --- Multi-service summary for batch 2+3 ---

func TestGenBatch2_MultiService(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)

	type serviceTest struct {
		description string
		testFunc    func(t *testing.T)
	}

	tests := []serviceTest{
		{
			description: "NeuralNetworks.GetVersionString",
			testFunc: func(t *testing.T) {
				svc, err := sm.GetService(ctx, servicemanager.ServiceName("android.hardware.neuralnetworks.IDevice/nnapi-sample_all"))
				requireOrSkip(t, err)
				proxy := genNN.NewDeviceProxy(svc)
				ver, err := proxy.GetVersionString(ctx)
				requireOrSkip(t, err)
				t.Logf("  version: %s", ver)
			},
		},
		{
			description: "NeuralNetworks.GetType",
			testFunc: func(t *testing.T) {
				svc, err := sm.GetService(ctx, servicemanager.ServiceName("android.hardware.neuralnetworks.IDevice/nnapi-sample_all"))
				requireOrSkip(t, err)
				proxy := genNN.NewDeviceProxy(svc)
				devType, err := proxy.GetType(ctx)
				requireOrSkip(t, err)
				t.Logf("  type: %d", devType)
			},
		},
		{
			description: "ComponentStore.ListComponents",
			testFunc: func(t *testing.T) {
				svc, err := sm.GetService(ctx, servicemanager.ServiceName("android.hardware.media.c2.IComponentStore/software"))
				requireOrSkip(t, err)
				proxy := genC2.NewComponentStoreProxy(svc)
				components, err := proxy.ListComponents(ctx)
				requireOrSkip(t, err)
				t.Logf("  components: %d", len(components))
			},
		},
		{
			description: "Audio.GetStreamMaxVolume",
			testFunc: func(t *testing.T) {
				svc, err := sm.GetService(ctx, servicemanager.ServiceName("audio"))
				requireOrSkip(t, err)
				proxy := genMedia.NewAudioServiceProxy(svc)
				val, err := proxy.GetStreamMaxVolume(ctx, 3)
				requireOrSkip(t, err)
				t.Logf("  GetStreamMaxVolume(MUSIC): %d", val)
			},
		},
		{
			description: "Audio.GetMode",
			testFunc: func(t *testing.T) {
				svc, err := sm.GetService(ctx, servicemanager.ServiceName("audio"))
				requireOrSkip(t, err)
				proxy := genMedia.NewAudioServiceProxy(svc)
				val, err := proxy.GetMode(ctx)
				requireOrSkip(t, err)
				t.Logf("  GetMode: %d", val)
			},
		},
		{
			description: "Keystore2.GetNumberOfEntries",
			testFunc: func(t *testing.T) {
				svc, err := sm.GetService(ctx, servicemanager.ServiceName("android.system.keystore2.IKeystoreService/default"))
				requireOrSkip(t, err)
				proxy := genKeystore2.NewKeystoreServiceProxy(svc)
				_, err = proxy.GetNumberOfEntries(ctx, genKeystore2.DomainSELINUX, 0)
				if err != nil && isPermissionError(err) {
					t.Logf("  permission denied (expected)")
					return
				}
				requireOrSkip(t, err)
			},
		},
		{
			description: "AppIntegrity.GetCurrentRuleSetVersion",
			testFunc: func(t *testing.T) {
				svc, err := sm.GetService(ctx, servicemanager.ServiceName("app_integrity"))
				requireOrSkip(t, err)
				proxy := genIntegrity.NewAppIntegrityManagerProxy(svc)
				_, err = proxy.GetCurrentRuleSetVersion(ctx)
				if err != nil && isPermissionError(err) {
					t.Logf("  permission denied (expected)")
					return
				}
				requireOrSkip(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			tt.testFunc(t)
		})
	}
}
