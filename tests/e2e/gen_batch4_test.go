//go:build e2e

package e2e

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	genApp "github.com/xaionaro-go/binder/android/app"
	genAdmin "github.com/xaionaro-go/binder/android/app/admin"
	genBackup "github.com/xaionaro-go/binder/android/app/backup"
	genBlob "github.com/xaionaro-go/binder/android/app/blob"
	genCompanion "github.com/xaionaro-go/binder/android/companion"
	genContent "github.com/xaionaro-go/binder/android/content"
	genPm "github.com/xaionaro-go/binder/android/content/pm"
	genDomain "github.com/xaionaro-go/binder/android/content/pm/verify/domain"
	genCredentials "github.com/xaionaro-go/binder/android/credentials"
	genBiometrics "github.com/xaionaro-go/binder/android/hardware/biometrics"
	genDeviceState "github.com/xaionaro-go/binder/android/hardware/devicestate"
	genDisplay "github.com/xaionaro-go/binder/android/hardware/display"
	genFingerprint "github.com/xaionaro-go/binder/android/hardware/fingerprint"
	genInput "github.com/xaionaro-go/binder/android/hardware/input"
	genLights "github.com/xaionaro-go/binder/android/hardware/lights"
	genMemtrack "github.com/xaionaro-go/binder/android/hardware/memtrack"
	genLocation "github.com/xaionaro-go/binder/android/location"
	genMediaMetrics "github.com/xaionaro-go/binder/android/media/metrics"
	genMidi "github.com/xaionaro-go/binder/android/media/midi"
	genProjection "github.com/xaionaro-go/binder/android/media/projection"
	genSession "github.com/xaionaro-go/binder/android/media/session"
	genNet "github.com/xaionaro-go/binder/android/net"
	genOs "github.com/xaionaro-go/binder/android/os"
	genImage "github.com/xaionaro-go/binder/android/os/image"
	genStorage "github.com/xaionaro-go/binder/android/os/storage"
	genDreams "github.com/xaionaro-go/binder/android/service/dreams"
)

// --- auth: android.hardware.biometrics.IAuthService ---

func TestGenBatch4_Auth_CanAuthenticate(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "auth")

	proxy := genBiometrics.NewAuthServiceProxy(svc)
	result, err := proxy.CanAuthenticate(ctx, 0)
	if err != nil {
		t.Logf("CanAuthenticate returned error (may require permission): %v", err)
	} else {
		t.Logf("CanAuthenticate: %d", result)
	}
}

func TestGenBatch4_Auth_GetUiPackage(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "auth")

	proxy := genBiometrics.NewAuthServiceProxy(svc)
	result, err := proxy.GetUiPackage(ctx)
	if err != nil {
		t.Logf("GetUiPackage returned error (may require permission): %v", err)
	} else {
		t.Logf("GetUiPackage: %q", result)
	}
}

// --- autofill: android.view.autofill.IAutoFillManager ---

func TestGenBatch4_Autofill_Ping(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "autofill")

	alive := svc.IsAlive(ctx)
	t.Logf("autofill alive: %v, handle: %d", alive, svc.Handle())
}

// --- background_install_control: android.content.pm.IBackgroundInstallControlService ---

func TestGenBatch4_BackgroundInstallControl_GetBackgroundInstalledPackages(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "background_install_control")

	proxy := genPm.NewBackgroundInstallControlServiceProxy(svc)
	_, err := proxy.GetBackgroundInstalledPackages(ctx, 0)
	if err != nil {
		t.Logf("GetBackgroundInstalledPackages returned error (may require permission): %v", err)
	} else {
		t.Logf("GetBackgroundInstalledPackages succeeded")
	}
}

// --- backup: android.app.backup.IBackupManager ---

func TestGenBatch4_Backup_IsBackupEnabled(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "backup")

	proxy := genBackup.NewBackupManagerProxy(svc)
	result, err := proxy.IsBackupEnabled(ctx)
	if err != nil {
		t.Logf("IsBackupEnabled returned error (may require permission): %v", err)
	} else {
		t.Logf("IsBackupEnabled: %v", result)
	}
}

// --- batteryproperties: android.os.IBatteryPropertiesRegistrar ---

func TestGenBatch4_BatteryProperties_ScheduleUpdate(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "batteryproperties")

	proxy := genOs.NewBatteryPropertiesRegistrarProxy(svc)
	err := proxy.ScheduleUpdate(ctx)
	if err != nil {
		t.Logf("ScheduleUpdate returned error: %v", err)
	} else {
		t.Logf("ScheduleUpdate succeeded (oneway)")
	}
}

// --- biometric: android.hardware.biometrics.IBiometricService ---

func TestGenBatch4_Biometric_GetSupportedModalities(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "biometric")

	proxy := genBiometrics.NewBiometricServiceProxy(svc)
	result, err := proxy.GetSupportedModalities(ctx, 0)
	if err != nil {
		t.Logf("GetSupportedModalities returned error (may require permission): %v", err)
	} else {
		t.Logf("GetSupportedModalities: 0x%x", result)
	}
}

// --- blob_store: android.app.blob.IBlobStoreManager ---

func TestGenBatch4_BlobStore_GetRemainingLeaseQuotaBytes(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "blob_store")

	proxy := genBlob.NewBlobStoreManagerProxy(svc)
	result, err := proxy.GetRemainingLeaseQuotaBytes(ctx, "com.android.shell")
	if err != nil {
		t.Logf("GetRemainingLeaseQuotaBytes returned error: %v", err)
	} else {
		assert.Greater(t, result, int64(0), "quota should be positive")
		t.Logf("GetRemainingLeaseQuotaBytes: %d bytes", result)
	}
}

// --- bugreport: android.os.IDumpstate ---

func TestGenBatch4_Bugreport_Ping(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "bugreport")

	alive := svc.IsAlive(ctx)
	t.Logf("bugreport alive: %v, handle: %d", alive, svc.Handle())
}

// --- color_display: android.hardware.display.IColorDisplayManager ---

func TestGenBatch4_ColorDisplay_IsDeviceColorManaged(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "color_display")

	proxy := genDisplay.NewColorDisplayManagerProxy(svc)
	result, err := proxy.IsDeviceColorManaged(ctx)
	requireOrSkip(t, err)
	t.Logf("IsDeviceColorManaged: %v", result)
}

func TestGenBatch4_ColorDisplay_GetColorMode(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "color_display")

	proxy := genDisplay.NewColorDisplayManagerProxy(svc)
	result, err := proxy.GetColorMode(ctx)
	requireOrSkip(t, err)
	t.Logf("GetColorMode: %d", result)
}

func TestGenBatch4_ColorDisplay_IsNightDisplayActivated(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "color_display")

	proxy := genDisplay.NewColorDisplayManagerProxy(svc)
	result, err := proxy.IsNightDisplayActivated(ctx)
	requireOrSkip(t, err)
	t.Logf("IsNightDisplayActivated: %v", result)
}

func TestGenBatch4_ColorDisplay_GetNightDisplayAutoMode(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "color_display")

	proxy := genDisplay.NewColorDisplayManagerProxy(svc)
	result, err := proxy.GetNightDisplayAutoMode(ctx)
	requireOrSkip(t, err)
	t.Logf("GetNightDisplayAutoMode: %d", result)
}

func TestGenBatch4_ColorDisplay_GetTransformCapabilities(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "color_display")

	proxy := genDisplay.NewColorDisplayManagerProxy(svc)
	result, err := proxy.GetTransformCapabilities(ctx)
	requireOrSkip(t, err)
	t.Logf("GetTransformCapabilities: 0x%x", result)
}

// --- companiondevice: android.companion.ICompanionDeviceManager ---

func TestGenBatch4_CompanionDevice_GetAllAssociationsForUser(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "companiondevice")

	proxy := genCompanion.NewCompanionDeviceManagerProxy(svc)
	result, err := proxy.GetAllAssociationsForUser(ctx)
	if err != nil {
		t.Logf("GetAllAssociationsForUser returned error (may require permission): %v", err)
	} else {
		t.Logf("GetAllAssociationsForUser: %d associations", len(result))
	}
}

// --- connmetrics: android.net.IIpConnectivityMetrics ---

func TestGenBatch4_ConnMetrics_Ping(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "connmetrics")

	alive := svc.IsAlive(ctx)
	t.Logf("connmetrics alive: %v, handle: %d", alive, svc.Handle())
}

// --- content: android.content.IContentService ---

func TestGenBatch4_Content_GetMasterSyncAutomatically(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "content")

	proxy := genContent.NewContentServiceProxy(svc)
	result, err := proxy.GetMasterSyncAutomatically(ctx)
	requireOrSkip(t, err)
	t.Logf("GetMasterSyncAutomatically: %v", result)
}

func TestGenBatch4_Content_GetMasterSyncAutomaticallyAsUser(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "content")

	proxy := genContent.NewContentServiceProxy(svc)
	result, err := proxy.GetMasterSyncAutomaticallyAsUser(ctx)
	requireOrSkip(t, err)
	t.Logf("GetMasterSyncAutomaticallyAsUser(0): %v", result)
}

// --- credential: android.credentials.ICredentialManager ---

func TestGenBatch4_Credential_IsServiceEnabled(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "credential")

	proxy := genCredentials.NewCredentialManagerProxy(svc)
	result, err := proxy.IsServiceEnabled(ctx)
	requireOrSkip(t, err)
	t.Logf("IsServiceEnabled: %v", result)
}

// --- crossprofileapps: android.content.pm.ICrossProfileApps ---

func TestGenBatch4_CrossProfileApps_CanInteractAcrossProfiles(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "crossprofileapps")

	proxy := genPm.NewCrossProfileAppsProxy(svc)
	result, err := proxy.CanInteractAcrossProfiles(ctx)
	if err != nil {
		t.Logf("CanInteractAcrossProfiles returned error: %v", err)
	} else {
		t.Logf("CanInteractAcrossProfiles: %v", result)
	}
}

// --- dataloader_manager: android.content.pm.IDataLoaderManager ---

func TestGenBatch4_DataLoaderManager_Ping(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "dataloader_manager")

	alive := svc.IsAlive(ctx)
	t.Logf("dataloader_manager alive: %v, handle: %d", alive, svc.Handle())
}

// --- device_identifiers: android.os.IDeviceIdentifiersPolicyService ---

func TestGenBatch4_DeviceIdentifiers_GetSerial(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "device_identifiers")

	proxy := genOs.NewDeviceIdentifiersPolicyServiceProxy(svc)
	result, err := proxy.GetSerial(ctx)
	if err != nil {
		t.Logf("GetSerial returned error (may require permission): %v", err)
	} else {
		t.Logf("GetSerial: %q", result)
	}
}

// --- device_policy: android.app.admin.IDevicePolicyManager ---

func TestGenBatch4_DevicePolicy_IsDeviceProvisioned(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "device_policy")

	proxy := genAdmin.NewDevicePolicyManagerProxy(svc)
	result, err := proxy.IsDeviceProvisioned(ctx)
	if err != nil {
		t.Logf("IsDeviceProvisioned returned error (may require permission): %v", err)
	} else {
		assert.True(t, result, "emulator should be provisioned")
		t.Logf("IsDeviceProvisioned: %v", result)
	}
}

func TestGenBatch4_DevicePolicy_GetDeviceOwnerName(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "device_policy")

	proxy := genAdmin.NewDevicePolicyManagerProxy(svc)
	result, err := proxy.GetDeviceOwnerName(ctx)
	if err != nil {
		t.Logf("GetDeviceOwnerName returned error (may require permission): %v", err)
	} else {
		t.Logf("GetDeviceOwnerName: %q", result)
	}
}

func TestGenBatch4_DevicePolicy_GetAutoTimeRequired(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "device_policy")

	proxy := genAdmin.NewDevicePolicyManagerProxy(svc)
	result, err := proxy.GetAutoTimeRequired(ctx)
	if err != nil {
		t.Logf("GetAutoTimeRequired returned error (may require permission): %v", err)
	} else {
		t.Logf("GetAutoTimeRequired: %v", result)
	}
}

// --- device_state: android.hardware.devicestate.IDeviceStateManager ---

func TestGenBatch4_DeviceState_GetDeviceStateInfo(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "device_state")

	proxy := genDeviceState.NewDeviceStateManagerProxy(svc)
	_, err := proxy.GetDeviceStateInfo(ctx)
	if err != nil {
		t.Logf("GetDeviceStateInfo returned error (may require permission): %v", err)
	} else {
		t.Logf("GetDeviceStateInfo succeeded")
	}
}

// --- domain_verification: android.content.pm.verify.domain.IDomainVerificationManager ---

func TestGenBatch4_DomainVerification_QueryValidVerificationPackageNames(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "domain_verification")

	proxy := genDomain.NewDomainVerificationManagerProxy(svc)
	result, err := proxy.QueryValidVerificationPackageNames(ctx)
	if err != nil {
		t.Logf("QueryValidVerificationPackageNames returned error (may require permission): %v", err)
	} else {
		t.Logf("QueryValidVerificationPackageNames: %d packages", len(result))
	}
}

// --- dreams: android.service.dreams.IDreamManager ---

func TestGenBatch4_Dreams_IsDreaming(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "dreams")

	proxy := genDreams.NewDreamManagerProxy(svc)
	result, err := proxy.IsDreaming(ctx)
	requireOrSkip(t, err)
	t.Logf("IsDreaming: %v", result)
}

func TestGenBatch4_Dreams_IsDreamingOrInPreview(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "dreams")

	proxy := genDreams.NewDreamManagerProxy(svc)
	result, err := proxy.IsDreamingOrInPreview(ctx)
	requireOrSkip(t, err)
	t.Logf("IsDreamingOrInPreview: %v", result)
}

// --- dynamic_system: android.os.image.IDynamicSystemService ---

func TestGenBatch4_DynamicSystem_IsInUse(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "dynamic_system")

	proxy := genImage.NewDynamicSystemServiceProxy(svc)
	result, err := proxy.IsInUse(ctx)
	requireOrSkip(t, err)
	assert.False(t, result, "dynamic system should not be in use")
	t.Logf("IsInUse: %v", result)
}

func TestGenBatch4_DynamicSystem_IsInstalled(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "dynamic_system")

	proxy := genImage.NewDynamicSystemServiceProxy(svc)
	result, err := proxy.IsInstalled(ctx)
	requireOrSkip(t, err)
	t.Logf("IsInstalled: %v", result)
}

func TestGenBatch4_DynamicSystem_IsEnabled(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "dynamic_system")

	proxy := genImage.NewDynamicSystemServiceProxy(svc)
	result, err := proxy.IsEnabled(ctx)
	if err != nil {
		t.Logf("IsEnabled returned error (may require permission): %v", err)
	} else {
		t.Logf("IsEnabled: %v", result)
	}
}

// --- external_vibrator_service: android.os.IExternalVibratorService ---

func TestGenBatch4_ExternalVibrator_Ping(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "external_vibrator_service")

	alive := svc.IsAlive(ctx)
	t.Logf("external_vibrator_service alive: %v, handle: %d", alive, svc.Handle())
}

// --- feature_flags: android.flags.IFeatureFlags ---

func TestGenBatch4_FeatureFlags_Ping(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "feature_flags")

	alive := svc.IsAlive(ctx)
	t.Logf("feature_flags alive: %v, handle: %d", alive, svc.Handle())
}

// --- fingerprint: android.hardware.fingerprint.IFingerprintService ---

func TestGenBatch4_Fingerprint_IsHardwareDetected(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "fingerprint")

	proxy := genFingerprint.NewFingerprintServiceProxy(svc)
	result, err := proxy.IsHardwareDetected(ctx, 0)
	if err != nil {
		t.Logf("IsHardwareDetected returned error (may require permission): %v", err)
	} else {
		t.Logf("IsHardwareDetected: %v", result)
	}
}

// --- game: android.app.IGameManagerService ---

func TestGenBatch4_Game_GetGameMode(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "game")

	proxy := genApp.NewGameManagerServiceProxy(svc)
	result, err := proxy.GetGameMode(ctx, "com.android.shell")
	if err != nil {
		t.Logf("GetGameMode returned error: %v", err)
	} else {
		t.Logf("GetGameMode: %d", result)
	}
}

// --- grammatical_inflection: android.app.IGrammaticalInflectionManager ---

func TestGenBatch4_GrammaticalInflection_Ping(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "grammatical_inflection")

	alive := svc.IsAlive(ctx)
	t.Logf("grammatical_inflection alive: %v, handle: %d", alive, svc.Handle())
}

// --- graphicsstats: android.view.IGraphicsStats ---

func TestGenBatch4_GraphicsStats_Ping(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "graphicsstats")

	alive := svc.IsAlive(ctx)
	t.Logf("graphicsstats alive: %v, handle: %d", alive, svc.Handle())
}

// --- hardware_properties: android.os.IHardwarePropertiesManager ---

func TestGenBatch4_HardwareProperties_GetDeviceTemperatures(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "hardware_properties")

	proxy := genOs.NewHardwarePropertiesManagerProxy(svc)
	// type_=0 (CPU), source=0 (current)
	result, err := proxy.GetDeviceTemperatures(ctx, 0, 0)
	if err != nil {
		t.Logf("GetDeviceTemperatures returned error (may require permission): %v", err)
	} else {
		t.Logf("GetDeviceTemperatures: %v", result)
	}
}

// --- incidentcompanion: android.os.IIncidentCompanion ---

func TestGenBatch4_IncidentCompanion_GetPendingReports(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "incidentcompanion")

	proxy := genOs.NewIncidentCompanionProxy(svc)
	result, err := proxy.GetPendingReports(ctx)
	if err != nil {
		t.Logf("GetPendingReports returned error (may require permission): %v", err)
	} else {
		t.Logf("GetPendingReports: %d reports", len(result))
	}
}

// --- incremental: android.os.incremental.IIncrementalService ---

func TestGenBatch4_Incremental_Ping(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "incremental")

	alive := svc.IsAlive(ctx)
	t.Logf("incremental alive: %v, handle: %d", alive, svc.Handle())
}

// --- input: android.hardware.input.IInputManager ---

func TestGenBatch4_Input_GetInputDeviceIds(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "input")

	proxy := genInput.NewInputManagerProxy(svc)
	result, err := proxy.GetInputDeviceIds(ctx)
	requireOrSkip(t, err)
	require.NotEmpty(t, result, "expected at least one input device")
	t.Logf("GetInputDeviceIds: %v (%d devices)", result, len(result))
}

func TestGenBatch4_Input_GetMousePointerSpeed(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "input")

	proxy := genInput.NewInputManagerProxy(svc)
	result, err := proxy.GetMousePointerSpeed(ctx)
	requireOrSkip(t, err)
	t.Logf("GetMousePointerSpeed: %d", result)
}

// --- inputflinger: android.os.IInputFlinger ---

func TestGenBatch4_InputFlinger_Ping(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "inputflinger")

	alive := svc.IsAlive(ctx)
	t.Logf("inputflinger alive: %v, handle: %d", alive, svc.Handle())
}

// JobScheduler tests removed: IJobScheduler proxy not generated.

// --- launcherapps: android.content.pm.ILauncherApps ---

func TestGenBatch4_LauncherApps_Ping(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "launcherapps")

	alive := svc.IsAlive(ctx)
	t.Logf("launcherapps alive: %v, handle: %d", alive, svc.Handle())
}

// --- legacy_permission: android.permission.ILegacyPermissionManager ---

func TestGenBatch4_LegacyPermission_Ping(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "legacy_permission")

	alive := svc.IsAlive(ctx)
	t.Logf("legacy_permission alive: %v, handle: %d", alive, svc.Handle())
}

// --- lights: android.hardware.lights.ILightsManager ---

func TestGenBatch4_Lights_GetLights(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "lights")

	proxy := genLights.NewLightsManagerProxy(svc)
	result, err := proxy.GetLights(ctx)
	if err != nil {
		t.Logf("GetLights returned error (may require permission): %v", err)
	} else {
		t.Logf("GetLights: %d lights", len(result))
	}
}

// --- locale: android.app.ILocaleManager ---

func TestGenBatch4_Locale_GetSystemLocales(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "locale")

	proxy := genApp.NewLocaleManagerProxy(svc)
	_, err := proxy.GetSystemLocales(ctx)
	if err != nil {
		t.Logf("GetSystemLocales returned error: %v", err)
	} else {
		t.Logf("GetSystemLocales succeeded")
	}
}

// --- location: android.location.ILocationManager ---

func TestGenBatch4_Location_GetGnssYearOfHardware(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "location")

	proxy := genLocation.NewLocationManagerProxy(svc)
	result, err := proxy.GetGnssYearOfHardware(ctx)
	requireOrSkip(t, err)
	t.Logf("GetGnssYearOfHardware: %d", result)
}

func TestGenBatch4_Location_GetGnssHardwareModelName(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "location")

	proxy := genLocation.NewLocationManagerProxy(svc)
	result, err := proxy.GetGnssHardwareModelName(ctx)
	requireOrSkip(t, err)
	t.Logf("GetGnssHardwareModelName: %q", result)
}

func TestGenBatch4_Location_IsGeocodeAvailable(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "location")

	proxy := genLocation.NewLocationManagerProxy(svc)
	result, err := proxy.IsGeocodeAvailable(ctx)
	requireOrSkip(t, err)
	t.Logf("IsGeocodeAvailable: %v", result)
}

// --- logcat: android.os.logcat.ILogcatManagerService ---

func TestGenBatch4_Logcat_Ping(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "logcat")

	alive := svc.IsAlive(ctx)
	t.Logf("logcat alive: %v, handle: %d", alive, svc.Handle())
}

// --- manager: android.os.IServiceManager ---

func TestGenBatch4_Manager_IsDeclared(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "manager")

	proxy := genOs.NewServiceManagerProxy(svc)
	result, err := proxy.IsDeclared(ctx, "SurfaceFlinger")
	if err != nil {
		t.Logf("IsDeclared returned error: %v", err)
	} else {
		t.Logf("IsDeclared(SurfaceFlinger): %v", result)
	}
}

// --- media_metrics: android.media.metrics.IMediaMetricsManager ---

func TestGenBatch4_MediaMetrics_GetPlaybackSessionId(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "media_metrics")

	proxy := genMediaMetrics.NewMediaMetricsManagerProxy(svc)
	result, err := proxy.GetPlaybackSessionId(ctx)
	if err != nil {
		t.Logf("GetPlaybackSessionId returned error: %v", err)
	} else {
		t.Logf("GetPlaybackSessionId: %q", result)
	}
}

// --- media_projection: android.media.projection.IMediaProjectionManager ---

func TestGenBatch4_MediaProjection_HasProjectionPermission(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "media_projection")

	proxy := genProjection.NewMediaProjectionManagerProxy(svc)
	result, err := proxy.HasProjectionPermission(ctx, 0, "com.android.shell")
	if err != nil {
		t.Logf("HasProjectionPermission returned error: %v", err)
	} else {
		t.Logf("HasProjectionPermission: %v", result)
	}
}

// --- media_resource_monitor: android.media.IMediaResourceMonitor ---

func TestGenBatch4_MediaResourceMonitor_Ping(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "media_resource_monitor")

	alive := svc.IsAlive(ctx)
	t.Logf("media_resource_monitor alive: %v, handle: %d", alive, svc.Handle())
}

// --- media_router: android.media.IMediaRouterService ---

func TestGenBatch4_MediaRouter_Ping(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "media_router")

	alive := svc.IsAlive(ctx)
	t.Logf("media_router alive: %v, handle: %d", alive, svc.Handle())
}

// --- media_session: android.media.session.ISessionManager ---

func TestGenBatch4_MediaSession_IsGlobalPriorityActive(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "media_session")

	proxy := genSession.NewSessionManagerProxy(svc)
	result, err := proxy.IsGlobalPriorityActive(ctx)
	requireOrSkip(t, err)
	t.Logf("IsGlobalPriorityActive: %v", result)
}

// --- memtrack.proxy: android.hardware.memtrack.IMemtrack ---

func TestGenBatch4_Memtrack_GetGpuDeviceInfo(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "memtrack.proxy")

	proxy := genMemtrack.NewMemtrackProxy(svc)
	result, err := proxy.GetGpuDeviceInfo(ctx)
	if err != nil {
		t.Logf("GetGpuDeviceInfo returned error: %v", err)
	} else {
		t.Logf("GetGpuDeviceInfo: %d devices", len(result))
	}
}

// --- midi: android.media.midi.IMidiManager ---

func TestGenBatch4_Midi_GetDevices(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "midi")

	proxy := genMidi.NewMidiManagerProxy(svc)
	result, err := proxy.GetDevices(ctx)
	if err != nil {
		t.Logf("GetDevices returned error: %v", err)
	} else {
		t.Logf("GetDevices: %d devices", len(result))
	}
}

// --- mount: android.os.storage.IStorageManager ---

func TestGenBatch4_Mount_LastMaintenance(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "mount")

	proxy := genStorage.NewStorageManagerProxy(svc)
	result, err := proxy.LastMaintenance(ctx)
	if err != nil {
		t.Logf("LastMaintenance returned error (may differ in wire format): %v", err)
	} else {
		t.Logf("LastMaintenance: %d", result)
	}
}

// --- network_management: android.os.INetworkManagementService ---

func TestGenBatch4_NetworkManagement_IsBandwidthControlEnabled(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "network_management")

	proxy := genOs.NewNetworkManagementServiceProxy(svc)
	result, err := proxy.IsBandwidthControlEnabled(ctx)
	if err != nil {
		t.Logf("IsBandwidthControlEnabled returned error (may require permission): %v", err)
	} else {
		t.Logf("IsBandwidthControlEnabled: %v", result)
	}
}

func TestGenBatch4_NetworkManagement_ListInterfaces(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "network_management")

	proxy := genOs.NewNetworkManagementServiceProxy(svc)
	result, err := proxy.ListInterfaces(ctx)
	if err != nil {
		t.Logf("ListInterfaces returned error (may require permission): %v", err)
	} else {
		t.Logf("ListInterfaces: %v (%d interfaces)", result, len(result))
	}
}

// --- network_score: android.net.INetworkScoreService ---

func TestGenBatch4_NetworkScore_GetActiveScorerPackage(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "network_score")

	proxy := genNet.NewNetworkScoreServiceProxy(svc)
	result, err := proxy.GetActiveScorerPackage(ctx)
	if err != nil {
		t.Logf("GetActiveScorerPackage returned error: %v", err)
	} else {
		t.Logf("GetActiveScorerPackage: %q", result)
	}
}
