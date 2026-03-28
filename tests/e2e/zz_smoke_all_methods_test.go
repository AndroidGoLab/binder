//go:build e2e

package e2e

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/servicemanager"
	"github.com/AndroidGoLab/binder/tools/pkg/testutil"

	// Framework service proxies
	genAccessibility "github.com/AndroidGoLab/binder/android/view/accessibility"
	genAccounts "github.com/AndroidGoLab/binder/android/accounts"
	genAdmin "github.com/AndroidGoLab/binder/android/app/admin"
	genAmbientContext "github.com/AndroidGoLab/binder/android/app/ambientcontext"
	genApp "github.com/AndroidGoLab/binder/android/app"
	genAutofill "github.com/AndroidGoLab/binder/android/view/autofill"
	genBackup "github.com/AndroidGoLab/binder/android/app/backup"
	genBiometrics "github.com/AndroidGoLab/binder/android/hardware/biometrics"
	genBlob "github.com/AndroidGoLab/binder/android/app/blob"
	genCompanion "github.com/AndroidGoLab/binder/android/companion"
	genContent "github.com/AndroidGoLab/binder/android/content"
	genContentCapture "github.com/AndroidGoLab/binder/android/view/contentcapture"
	genContentSuggestions "github.com/AndroidGoLab/binder/android/app/contentsuggestions"
	genCredentials "github.com/AndroidGoLab/binder/android/credentials"
	genDebug "github.com/AndroidGoLab/binder/android/debug"
	genDeviceState "github.com/AndroidGoLab/binder/android/hardware/devicestate"
	genDisplay "github.com/AndroidGoLab/binder/android/hardware/display"
	genDomain "github.com/AndroidGoLab/binder/android/content/pm/verify/domain"
	genDreams "github.com/AndroidGoLab/binder/android/service/dreams"
	genFace "github.com/AndroidGoLab/binder/android/hardware/face"
	genFingerprint "github.com/AndroidGoLab/binder/android/hardware/fingerprint"
	genFlags "github.com/AndroidGoLab/binder/android/flags"
	genGui "github.com/AndroidGoLab/binder/android/gui"
	genImage "github.com/AndroidGoLab/binder/android/os/image"
	genIncremental "github.com/AndroidGoLab/binder/android/os/incremental"
	genInput "github.com/AndroidGoLab/binder/android/hardware/input"
	genIntegrity "github.com/AndroidGoLab/binder/android/content/integrity"
	genLights "github.com/AndroidGoLab/binder/android/hardware/lights"
	genLocation "github.com/AndroidGoLab/binder/android/location"
	genLogcat "github.com/AndroidGoLab/binder/android/os/logcat"
	genMedia "github.com/AndroidGoLab/binder/android/media"
	genMediaMetrics "github.com/AndroidGoLab/binder/android/media/metrics"
	genMemtrack "github.com/AndroidGoLab/binder/android/hardware/memtrack"
	genMidi "github.com/AndroidGoLab/binder/android/media/midi"
	genMusicRecognition "github.com/AndroidGoLab/binder/android/media/musicrecognition"
	genNet "github.com/AndroidGoLab/binder/android/net"
	genOnDeviceIntelligence "github.com/AndroidGoLab/binder/android/app/ondeviceintelligence"
	genOs "github.com/AndroidGoLab/binder/android/os"
	genOsStorage "github.com/AndroidGoLab/binder/android/os/storage"
	genPeople "github.com/AndroidGoLab/binder/android/app/people"
	genPermission "github.com/AndroidGoLab/binder/android/permission"
	genPinner "github.com/AndroidGoLab/binder/android/app/pinner"
	genPm "github.com/AndroidGoLab/binder/android/content/pm"
	genPrediction "github.com/AndroidGoLab/binder/android/app/prediction"
	genPrint "github.com/AndroidGoLab/binder/android/print"
	genProjection "github.com/AndroidGoLab/binder/android/media/projection"
	genRollback "github.com/AndroidGoLab/binder/android/content/rollback"
	genSe "github.com/AndroidGoLab/binder/android/se/omapi"
	genSearchUI "github.com/AndroidGoLab/binder/android/app/search"
	genSecurity "github.com/AndroidGoLab/binder/android/security"
	genSession "github.com/AndroidGoLab/binder/android/media/session"
	genSlice "github.com/AndroidGoLab/binder/android/app/slice"
	genSmartspace "github.com/AndroidGoLab/binder/android/app/smartspace"
	genSpeechTts "github.com/AndroidGoLab/binder/android/speech/tts"
	genSvcTextClassifier "github.com/AndroidGoLab/binder/android/service/textclassifier"
	genTimeDetector "github.com/AndroidGoLab/binder/android/app/timedetector"
	genTimeZoneDetector "github.com/AndroidGoLab/binder/android/app/timezonedetector"
	genTracing "github.com/AndroidGoLab/binder/android/tracing"
	genTranslation "github.com/AndroidGoLab/binder/android/view/translation"
	genTrust "github.com/AndroidGoLab/binder/android/app/trust"
	genUsage "github.com/AndroidGoLab/binder/android/app/usage"
	genVcn "github.com/AndroidGoLab/binder/android/net/vcn"
	genView "github.com/AndroidGoLab/binder/android/view"
	genVirtual "github.com/AndroidGoLab/binder/android/companion/virtual"
	genVirtualNative "github.com/AndroidGoLab/binder/android/companion/virtualnative"
	genWallpaperEffects "github.com/AndroidGoLab/binder/android/app/wallpapereffectsgeneration"
	genWearable "github.com/AndroidGoLab/binder/android/app/wearable"
	genWebkit "github.com/AndroidGoLab/binder/android/webkit"

	// com.android.internal proxies
	genInternalApp "github.com/AndroidGoLab/binder/com/android/internal_/app"
	genInternalFonts "github.com/AndroidGoLab/binder/com/android/internal_/graphics/fonts"
	genInternalTelecom "github.com/AndroidGoLab/binder/com/android/internal_/telecom"

	// HAL proxies
	genAuthSecret "github.com/AndroidGoLab/binder/android/hardware/authsecret"
	genHwBiometricsFace "github.com/AndroidGoLab/binder/android/hardware/biometrics/face"
	genHwBiometricsFingerprint "github.com/AndroidGoLab/binder/android/hardware/biometrics/fingerprint"
	genHwBt "github.com/AndroidGoLab/binder/android/hardware/bluetooth"
	genHwBtAudio "github.com/AndroidGoLab/binder/android/hardware/bluetooth/audio"
	genCameraProvider "github.com/AndroidGoLab/binder/android/hardware/camera/provider"
	genCas "github.com/AndroidGoLab/binder/android/hardware/cas"
	genDrm "github.com/AndroidGoLab/binder/android/hardware/drm"
	genGnss "github.com/AndroidGoLab/binder/android/hardware/gnss"
	genHealth "github.com/AndroidGoLab/binder/android/hardware/health"
	genHwLight "github.com/AndroidGoLab/binder/android/hardware/light"
	genHwPower "github.com/AndroidGoLab/binder/android/hardware/power"
	genHwPowerStats "github.com/AndroidGoLab/binder/android/hardware/power/stats"
	genHwUsb "github.com/AndroidGoLab/binder/android/hardware/usb"
	genHwVibrator "github.com/AndroidGoLab/binder/android/hardware/vibrator"
	genIdentity "github.com/AndroidGoLab/binder/android/hardware/identity"
	genKeyMint "github.com/AndroidGoLab/binder/android/hardware/security/keymint"
	genKeystore2 "github.com/AndroidGoLab/binder/android/system/keystore2"
	genNN "github.com/AndroidGoLab/binder/android/hardware/neuralnetworks"
	genNetd "github.com/AndroidGoLab/binder/android/system/net/netd"
	genRadioConfig "github.com/AndroidGoLab/binder/android/hardware/radio/config"
	genRadioData "github.com/AndroidGoLab/binder/android/hardware/radio/data"
	genRadioIms "github.com/AndroidGoLab/binder/android/hardware/radio/ims"
	genRadioImsMedia "github.com/AndroidGoLab/binder/android/hardware/radio/ims/media"
	genRadioMessaging "github.com/AndroidGoLab/binder/android/hardware/radio/messaging"
	genRadioModem "github.com/AndroidGoLab/binder/android/hardware/radio/modem"
	genRadioNetwork "github.com/AndroidGoLab/binder/android/hardware/radio/network"
	genRadioSim "github.com/AndroidGoLab/binder/android/hardware/radio/sim"
	genRadioVoice "github.com/AndroidGoLab/binder/android/hardware/radio/voice"
	genRebootEscrow "github.com/AndroidGoLab/binder/android/hardware/rebootescrow"
	genSecureClock "github.com/AndroidGoLab/binder/android/hardware/security/secureclock"
	genSensors "github.com/AndroidGoLab/binder/android/hardware/sensors"
	genSharedSecret "github.com/AndroidGoLab/binder/android/hardware/security/sharedsecret"
	genSupplicant "github.com/AndroidGoLab/binder/android/hardware/wifi/supplicant"
	genSuspend "github.com/AndroidGoLab/binder/android/system/suspend"
	genWifi "github.com/AndroidGoLab/binder/android/hardware/wifi"
)

type serviceEntry struct {
	name        string
	constructor func(binder.IBinder) interface{}
}

// safeServicePrefixes lists service name prefixes that are safe to smoke-test.
// Only framework services with permission checks are included. HAL services
// (android.hardware.*), system services (android.system.*), and specific
// dangerous services (installd, SurfaceFlinger) are excluded.
//
// This allowlist exists because calling arbitrary methods with zero-value
// arguments on unprotected services bricked a phone: IKeyMintDevice.DeleteAllKeys()
// destroyed hardware crypto keys, IKeyMintDevice.DestroyAttestationIds() permanently
// destroyed attestation identity, and installd.DestroyUserData() wiped user 0 data.
var safeServicePrefixes = []string{
	"accessibility",
	"account",
	"activity",
	"alarm",
	"app",
	"audio",
	"autofill",
	"battery",
	"bluetooth_manager",
	"clipboard",
	"connectivity",
	"content",
	"country_detector",
	"device_policy",
	"display",
	"dreams",
	"dropbox",
	"ethernet",
	"font",
	"gpu",
	"input",
	"input_method",
	"jobscheduler",
	"launcherapps",
	"location",
	"media",
	"midi",
	"mount",
	"netpolicy",
	"netstats",
	"network_management",
	"network_score",
	"notification",
	"overlay",
	"package",
	"permission",
	"phone",
	"power",
	"print",
	"search",
	"sensor_privacy",
	"serial",
	"settings",
	"shortcut",
	"statusbar",
	"telecom",
	"telephony",
	"trust",
	"uimode",
	"uri_grants",
	"usagestats",
	"user",
	"vibrator_manager",
	"voiceinteraction",
	"wallpaper",
	"webviewupdate",
	"wifi",
	"window",
}

// dangerousMethodSubstrings lists method name substrings that are never safe
// to call with zero-value arguments, even on allowlisted services. These
// methods can destroy data, reset hardware state, power-cycle the device,
// cause the system to kill our process, or block indefinitely.
var dangerousMethodSubstrings = []string{
	// --- Generic destructive verbs ---
	"Delete",
	"Destroy",
	"Wipe",
	"Erase",
	"Reset",
	"Shutdown",
	"Reboot",
	"Format",
	"Kill",
	"Crash",     // PowerManager.Crash, CrashApplication*, HandleApplicationCrash
	"ForceStop", // ActivityManager.ForceStopPackage, ForceStopPackageEvenWhenStopping

	// --- Methods that report our process as broken (system kills us) ---
	"AttachApplication",  // ActivityManager: registers as system app, killed on mismatch
	"AppNotResponding",   // ActivityManager: reports ANR → system kills the process
	"HandleApplicationWtf",
	"HandleApplicationStrictMode",

	// --- Methods that block indefinitely or hang the test ---
	"Hang",
	"HoldLock",  // ActivityManager, PackageManager, WindowManager
	"WaitFor",   // WaitForBroadcastIdle, WaitForHandler, WaitForNetworkStateUpdate, etc.

	// --- Methods that disrupt the device or our process ---
	"EnterSafeMode",  // ActivityManager, PackageManager: triggers safe mode reboot
	"ForceSuspend",   // PowerManager: suspends the device
	"GoToSleep",      // PowerManager: puts device to sleep
	"Restart",        // ActivityManager.Restart (restarts AM), RestartUserInBackground
	"StopAppForUser", // ActivityManager: force-stops an app for a user
}

// isServiceSafe returns true if the service name matches any safe prefix
// and does not match any known-dangerous service pattern.
func isServiceSafe(name string) bool {
	// HAL services talk directly to hardware with no permission checks.
	if strings.Contains(name, "android.hardware.") {
		return false
	}

	// System services (vold, netd, etc.) are low-level and unprotected.
	if strings.Contains(name, "android.system.") {
		return false
	}

	// Specific dangerous services.
	switch name {
	case "installd", "SurfaceFlinger", "SurfaceFlingerAIDL":
		return false
	}

	for _, prefix := range safeServicePrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}

	return false
}

// isMethodSafe returns true if the method name does not contain any
// dangerous substring.
func isMethodSafe(methodName string) bool {
	for _, substr := range dangerousMethodSubstrings {
		if strings.Contains(methodName, substr) {
			return false
		}
	}
	return true
}

// serviceRegistry maps Android service names to typed proxy constructors.
// Framework services use short names (e.g. "power"), HAL services use
// VINTF-style names (e.g. "android.hardware.health.IHealth/default").
//
// Only services with confirmed proxy constructors in gen/ are included.
// Services without a generated interface proxy (e.g. overlay, jobscheduler,
// permissionmgr, app_hibernation) are omitted.
var serviceRegistry = []serviceEntry{
	// --- Framework services ---
	{"accessibility", func(b binder.IBinder) interface{} { return genAccessibility.NewAccessibilityManagerProxy(b) }},
	{"account", func(b binder.IBinder) interface{} { return genAccounts.NewAccountManagerProxy(b) }},
	{"activity", func(b binder.IBinder) interface{} { return genApp.NewActivityManagerProxy(b) }},
	{"activity_task", func(b binder.IBinder) interface{} { return genApp.NewActivityTaskManagerProxy(b) }},
	{"adb", func(b binder.IBinder) interface{} { return genDebug.NewAdbManagerProxy(b) }},
	{"alarm", func(b binder.IBinder) interface{} { return genApp.NewAlarmManagerProxy(b) }},
	{"ambient_context", func(b binder.IBinder) interface{} { return genAmbientContext.NewAmbientContextManagerProxy(b) }},
	{"app_integrity", func(b binder.IBinder) interface{} { return genIntegrity.NewAppIntegrityManagerProxy(b) }},
	{"app_prediction", func(b binder.IBinder) interface{} { return genPrediction.NewPredictionManagerProxy(b) }},
	{"appops", func(b binder.IBinder) interface{} { return genInternalApp.NewAppOpsServiceProxy(b) }},
	{"auth", func(b binder.IBinder) interface{} { return genBiometrics.NewAuthServiceProxy(b) }},
	{"autofill", func(b binder.IBinder) interface{} { return genAutofill.NewAutoFillManagerProxy(b) }},
	{"background_install_control", func(b binder.IBinder) interface{} { return genPm.NewBackgroundInstallControlServiceProxy(b) }},
	{"backup", func(b binder.IBinder) interface{} { return genBackup.NewBackupManagerProxy(b) }},
	{"batteryproperties", func(b binder.IBinder) interface{} { return genOs.NewBatteryPropertiesRegistrarProxy(b) }},
	{"biometric", func(b binder.IBinder) interface{} { return genBiometrics.NewBiometricServiceProxy(b) }},
	{"blob_store", func(b binder.IBinder) interface{} { return genBlob.NewBlobStoreManagerProxy(b) }},
	{"bugreport", func(b binder.IBinder) interface{} { return genOs.NewDumpstateProxy(b) }},
	{"clipboard", func(b binder.IBinder) interface{} { return genContent.NewClipboardProxy(b) }},
	{"color_display", func(b binder.IBinder) interface{} { return genDisplay.NewColorDisplayManagerProxy(b) }},
	{"companiondevice", func(b binder.IBinder) interface{} { return genCompanion.NewCompanionDeviceManagerProxy(b) }},
	{"connmetrics", func(b binder.IBinder) interface{} { return genNet.NewIpConnectivityMetricsProxy(b) }},
	{"content", func(b binder.IBinder) interface{} { return genContent.NewContentServiceProxy(b) }},
	{"content_capture", func(b binder.IBinder) interface{} { return genContentCapture.NewContentCaptureManagerProxy(b) }},
	{"content_suggestions", func(b binder.IBinder) interface{} { return genContentSuggestions.NewContentSuggestionsManagerProxy(b) }},
	{"country_detector", func(b binder.IBinder) interface{} { return genLocation.NewCountryDetectorProxy(b) }},
	{"credential", func(b binder.IBinder) interface{} { return genCredentials.NewCredentialManagerProxy(b) }},
	{"crossprofileapps", func(b binder.IBinder) interface{} { return genPm.NewCrossProfileAppsProxy(b) }},
	{"dataloader_manager", func(b binder.IBinder) interface{} { return genPm.NewDataLoaderManagerProxy(b) }},
	{"device_identifiers", func(b binder.IBinder) interface{} { return genOs.NewDeviceIdentifiersPolicyServiceProxy(b) }},
	{"device_policy", func(b binder.IBinder) interface{} { return genAdmin.NewDevicePolicyManagerProxy(b) }},
	{"device_state", func(b binder.IBinder) interface{} { return genDeviceState.NewDeviceStateManagerProxy(b) }},
	{"deviceidle", func(b binder.IBinder) interface{} { return genOs.NewDeviceIdleControllerProxy(b) }},
	{"display", func(b binder.IBinder) interface{} { return genDisplay.NewDisplayManagerProxy(b) }},
	{"domain_verification", func(b binder.IBinder) interface{} { return genDomain.NewDomainVerificationManagerProxy(b) }},
	{"dreams", func(b binder.IBinder) interface{} { return genDreams.NewDreamManagerProxy(b) }},
	{"dynamic_system", func(b binder.IBinder) interface{} { return genImage.NewDynamicSystemServiceProxy(b) }},
	{"external_vibrator_service", func(b binder.IBinder) interface{} { return genOs.NewExternalVibratorServiceProxy(b) }},
	{"face", func(b binder.IBinder) interface{} { return genFace.NewFaceServiceProxy(b) }},
	{"feature_flags", func(b binder.IBinder) interface{} { return genFlags.NewFeatureFlagsProxy(b) }},
	{"file_integrity", func(b binder.IBinder) interface{} { return genSecurity.NewFileIntegrityServiceProxy(b) }},
	{"fingerprint", func(b binder.IBinder) interface{} { return genFingerprint.NewFingerprintServiceProxy(b) }},
	{"font", func(b binder.IBinder) interface{} { return genInternalFonts.NewFontManagerProxy(b) }},
	{"game", func(b binder.IBinder) interface{} { return genApp.NewGameManagerServiceProxy(b) }},
	{"grammatical_inflection", func(b binder.IBinder) interface{} { return genApp.NewGrammaticalInflectionManagerProxy(b) }},
	{"graphicsstats", func(b binder.IBinder) interface{} { return genView.NewGraphicsStatsProxy(b) }},
	{"hardware_properties", func(b binder.IBinder) interface{} { return genOs.NewHardwarePropertiesManagerProxy(b) }},
	{"incident", func(b binder.IBinder) interface{} { return genOs.NewIncidentManagerProxy(b) }},
	{"incidentcompanion", func(b binder.IBinder) interface{} { return genOs.NewIncidentCompanionProxy(b) }},
	{"incremental", func(b binder.IBinder) interface{} { return genIncremental.NewIncrementalServiceProxy(b) }},
	{"input", func(b binder.IBinder) interface{} { return genInput.NewInputManagerProxy(b) }},
	{"inputflinger", func(b binder.IBinder) interface{} { return genOs.NewInputFlingerProxy(b) }},
	{"installd", func(b binder.IBinder) interface{} { return genOs.NewInstalldProxy(b) }},
	{"legacy_permission", func(b binder.IBinder) interface{} { return genPermission.NewLegacyPermissionManagerProxy(b) }},
	{"lights", func(b binder.IBinder) interface{} { return genLights.NewLightsManagerProxy(b) }},
	{"locale", func(b binder.IBinder) interface{} { return genApp.NewLocaleManagerProxy(b) }},
	{"location", func(b binder.IBinder) interface{} { return genLocation.NewLocationManagerProxy(b) }},
	{"logcat", func(b binder.IBinder) interface{} { return genLogcat.NewLogcatManagerServiceProxy(b) }},
	{"manager", func(b binder.IBinder) interface{} { return genOs.NewServiceManagerProxy(b) }},
	{"media_metrics", func(b binder.IBinder) interface{} { return genMediaMetrics.NewMediaMetricsManagerProxy(b) }},
	{"media_projection", func(b binder.IBinder) interface{} { return genProjection.NewMediaProjectionManagerProxy(b) }},
	{"media_resource_monitor", func(b binder.IBinder) interface{} { return genMedia.NewMediaResourceMonitorProxy(b) }},
	{"media_router", func(b binder.IBinder) interface{} { return genMedia.NewMediaRouterServiceProxy(b) }},
	{"media_session", func(b binder.IBinder) interface{} { return genSession.NewSessionManagerProxy(b) }},
	{"memtrack.proxy", func(b binder.IBinder) interface{} { return genMemtrack.NewMemtrackProxy(b) }},
	{"midi", func(b binder.IBinder) interface{} { return genMidi.NewMidiManagerProxy(b) }},
	{"mount", func(b binder.IBinder) interface{} { return genOsStorage.NewStorageManagerProxy(b) }},
	{"music_recognition", func(b binder.IBinder) interface{} { return genMusicRecognition.NewMusicRecognitionManagerProxy(b) }},
	{"netpolicy", func(b binder.IBinder) interface{} { return genNet.NewNetworkPolicyManagerProxy(b) }},
	{"network_management", func(b binder.IBinder) interface{} { return genOs.NewNetworkManagementServiceProxy(b) }},
	{"network_score", func(b binder.IBinder) interface{} { return genNet.NewNetworkScoreServiceProxy(b) }},
	{"on_device_intelligence", func(b binder.IBinder) interface{} { return genOnDeviceIntelligence.NewOnDeviceIntelligenceManagerProxy(b) }},
	{"package", func(b binder.IBinder) interface{} { return genPm.NewPackageManagerProxy(b) }},
	{"package_native", func(b binder.IBinder) interface{} { return genPm.NewPackageManagerNativeProxy(b) }},
	{"people", func(b binder.IBinder) interface{} { return genPeople.NewPeopleManagerProxy(b) }},
	{"performance_hint", func(b binder.IBinder) interface{} { return genOs.NewHintManagerProxy(b) }},
	{"pinner", func(b binder.IBinder) interface{} { return genPinner.NewPinnerServiceProxy(b) }},
	{"power", func(b binder.IBinder) interface{} { return genOs.NewPowerManagerProxy(b) }},
	{"powerstats", func(b binder.IBinder) interface{} { return genOs.NewPowerStatsServiceProxy(b) }},
	{"print", func(b binder.IBinder) interface{} { return genPrint.NewPrintManagerProxy(b) }},
	{"processinfo", func(b binder.IBinder) interface{} { return genOs.NewProcessInfoServiceProxy(b) }},
	{"recovery", func(b binder.IBinder) interface{} { return genOs.NewRecoverySystemProxy(b) }},
	{"rollback", func(b binder.IBinder) interface{} { return genRollback.NewRollbackManagerProxy(b) }},
	{"scheduling_policy", func(b binder.IBinder) interface{} { return genOs.NewSchedulingPolicyServiceProxy(b) }},
	{"search", func(b binder.IBinder) interface{} { return genApp.NewSearchManagerProxy(b) }},
	{"search_ui", func(b binder.IBinder) interface{} { return genSearchUI.NewSearchUiManagerProxy(b) }},
	{"secure_element", func(b binder.IBinder) interface{} { return genSe.NewSecureElementServiceProxy(b) }},
	{"security_state", func(b binder.IBinder) interface{} { return genOs.NewSecurityStateManagerProxy(b) }},
	{"slice", func(b binder.IBinder) interface{} { return genSlice.NewSliceManagerProxy(b) }},
	{"smartspace", func(b binder.IBinder) interface{} { return genSmartspace.NewSmartspaceManagerProxy(b) }},
	{"storagestats", func(b binder.IBinder) interface{} { return genUsage.NewStorageStatsManagerProxy(b) }},
	{"SurfaceFlingerAIDL", func(b binder.IBinder) interface{} { return genGui.NewSurfaceComposerProxy(b) }},
	{"system_config", func(b binder.IBinder) interface{} { return genOs.NewSystemConfigProxy(b) }},
	{"system_update", func(b binder.IBinder) interface{} { return genOs.NewSystemUpdateManagerProxy(b) }},
	{"telecom", func(b binder.IBinder) interface{} { return genInternalTelecom.NewTelecomServiceProxy(b) }},
	{"textclassification", func(b binder.IBinder) interface{} { return genSvcTextClassifier.NewTextClassifierServiceProxy(b) }},
	{"texttospeech", func(b binder.IBinder) interface{} { return genSpeechTts.NewTextToSpeechManagerProxy(b) }},
	{"thermalservice", func(b binder.IBinder) interface{} { return genOs.NewThermalServiceProxy(b) }},
	{"time_detector", func(b binder.IBinder) interface{} { return genTimeDetector.NewTimeDetectorServiceProxy(b) }},
	{"time_zone_detector", func(b binder.IBinder) interface{} { return genTimeZoneDetector.NewTimeZoneDetectorServiceProxy(b) }},
	{"tracing.proxy", func(b binder.IBinder) interface{} { return genTracing.NewTracingServiceProxyProxy(b) }},
	{"translation", func(b binder.IBinder) interface{} { return genTranslation.NewTranslationManagerProxy(b) }},
	{"trust", func(b binder.IBinder) interface{} { return genTrust.NewTrustManagerProxy(b) }},
	{"uimode", func(b binder.IBinder) interface{} { return genApp.NewUiModeManagerProxy(b) }},
	{"usagestats", func(b binder.IBinder) interface{} { return genUsage.NewUsageStatsManagerProxy(b) }},
	{"usb", func(b binder.IBinder) interface{} { return genHwUsb.NewUsbManagerProxy(b) }},
	{"user", func(b binder.IBinder) interface{} { return genOs.NewUserManagerProxy(b) }},
	{"vcn_management", func(b binder.IBinder) interface{} { return genVcn.NewVcnManagementServiceProxy(b) }},
	{"vibrator_manager", func(b binder.IBinder) interface{} { return genOs.NewVibratorManagerServiceProxy(b) }},
	{"virtualdevice", func(b binder.IBinder) interface{} { return genVirtual.NewVirtualDeviceManagerProxy(b) }},
	{"virtualdevice_native", func(b binder.IBinder) interface{} { return genVirtualNative.NewVirtualDeviceManagerNativeProxy(b) }},
	{"wallpaper", func(b binder.IBinder) interface{} { return genApp.NewWallpaperManagerProxy(b) }},
	{"wallpaper_effects_generation", func(b binder.IBinder) interface{} { return genWallpaperEffects.NewWallpaperEffectsGenerationManagerProxy(b) }},
	{"wearable_sensing", func(b binder.IBinder) interface{} { return genWearable.NewWearableSensingManagerProxy(b) }},
	{"webviewupdate", func(b binder.IBinder) interface{} { return genWebkit.NewWebViewUpdateServiceProxy(b) }},
	{"window", func(b binder.IBinder) interface{} { return genView.NewWindowManagerProxy(b) }},

	// --- HAL services (VINTF naming) ---
	{"android.hardware.authsecret.IAuthSecret/default", func(b binder.IBinder) interface{} { return genAuthSecret.NewAuthSecretProxy(b) }},
	{"android.hardware.biometrics.face.IFace/virtual", func(b binder.IBinder) interface{} { return genHwBiometricsFace.NewFaceProxy(b) }},
	{"android.hardware.biometrics.fingerprint.IFingerprint/default", func(b binder.IBinder) interface{} { return genHwBiometricsFingerprint.NewFingerprintProxy(b) }},
	{"android.hardware.bluetooth.IBluetoothHci/default", func(b binder.IBinder) interface{} { return genHwBt.NewBluetoothHciProxy(b) }},
	{"android.hardware.bluetooth.audio.IBluetoothAudioProviderFactory/default", func(b binder.IBinder) interface{} { return genHwBtAudio.NewBluetoothAudioProviderFactoryProxy(b) }},
	{"android.hardware.camera.provider.ICameraProvider/internal/0", func(b binder.IBinder) interface{} { return genCameraProvider.NewCameraProviderProxy(b) }},
	{"android.hardware.cas.IMediaCasService/default", func(b binder.IBinder) interface{} { return genCas.NewMediaCasServiceProxy(b) }},
	{"android.hardware.drm.IDrmFactory/widevine", func(b binder.IBinder) interface{} { return genDrm.NewDrmFactoryProxy(b) }},
	{"android.hardware.gnss.IGnss/default", func(b binder.IBinder) interface{} { return genGnss.NewGnssProxy(b) }},
	{"android.hardware.health.IHealth/default", func(b binder.IBinder) interface{} { return genHealth.NewHealthProxy(b) }},
	{"android.hardware.identity.IIdentityCredentialStore/default", func(b binder.IBinder) interface{} { return genIdentity.NewIdentityCredentialStoreProxy(b) }},
	{"android.hardware.light.ILights/default", func(b binder.IBinder) interface{} { return genHwLight.NewLightsProxy(b) }},
	{"android.hardware.neuralnetworks.IDevice/nnapi-sample_all", func(b binder.IBinder) interface{} { return genNN.NewDeviceProxy(b) }},
	{"android.hardware.power.IPower/default", func(b binder.IBinder) interface{} { return genHwPower.NewPowerProxy(b) }},
	{"android.hardware.power.stats.IPowerStats/default", func(b binder.IBinder) interface{} { return genHwPowerStats.NewPowerStatsProxy(b) }},
	{"android.hardware.radio.config.IRadioConfig/default", func(b binder.IBinder) interface{} { return genRadioConfig.NewRadioConfigProxy(b) }},
	{"android.hardware.radio.data.IRadioData/slot1", func(b binder.IBinder) interface{} { return genRadioData.NewRadioDataProxy(b) }},
	{"android.hardware.radio.ims.IRadioIms/slot1", func(b binder.IBinder) interface{} { return genRadioIms.NewRadioImsProxy(b) }},
	{"android.hardware.radio.ims.media.IImsMedia/default", func(b binder.IBinder) interface{} { return genRadioImsMedia.NewImsMediaProxy(b) }},
	{"android.hardware.radio.messaging.IRadioMessaging/slot1", func(b binder.IBinder) interface{} { return genRadioMessaging.NewRadioMessagingProxy(b) }},
	{"android.hardware.radio.modem.IRadioModem/slot1", func(b binder.IBinder) interface{} { return genRadioModem.NewRadioModemProxy(b) }},
	{"android.hardware.radio.network.IRadioNetwork/slot1", func(b binder.IBinder) interface{} { return genRadioNetwork.NewRadioNetworkProxy(b) }},
	{"android.hardware.radio.sim.IRadioSim/slot1", func(b binder.IBinder) interface{} { return genRadioSim.NewRadioSimProxy(b) }},
	{"android.hardware.radio.voice.IRadioVoice/slot1", func(b binder.IBinder) interface{} { return genRadioVoice.NewRadioVoiceProxy(b) }},
	{"android.hardware.rebootescrow.IRebootEscrow/default", func(b binder.IBinder) interface{} { return genRebootEscrow.NewRebootEscrowProxy(b) }},
	{"android.hardware.security.keymint.IKeyMintDevice/default", func(b binder.IBinder) interface{} { return genKeyMint.NewKeyMintDeviceProxy(b) }},
	{"android.hardware.security.keymint.IRemotelyProvisionedComponent/default", func(b binder.IBinder) interface{} { return genKeyMint.NewRemotelyProvisionedComponentProxy(b) }},
	{"android.hardware.security.secureclock.ISecureClock/default", func(b binder.IBinder) interface{} { return genSecureClock.NewSecureClockProxy(b) }},
	{"android.hardware.security.sharedsecret.ISharedSecret/default", func(b binder.IBinder) interface{} { return genSharedSecret.NewSharedSecretProxy(b) }},
	{"android.hardware.sensors.ISensors/default", func(b binder.IBinder) interface{} { return genSensors.NewSensorsProxy(b) }},
	{"android.hardware.usb.IUsb/default", func(b binder.IBinder) interface{} { return genHwUsb.NewUsbProxy(b) }},
	{"android.hardware.vibrator.IVibrator/default", func(b binder.IBinder) interface{} { return genHwVibrator.NewVibratorProxy(b) }},
	{"android.hardware.vibrator.IVibratorManager/default", func(b binder.IBinder) interface{} { return genHwVibrator.NewVibratorManagerProxy(b) }},
	{"android.hardware.wifi.IWifi/default", func(b binder.IBinder) interface{} { return genWifi.NewWifiProxy(b) }},
	{"android.hardware.wifi.supplicant.ISupplicant/default", func(b binder.IBinder) interface{} { return genSupplicant.NewSupplicantProxy(b) }},
	{"android.system.keystore2.IKeystoreService/default", func(b binder.IBinder) interface{} { return genKeystore2.NewKeystoreServiceProxy(b) }},
	{"android.system.net.netd.INetd/default", func(b binder.IBinder) interface{} { return genNetd.NewNetdProxy(b) }},
	{"android.system.suspend.ISystemSuspend/default", func(b binder.IBinder) interface{} { return genSuspend.NewSystemSuspendProxy(b) }},
}

// TestE2E_SmokeAllServiceMethods calls every exported method on each
// service's typed proxy with zero-value arguments. The test uses a single
// shared binder driver to avoid resource exhaustion. Services that are
// unavailable (SELinux denial, not registered) are skipped.
func TestE2E_SmokeAllServiceMethods(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)

	var totalServices, testedServices int
	var totalMethods, passedMethods, panickedMethods, failedMethods int

	for _, entry := range serviceRegistry {
		totalServices++
		entry := entry

		if !isServiceSafe(entry.name) {
			t.Run(entry.name, func(t *testing.T) {
				t.Skipf("service %s not in safe allowlist (see bricking incident)", entry.name)
			})
			continue
		}

		t.Run(entry.name, func(t *testing.T) {
			svc, err := sm.GetService(ctx, servicemanager.ServiceName(entry.name))
			if err != nil {
				t.Skipf("GetService(%s): %v", entry.name, err)
				return
			}
			if svc == nil {
				t.Skipf("service %s not available", entry.name)
				return
			}
			testedServices++

			proxy := entry.constructor(svc)
			result := testutil.SmokeTestAllMethods(t, proxy, testutil.WithMethodFilter(isMethodSafe))

			totalMethods += result.Total
			passedMethods += result.Passed
			panickedMethods += result.Panicked
			failedMethods += result.Failed

			t.Logf("%s: %d/%d passed, %d panicked, %d failed",
				entry.name, result.Passed, result.Total, result.Panicked, result.Failed)
		})
	}

	t.Logf("=== Summary ===")
	t.Logf("Services: %d registered, %d tested", totalServices, testedServices)
	t.Logf("Methods: %d total, %d passed, %d panicked, %d failed",
		totalMethods, passedMethods, panickedMethods, failedMethods)
	assert.Equal(t, 0, panickedMethods, "proxy methods should not panic with zero-value arguments")
}
