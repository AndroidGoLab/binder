//go:build e2e

package e2e

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	genAccounts "github.com/xaionaro-go/binder/android/accounts"
	genApp "github.com/xaionaro-go/binder/android/app"
	genBackup "github.com/xaionaro-go/binder/android/app/backup"
	genJob "github.com/xaionaro-go/binder/android/app/job"
	genSlice "github.com/xaionaro-go/binder/android/app/slice"
	genUsage "github.com/xaionaro-go/binder/android/app/usage"
	genContent "github.com/xaionaro-go/binder/android/content"
	genIntegrity "github.com/xaionaro-go/binder/android/content/integrity"
	genPm "github.com/xaionaro-go/binder/android/content/pm"
	genCredentials "github.com/xaionaro-go/binder/android/credentials"
	genDeviceState "github.com/xaionaro-go/binder/android/hardware/devicestate"
	genDisplay "github.com/xaionaro-go/binder/android/hardware/display"
	genFingerprint "github.com/xaionaro-go/binder/android/hardware/fingerprint"
	genLocation "github.com/xaionaro-go/binder/android/location"
	genMidi "github.com/xaionaro-go/binder/android/media/midi"
	genSession "github.com/xaionaro-go/binder/android/media/session"
	genNet "github.com/xaionaro-go/binder/android/net"
	genOs "github.com/xaionaro-go/binder/android/os"
	genPrint "github.com/xaionaro-go/binder/android/print"
	genDreams "github.com/xaionaro-go/binder/android/service/dreams"
	genAccessibility "github.com/xaionaro-go/binder/android/view/accessibility"
	"github.com/xaionaro-go/binder/servicemanager"
	genInternalTelecom "github.com/xaionaro-go/binder/com/android/internal_/telecom"
)

// --- content: android.content.IContentService ---

func TestSubsystem_Content(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "content")

	proxy := genContent.NewContentServiceProxy(svc)
	result, err := proxy.GetMasterSyncAutomatically(ctx)
	requireOrSkip(t, err)
	t.Logf("getMasterSyncAutomatically: %v", result)
}

// --- search: android.app.ISearchManager ---

func TestSubsystem_Search(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "search")

	proxy := genApp.NewSearchManagerProxy(svc)
	result, err := proxy.GetGlobalSearchActivities(ctx)
	if err != nil {
		t.Logf("getGlobalSearchActivities returned error (may require permission): %v", err)
	} else {
		t.Logf("getGlobalSearchActivities: %d activities", len(result))
	}
}

// --- telecom: com.android.internal.telecom.ITelecomService ---

func TestSubsystem_Telecom(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "telecom")

	proxy := genInternalTelecom.NewTelecomServiceProxy(svc)
	result, err := proxy.GetDefaultDialerPackage(ctx)
	if err != nil {
		t.Logf("getDefaultDialerPackage returned error (may require permission): %v", err)
	} else {
		t.Logf("getDefaultDialerPackage: %q", result)
	}
}

// --- credential: android.credentials.ICredentialManager ---

func TestSubsystem_Security_Credential(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "credential")

	proxy := genCredentials.NewCredentialManagerProxy(svc)
	result, err := proxy.IsServiceEnabled(ctx)
	requireOrSkip(t, err)
	t.Logf("isServiceEnabled: %v", result)
}

// --- fingerprint: android.hardware.fingerprint.IFingerprintService ---

func TestSubsystem_Security_Fingerprint(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "fingerprint")

	proxy := genFingerprint.NewFingerprintServiceProxy(svc)
	result, err := proxy.IsHardwareDetected(ctx, 0)
	if err != nil {
		// Requires USE_BIOMETRIC_INTERNAL on some devices/emulators.
		t.Skipf("isHardwareDetected requires biometric permission: %v", err)
	}
	t.Logf("isHardwareDetected(sensorId=0): %v", result)
}

// --- wallpaper: android.app.IWallpaperManager ---

func TestSubsystem_Wallpaper(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "wallpaper")

	proxy := genApp.NewWallpaperManagerProxy(svc)
	result, err := proxy.IsWallpaperSupported(ctx)
	requireOrSkip(t, err)
	t.Logf("isWallpaperSupported: %v", result)
}

// --- dreams: android.service.dreams.IDreamManager ---

func TestSubsystem_Dreams(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "dreams")

	proxy := genDreams.NewDreamManagerProxy(svc)
	result, err := proxy.IsDreaming(ctx)
	requireOrSkip(t, err)
	t.Logf("isDreaming: %v", result)
}

// --- uimode: android.app.IUiModeManager ---

func TestSubsystem_UiMode(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "uimode")

	proxy := genApp.NewUiModeManagerProxy(svc)
	result, err := proxy.GetCurrentModeType(ctx)
	requireOrSkip(t, err)
	t.Logf("getCurrentModeType: %d", result)
}

// --- color_display: android.hardware.display.IColorDisplayManager ---

func TestSubsystem_ColorDisplay(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "color_display")

	proxy := genDisplay.NewColorDisplayManagerProxy(svc)

	activated, err := proxy.IsNightDisplayActivated(ctx)
	requireOrSkip(t, err)
	t.Logf("isNightDisplayActivated: %v", activated)

	colorMode, err := proxy.GetColorMode(ctx)
	requireOrSkip(t, err)
	t.Logf("getColorMode: %d", colorMode)
}

// --- device_state: android.hardware.devicestate.IDeviceStateManager ---

func TestSubsystem_DeviceState(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "device_state")

	proxy := genDeviceState.NewDeviceStateManagerProxy(svc)
	result, err := proxy.GetDeviceStateInfo(ctx)
	requireOrSkip(t, err)
	t.Logf("getDeviceStateInfo: %v", result)
}

// --- deviceidle: android.os.IDeviceIdleController ---

func TestSubsystem_DeviceIdle(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "deviceidle")

	proxy := genOs.NewDeviceIdleControllerProxy(svc)
	result, err := proxy.GetFullPowerWhitelist(ctx)
	requireOrSkip(t, err)
	t.Logf("getFullPowerWhitelist: %d entries", len(result))
}

// --- alarm: android.app.IAlarmManager ---

func TestSubsystem_Alarm(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "alarm")

	proxy := genApp.NewAlarmManagerProxy(svc)
	result, err := proxy.GetNextAlarmClock(ctx)
	requireOrSkip(t, err)
	t.Logf("getNextAlarmClock: %v", result)
}

// --- country_detector: android.location.ICountryDetector ---

func TestSubsystem_CountryDetector(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "country_detector")

	proxy := genLocation.NewCountryDetectorProxy(svc)
	result, err := proxy.DetectCountry(ctx)
	requireOrSkip(t, err)
	t.Logf("detectCountry: %v", result)
}

// --- autofill: android.view.autofill.IAutoFillManager (ping via IsAlive) ---

func TestSubsystem_Autofill(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "autofill")

	alive := svc.IsAlive(ctx)
	t.Logf("autofill alive: %v, handle: %d", alive, svc.Handle())
}

// --- accessibility: android.view.accessibility.IAccessibilityManager ---

func TestSubsystem_Accessibility(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "accessibility")

	proxy := genAccessibility.NewAccessibilityManagerProxy(svc)
	result, err := proxy.GetAccessibilityShortcutTargets(ctx, 0)
	if err != nil {
		t.Logf("getAccessibilityShortcutTargets returned error (may require permission): %v", err)
	} else {
		t.Logf("getAccessibilityShortcutTargets: %d targets", len(result))
	}
}

// --- account: android.accounts.IAccountManager ---

func TestSubsystem_Accounts(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "account")

	proxy := genAccounts.NewAccountManagerProxy(svc)
	result, err := proxy.GetAuthenticatorTypes(ctx)
	if err != nil {
		t.Logf("getAuthenticatorTypes returned error (may require permission): %v", err)
	} else {
		t.Logf("getAuthenticatorTypes: %d types", len(result))
	}
}

// --- package: android.content.pm.IPackageManager ---

func TestSubsystem_PackageManager(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "package")

	proxy := genPm.NewPackageManagerProxy(svc)
	result, err := proxy.IsPackageAvailable(ctx, "com.android.settings")
	requireOrSkip(t, err)
	t.Logf("isPackageAvailable(com.android.settings): %v", result)
}

// --- app_integrity: android.content.integrity.IAppIntegrityManager ---

func TestSubsystem_AppIntegrity(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "app_integrity")

	proxy := genIntegrity.NewAppIntegrityManagerProxy(svc)
	result, err := proxy.GetCurrentRuleSetVersion(ctx)
	if err != nil {
		t.Logf("getCurrentRuleSetVersion returned error (may require permission): %v", err)
	} else {
		t.Logf("getCurrentRuleSetVersion: %q", result)
	}
}

// --- usagestats: android.app.usage.IUsageStatsManager ---

func TestSubsystem_UsageStats(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "usagestats")

	proxy := genUsage.NewUsageStatsManagerProxy(svc)
	result, err := proxy.IsAppStandbyEnabled(ctx)
	requireOrSkip(t, err)
	t.Logf("isAppStandbyEnabled: %v", result)
}

// --- connectivity: android.net.IConnectivityManager ---
// IConnectivityManager is a Java-only AIDL interface not in the version
// tables, so we cannot resolve transaction codes. Use CheckService + IsAlive
// instead (same approach as the WiFi test in device_features_test.go).

func TestSubsystem_Connectivity(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)

	svc, err := sm.CheckService(ctx, "connectivity")
	requireOrSkip(t, err)
	if svc == nil {
		t.Skip("connectivity service not registered")
	}
	require.True(t, svc.IsAlive(ctx), "connectivity service should be alive")
	t.Logf("connectivity service: alive, handle=%d", svc.Handle())
}

// --- netpolicy: android.net.INetworkPolicyManager ---

func TestSubsystem_NetworkPolicy(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "netpolicy")

	proxy := genNet.NewNetworkPolicyManagerProxy(svc)
	result, err := proxy.GetRestrictBackground(ctx)
	requireOrSkip(t, err)
	t.Logf("getRestrictBackground: %v", result)
}

// --- jobscheduler: android.app.job.IJobScheduler (ping via IsAlive) ---

func TestSubsystem_JobScheduler(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "jobscheduler")

	_ = genJob.NewJobSchedulerProxy(svc)
	alive := svc.IsAlive(ctx)
	t.Logf("jobscheduler alive: %v, handle: %d", alive, svc.Handle())
}

// --- backup: android.app.backup.IBackupManager ---

func TestSubsystem_Backup(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "backup")

	proxy := genBackup.NewBackupManagerProxy(svc)
	result, err := proxy.IsBackupEnabled(ctx)
	if err != nil {
		t.Logf("isBackupEnabled returned error (may require permission): %v", err)
	} else {
		t.Logf("isBackupEnabled: %v", result)
	}
}

// --- print: android.print.IPrintManager ---

func TestSubsystem_Print(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "print")

	proxy := genPrint.NewPrintManagerProxy(svc)
	result, err := proxy.GetPrintJobInfos(ctx, -1)
	if err != nil {
		t.Logf("getPrintJobInfos returned error (may require permission): %v", err)
	} else {
		t.Logf("getPrintJobInfos: %d jobs", len(result))
	}
}

// --- midi: android.media.midi.IMidiManager ---

func TestSubsystem_Midi(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "midi")

	proxy := genMidi.NewMidiManagerProxy(svc)
	result, err := proxy.GetDevices(ctx)
	requireOrSkip(t, err)
	t.Logf("getDevices: %d devices", len(result))
}

// --- media_session: android.media.session.ISessionManager ---

func TestSubsystem_MediaSession(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "media_session")

	proxy := genSession.NewSessionManagerProxy(svc)
	result, err := proxy.IsGlobalPriorityActive(ctx)
	requireOrSkip(t, err)
	t.Logf("isGlobalPriorityActive: %v", result)
}

// --- game: android.app.IGameManagerService ---

func TestSubsystem_GameMode(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "game")

	proxy := genApp.NewGameManagerServiceProxy(svc)
	result, err := proxy.GetGameMode(ctx, "com.android.settings")
	if err != nil {
		t.Logf("getGameMode returned error (may require permission): %v", err)
	} else {
		t.Logf("getGameMode(com.android.settings): %d", result)
	}
}

// --- slice: android.app.slice.ISliceManager ---

func TestSubsystem_Slice(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "slice")

	proxy := genSlice.NewSliceManagerProxy(svc)
	result, err := proxy.HasSliceAccess(ctx, "com.android.settings")
	if err != nil {
		t.Logf("hasSliceAccess returned error (may require permission): %v", err)
	} else {
		t.Logf("hasSliceAccess(com.android.settings): %v", result)
	}
}
