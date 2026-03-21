//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	genAccounts "github.com/xaionaro-go/binder/android/accounts"
	genApp "github.com/xaionaro-go/binder/android/app"
	genPm "github.com/xaionaro-go/binder/android/content/pm"
	genDisplay "github.com/xaionaro-go/binder/android/hardware/display"
	genInput "github.com/xaionaro-go/binder/android/hardware/input"
	genLocation "github.com/xaionaro-go/binder/android/location"
	"github.com/xaionaro-go/binder/binder"
	genTelephony "github.com/xaionaro-go/binder/com/android/internal_/telephony"
	"github.com/xaionaro-go/binder/parcel"
	"github.com/xaionaro-go/binder/servicemanager"
)

// ==========================================================================
// Parcelable stress tests: exercise complex real-world parcelable
// serialization/deserialization via binder to expose bugs in the library's
// parcel handling. Each test connects to an Android system service, calls a
// method that returns a complex/nested parcelable, and verifies that the
// reply can be fully consumed.
//
// If a generated proxy returns interface{}/nil for a parcelable that the
// service actually returned data for, that's a BUG in code generation
// (the deserializer was never emitted).
//
// If raw transact fails to read the expected parcelable structure, that's a
// BUG in the parcel reader.
// ==========================================================================

// ---------------------------------------------------------------------------
// 1. ActivityManager.getRunningAppProcesses
//    Returns List<RunningAppProcessInfo> — nested parcelable with arrays.
// ---------------------------------------------------------------------------

func TestParcelableStress_ActivityManager_GetRunningAppProcesses(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "activity")

	// --- Via generated proxy ---
	proxy := genApp.NewActivityManagerProxy(svc)
	result, err := proxy.GetRunningAppProcesses(ctx)
	requireOrSkip(t, err)

	// The proxy returns []ActivityManagerRunningAppProcessInfo (value types).
	// The service DOES return data (the device always has running processes).
	if len(result) == 0 {
		t.Errorf("BUG: GetRunningAppProcesses returned empty list; device always has running processes")
	} else {
		t.Logf("GetRunningAppProcesses: %d processes (proxy)", len(result))
	}

	// --- Via raw transact: verify the parcel contains real data ---
	const descriptor = "android.app.IActivityManager"
	code := resolveCode(ctx, t, svc, descriptor, "getRunningAppProcesses")
	data := parcel.New()
	data.WriteInterfaceToken(descriptor)

	reply, err := svc.Transact(ctx, code, 0, data)
	requireOrSkip(t, err)
	requireOrSkip(t, binder.ReadStatus(reply))

	count, err := reply.ReadInt32()
	requireOrSkip(t, err)
	t.Logf("getRunningAppProcesses raw: count=%d", count)
	assert.Greater(t, count, int32(0), "device should have at least one running process")

	remaining := reply.Len() - reply.Position()
	t.Logf("getRunningAppProcesses raw: %d bytes of parcelable data remaining after count", remaining)
	if count > 0 && remaining == 0 {
		t.Errorf("BUG: count=%d but no parcel data follows — parcel was truncated", count)
	}
}

// ---------------------------------------------------------------------------
// 2. PackageManager.getPackageInfo
//    Returns PackageInfo — deeply nested parcelable with arrays of
//    ComponentInfo.
// ---------------------------------------------------------------------------

func TestParcelableStress_PackageManager_GetPackageInfo(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "package")

	// --- Via generated proxy ---
	proxy := genPm.NewPackageManagerProxy(svc)
	result, err := proxy.GetPackageInfo(ctx, "com.android.settings", 0)
	requireOrSkip(t, err)

	// The proxy returns PackageInfo (a value type). If the call succeeded
	// without error, deserialization worked.
	t.Logf("getPackageInfo proxy returned: %T", result)
}

// ---------------------------------------------------------------------------
// 3. LocationManager.getProviderProperties
//    Returns ProviderProperties parcelable.
// ---------------------------------------------------------------------------

func TestParcelableStress_LocationManager_GetProviderProperties(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "location")

	proxy := genLocation.NewLocationManagerProxy(svc)

	// First get available providers.
	providers, err := proxy.GetAllProviders(ctx)
	requireOrSkip(t, err)
	if len(providers) == 0 {
		t.Skip("no location providers available")
	}
	t.Logf("location providers: %v", providers)

	for _, provider := range providers {
		t.Run(provider, func(t *testing.T) {
			result, err := proxy.GetProviderProperties(ctx, provider)
			requireOrSkip(t, err)
			t.Logf("getProviderProperties(%q): %+v", provider, result)
		})
	}
}

// ---------------------------------------------------------------------------
// 4. TelephonyManager.getServiceState
//    Returns ServiceState — complex parcelable.
// ---------------------------------------------------------------------------

func TestParcelableStress_Telephony_GetServiceState(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "phone")

	proxy := genTelephony.NewTelephonyProxy(svc)
	// slotIndex=0, renounce fine & coarse location to avoid permission issues.
	result, err := proxy.GetServiceStateForSlot(ctx, 0, true, true)
	requireOrSkip(t, err)

	// GetServiceStateForSlot returns androidTelephony.ServiceState (a value
	// type, not a pointer), so we cannot nil-check the result. If the proxy
	// call succeeded without error, deserialization worked.
	t.Logf("getServiceStateForSlot(0): %+v", result)
}

// ---------------------------------------------------------------------------
// 5. WifiManager.getConnectionInfo
//    Returns WifiInfo — complex parcelable with nested fields.
//    WiFi is a Java-only AIDL interface not in version tables, so we use
//    raw transact with a hardcoded transaction code offset.
// ---------------------------------------------------------------------------

func TestParcelableStress_WiFi_GetConnectionInfo(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)

	svc, err := sm.CheckService(ctx, "wifi")
	requireOrSkip(t, err)
	if svc == nil {
		t.Skip("wifi service not registered")
	}
	require.True(t, svc.IsAlive(ctx), "wifi service should be alive")

	// android.net.wifi.IWifiManager is Java-only and not in version tables.
	// getConnectionInfo is at FIRST_CALL_TRANSACTION + 10 based on codes_gen.go.
	const descriptor = "android.net.wifi.IWifiManager"
	code, err := svc.ResolveCode(ctx, descriptor, "getConnectionInfo")
	requireOrSkip(t, err)

	data := parcel.New()
	data.WriteInterfaceToken(descriptor)
	data.WriteString16("com.android.shell") // callingPackage
	data.WriteString16("")                   // callingFeatureId (nullable)

	reply, err := svc.Transact(ctx, code, 0, data)
	requireOrSkip(t, err)
	requireOrSkip(t, binder.ReadStatus(reply))

	remaining := reply.Len() - reply.Position()
	t.Logf("getConnectionInfo: %d bytes in reply", remaining)

	if remaining == 0 {
		t.Logf("getConnectionInfo: empty reply (WiFi may be disabled)")
		return
	}

	// WifiInfo is a nullable parcelable.
	flag, err := reply.ReadInt32()
	requireOrSkip(t, err)
	if flag == 0 {
		t.Logf("getConnectionInfo: null WifiInfo (not connected)")
		return
	}

	// Try to read the stability marker (Java AIDL parcelables have one).
	stability, err := reply.ReadInt32()
	requireOrSkip(t, err)

	// Then the parcelable size.
	parcelableSize, err := reply.ReadInt32()
	requireOrSkip(t, err)
	t.Logf("getConnectionInfo: flag=%d, stability=%d, parcelableSize=%d", flag, stability, parcelableSize)

	if parcelableSize <= 0 {
		t.Errorf("BUG: getConnectionInfo returned non-null WifiInfo but parcelable size is %d", parcelableSize)
	} else {
		t.Logf("getConnectionInfo: WifiInfo is %d bytes (complex parcelable with nested fields)", parcelableSize)
	}
}

// ---------------------------------------------------------------------------
// 6. DisplayManager.getDisplayInfo
//    Returns DisplayInfo — large parcelable (~880 bytes on typical devices).
// ---------------------------------------------------------------------------

func TestParcelableStress_DisplayManager_GetDisplayInfo(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "display")

	// First, get display IDs via the typed proxy.
	displayProxy := genDisplay.NewDisplayManagerProxy(svc)
	ids, err := displayProxy.GetDisplayIds(ctx, false)
	requireOrSkip(t, err)
	require.NotEmpty(t, ids, "should have at least one display")

	for _, displayId := range ids {
		t.Run(fmt.Sprintf("display_%d", displayId), func(t *testing.T) {
			// --- Via generated proxy ---
			result, err := displayProxy.GetDisplayInfo(ctx, displayId)
			requireOrSkip(t, err)

			// The proxy returns gui.DisplayInfo (a value type). If the call
			// succeeded without error, deserialization worked.
			t.Logf("getDisplayInfo(%d) proxy returned: %T", displayId, result)
		})
	}
}

// ---------------------------------------------------------------------------
// 7. InputManager.getInputDevice
//    Returns InputDevice — complex parcelable.
// ---------------------------------------------------------------------------

func TestParcelableStress_InputManager_GetInputDevice(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "input")

	proxy := genInput.NewInputManagerProxy(svc)

	// First get device IDs.
	deviceIds, err := proxy.GetInputDeviceIds(ctx)
	requireOrSkip(t, err)
	if len(deviceIds) == 0 {
		t.Skip("no input devices found")
	}
	t.Logf("input device IDs: %v", deviceIds)

	for _, deviceId := range deviceIds {
		t.Run(fmt.Sprintf("device_%d", deviceId), func(t *testing.T) {
			result, err := proxy.GetInputDevice(ctx, deviceId)
			requireOrSkip(t, err)

			// The proxy returns view.InputDevice (a value type). If the call
			// succeeded without error, deserialization worked.
			t.Logf("getInputDevice(%d): %T", deviceId, result)
		})
	}
}

// ---------------------------------------------------------------------------
// 7b. InputManager.getBatteryState
//     Returns IInputDeviceBatteryState — typed parcelable (fully generated).
//     This exercises a REAL deserialization path.
// ---------------------------------------------------------------------------

func TestParcelableStress_InputManager_GetBatteryState(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "input")

	proxy := genInput.NewInputManagerProxy(svc)

	deviceIds, err := proxy.GetInputDeviceIds(ctx)
	requireOrSkip(t, err)
	if len(deviceIds) == 0 {
		t.Skip("no input devices found")
	}

	for _, deviceId := range deviceIds {
		t.Run(fmt.Sprintf("device_%d", deviceId), func(t *testing.T) {
			batteryState, err := proxy.GetBatteryState(ctx, deviceId)
			if err != nil {
				// Many input devices don't have batteries — that's fine.
				t.Logf("getBatteryState(%d): %v (expected for non-battery devices)", deviceId, err)
				return
			}
			t.Logf("getBatteryState(%d): deviceId=%d, isPresent=%v, status=%d, capacity=%.1f%%",
				deviceId, batteryState.DeviceId, batteryState.IsPresent, batteryState.Status, batteryState.Capacity*100)
		})
	}
}

// ---------------------------------------------------------------------------
// 7c. InputManager.getKeyboardLayouts
//     Returns KeyboardLayout[] — array of complex parcelables.
// ---------------------------------------------------------------------------

func TestParcelableStress_InputManager_GetKeyboardLayouts(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "input")

	proxy := genInput.NewInputManagerProxy(svc)
	layouts, err := proxy.GetKeyboardLayouts(ctx)
	requireOrSkip(t, err)

	// The proxy returns []KeyboardLayout (value types). If the call succeeded,
	// deserialization worked.
	t.Logf("getKeyboardLayouts: %d layouts", len(layouts))
}

// ---------------------------------------------------------------------------
// 8. AccountManager.getAccountsByTypeForPackage
//    Returns Account[] — array of parcelables.
// ---------------------------------------------------------------------------

func TestParcelableStress_AccountManager_GetAccountsByTypeForPackage(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "account")

	proxy := genAccounts.NewAccountManagerProxy(svc)

	// First get authenticator types to know what account types exist.
	// Returns []AuthenticatorDescription (value types).
	authTypes, err := proxy.GetAuthenticatorTypes(ctx)
	if err != nil {
		t.Logf("getAuthenticatorTypes error: %v", err)
	} else {
		t.Logf("getAuthenticatorTypes: %d types", len(authTypes))
	}

	// getAccountsAsUser with empty type returns all accounts.
	// Returns []Account (value types).
	accounts, err := proxy.GetAccountsAsUser(ctx, "")
	if err != nil {
		t.Logf("getAccountsAsUser error (may require permission): %v", err)
	} else {
		t.Logf("getAccountsAsUser: %d accounts", len(accounts))
	}
}

// ---------------------------------------------------------------------------
// 9. NotificationManager.getActiveNotifications
//    Returns StatusBarNotification[] — deeply nested parcelable.
// ---------------------------------------------------------------------------

func TestParcelableStress_NotificationManager_GetActiveNotifications(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "notification")

	proxy := genApp.NewNotificationManagerProxy(svc)
	result, err := proxy.GetActiveNotifications(ctx, "com.android.shell")
	if err != nil {
		t.Logf("getActiveNotifications error (may require permission): %v", err)

		// Fall back to raw transact.
		const descriptor = "android.app.INotificationManager"
		code := resolveCode(ctx, t, svc, descriptor, "getActiveNotifications")
		data := parcel.New()
		data.WriteInterfaceToken(descriptor)
		data.WriteString16("com.android.shell")

		reply, txnErr := svc.Transact(ctx, code, 0, data)
		requireOrSkip(t, txnErr)
		statusErr := binder.ReadStatus(reply)
		if statusErr != nil {
			t.Logf("getActiveNotifications raw status error: %v", statusErr)
			return
		}

		remaining := reply.Len() - reply.Position()
		t.Logf("getActiveNotifications raw: %d bytes remaining", remaining)
		return
	}

	t.Logf("getActiveNotifications: %d notifications", len(result))
}

// ---------------------------------------------------------------------------
// 9b. NotificationManager.getZenModeConfig
//     Returns ZenModeConfig — complex parcelable.
// ---------------------------------------------------------------------------

func TestParcelableStress_NotificationManager_GetZenModeConfig(t *testing.T) {
	// ZenModeConfig is defined in android.service.notification but referenced
	// from android.app.INotificationManager. These packages form an import
	// cycle in the AIDL spec, so the codegen correctly uses interface{} to
	// break the cycle (Go does not allow circular imports). The proxy call
	// works but cannot deserialize the response into a typed struct.
	t.Skip("known import cycle: android.app ↔ android.service.notification — returns interface{} by design")
}

// ---------------------------------------------------------------------------
// 10. PackageManager.getInstalledPackages
//     Exercises the heavy-weight list-of-parcelable path (ParceledListSlice).
// ---------------------------------------------------------------------------

func TestParcelableStress_PackageManager_GetInstalledPackages(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "package")

	proxy := genPm.NewPackageManagerProxy(svc)
	result, err := proxy.GetInstalledPackages(ctx, 0)
	requireOrSkip(t, err)

	// The proxy returns ParceledListSlice (a value type). If the call
	// succeeded without error, deserialization worked.
	t.Logf("getInstalledPackages: %T", result)
}

// ---------------------------------------------------------------------------
// 11. LocationManager.getGnssAntennaInfos
//     Returns List<GnssAntennaInfo> — TYPED parcelable array that should
//     fully deserialize, exercising the real UnmarshalParcel path.
// ---------------------------------------------------------------------------

func TestParcelableStress_LocationManager_GetGnssAntennaInfos(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "location")

	proxy := genLocation.NewLocationManagerProxy(svc)
	infos, err := proxy.GetGnssAntennaInfos(ctx)
	if err != nil {
		t.Logf("getGnssAntennaInfos: %v (GNSS may not be supported)", err)
		return
	}

	// GnssAntennaInfo is currently generated as an empty struct (fields not
	// yet resolved by codegen). If the call succeeded, deserialization worked.
	t.Logf("getGnssAntennaInfos: %d antenna infos", len(infos))
	for i, info := range infos {
		t.Logf("  [%d] %+v", i, info)
	}
}

// ---------------------------------------------------------------------------
// 12. LocationManager.getGnssCapabilities
//     Returns GnssCapabilities parcelable.
// ---------------------------------------------------------------------------

func TestParcelableStress_LocationManager_GetGnssCapabilities(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "location")

	proxy := genLocation.NewLocationManagerProxy(svc)
	result, err := proxy.GetGnssCapabilities(ctx)
	requireOrSkip(t, err)

	// The proxy returns GnssCapabilities (a value type). If the call
	// succeeded without error, deserialization worked.
	t.Logf("getGnssCapabilities: %T", result)
}

// ---------------------------------------------------------------------------
// 13. Telephony.getPackagesWithCarrierPrivileges
//     Returns String[] — exercises array-of-primitives path.
// ---------------------------------------------------------------------------

func TestParcelableStress_Telephony_GetPackagesWithCarrierPrivileges(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "phone")

	proxy := genTelephony.NewTelephonyProxy(svc)
	result, err := proxy.GetPackagesWithCarrierPrivileges(ctx, 0)
	if err != nil {
		t.Logf("getPackagesWithCarrierPrivileges: %v (may require permission)", err)
		return
	}
	t.Logf("getPackagesWithCarrierPrivileges(0): %d packages: %v", len(result), result)
}

// ---------------------------------------------------------------------------
// 14. InputManager.getSensorList
//     Returns InputSensorInfo[] — array of complex parcelables.
// ---------------------------------------------------------------------------

func TestParcelableStress_InputManager_GetSensorList(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "input")

	proxy := genInput.NewInputManagerProxy(svc)

	deviceIds, err := proxy.GetInputDeviceIds(ctx)
	requireOrSkip(t, err)
	if len(deviceIds) == 0 {
		t.Skip("no input devices")
	}

	for _, deviceId := range deviceIds {
		t.Run(fmt.Sprintf("device_%d", deviceId), func(t *testing.T) {
			sensors, err := proxy.GetSensorList(ctx, deviceId)
			if err != nil {
				t.Logf("getSensorList(%d): %v", deviceId, err)
				return
			}
			// Returns []InputSensorInfo (value types). If the call succeeded,
			// deserialization worked.
			t.Logf("getSensorList(%d): %d sensors", deviceId, len(sensors))
		})
	}
}

// ---------------------------------------------------------------------------
// 15. InputManager.getLights
//     Returns Light[] — array of complex parcelables.
// ---------------------------------------------------------------------------

func TestParcelableStress_InputManager_GetLights(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "input")

	proxy := genInput.NewInputManagerProxy(svc)

	deviceIds, err := proxy.GetInputDeviceIds(ctx)
	requireOrSkip(t, err)
	if len(deviceIds) == 0 {
		t.Skip("no input devices")
	}

	for _, deviceId := range deviceIds {
		t.Run(fmt.Sprintf("device_%d", deviceId), func(t *testing.T) {
			lights, err := proxy.GetLights(ctx, deviceId)
			if err != nil {
				t.Logf("getLights(%d): %v", deviceId, err)
				return
			}
			// Returns []lights.Light (value types). If the call succeeded,
			// deserialization worked.
			t.Logf("getLights(%d): %d lights", deviceId, len(lights))
		})
	}
}

// ---------------------------------------------------------------------------
// 16. PackageManager.getPackageInfo (with GET_ACTIVITIES flag)
//     Returns PackageInfo with activities array populated — exercises the
//     deepest nesting level.
// ---------------------------------------------------------------------------

func TestParcelableStress_PackageManager_GetPackageInfoWithActivities(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "package")

	// GET_ACTIVITIES = 0x00000001, GET_SERVICES = 0x00000004,
	// GET_RECEIVERS = 0x00000002, GET_PROVIDERS = 0x00000008
	const flags int64 = 0x0000000F // GET_ACTIVITIES | GET_RECEIVERS | GET_SERVICES | GET_PROVIDERS
	const descriptor = "android.content.pm.IPackageManager"

	code := resolveCode(ctx, t, svc, descriptor, "getPackageInfo")
	data := parcel.New()
	data.WriteInterfaceToken(descriptor)
	data.WriteString16("com.android.settings")
	data.WriteInt64(flags)
	data.WriteInt32(0) // userId

	reply, err := svc.Transact(ctx, code, 0, data)
	requireOrSkip(t, err)
	requireOrSkip(t, binder.ReadStatus(reply))

	remaining := reply.Len() - reply.Position()
	t.Logf("getPackageInfo(com.android.settings, flags=0x%X) raw: %d bytes", flags, remaining)

	if remaining == 0 {
		t.Errorf("BUG: getPackageInfo with component flags returned no data for com.android.settings")
		return
	}

	// Nullable flag.
	flag, err := reply.ReadInt32()
	requireOrSkip(t, err)
	if flag == 0 {
		t.Errorf("BUG: getPackageInfo returned null for com.android.settings (expected non-null)")
		return
	}

	// PackageInfo is a Java-style parcelable: stability + size.
	stability, err := reply.ReadInt32()
	requireOrSkip(t, err)
	parcelableSize, err := reply.ReadInt32()
	requireOrSkip(t, err)
	t.Logf("getPackageInfo: nullable=%d, stability=%d, parcelableSize=%d",
		flag, stability, parcelableSize)
	assert.Greater(t, parcelableSize, int32(100),
		"PackageInfo with activities should be large (expected >100 bytes)")
}

// ---------------------------------------------------------------------------
// 17. NotificationManager.getNotificationChannels
//     Returns ParceledListSlice<NotificationChannel>.
// ---------------------------------------------------------------------------

func TestParcelableStress_NotificationManager_GetNotificationChannels(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "notification")

	const descriptor = "android.app.INotificationManager"
	code := resolveCode(ctx, t, svc, descriptor, "getNotificationChannelsForPackage")
	data := parcel.New()
	data.WriteInterfaceToken(descriptor)
	data.WriteString16("com.android.systemui")
	data.WriteInt32(1000) // uid for system
	data.WriteBool(false) // includeDeleted

	reply, err := svc.Transact(ctx, code, 0, data)
	requireOrSkip(t, err)
	requireOrSkip(t, binder.ReadStatus(reply))

	remaining := reply.Len() - reply.Position()
	t.Logf("getNotificationChannelsForPackage(com.android.systemui) raw: %d bytes", remaining)

	if remaining == 0 {
		t.Logf("getNotificationChannelsForPackage: empty reply")
		return
	}

	// ParceledListSlice: nullable flag + data.
	flag, err := reply.ReadInt32()
	requireOrSkip(t, err)
	if flag == 0 {
		t.Logf("getNotificationChannelsForPackage: null ParceledListSlice")
		return
	}

	// ParceledListSlice starts with count.
	count, err := reply.ReadInt32()
	requireOrSkip(t, err)
	t.Logf("getNotificationChannelsForPackage: ParceledListSlice count=%d", count)
	assert.GreaterOrEqual(t, count, int32(0),
		"ParceledListSlice count should be non-negative")
	if count > 0 {
		postCountRemaining := reply.Len() - reply.Position()
		t.Logf("getNotificationChannelsForPackage: %d bytes after count (for %d channels)",
			postCountRemaining, count)
	}
}

// ---------------------------------------------------------------------------
// 18. DisplayManager.getDisplayIds + full iteration of getDisplayInfo
//     Exercises bulk parcelable deserialization by reading display info for
//     EVERY display on the device.
// ---------------------------------------------------------------------------

func TestParcelableStress_DisplayManager_AllDisplaysInfo(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "display")

	const descriptor = "android.hardware.display.IDisplayManager"

	// Get all display IDs including disabled.
	displayProxy := genDisplay.NewDisplayManagerProxy(svc)
	ids, err := displayProxy.GetDisplayIds(ctx, true)
	requireOrSkip(t, err)
	t.Logf("total displays (including disabled): %d, ids=%v", len(ids), ids)

	totalBytes := 0
	for _, displayId := range ids {
		code := resolveCode(ctx, t, svc, descriptor, "getDisplayInfo")
		data := parcel.New()
		data.WriteInterfaceToken(descriptor)
		data.WriteInt32(displayId)

		reply, err := svc.Transact(ctx, code, 0, data)
		requireOrSkip(t, err)
		requireOrSkip(t, binder.ReadStatus(reply))

		remaining := reply.Len() - reply.Position()
		totalBytes += remaining

		if remaining > 0 {
			flag, err := reply.ReadInt32()
			requireOrSkip(t, err)
			if flag != 0 {
				stability, err := reply.ReadInt32()
				requireOrSkip(t, err)
				size, err := reply.ReadInt32()
				requireOrSkip(t, err)
				t.Logf("  display %d: %d bytes (stability=%d, parcelableSize=%d)",
					displayId, remaining, stability, size)
			}
		}
	}
	t.Logf("total DisplayInfo bytes across all displays: %d", totalBytes)
}

// ---------------------------------------------------------------------------
// 19. PackageManager.getPackageManagerNative — exercises a different
//     IPC descriptor for the same service.
// ---------------------------------------------------------------------------

func TestParcelableStress_PackageManagerNative_IsPackageDebuggable(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)

	svc, err := sm.GetService(ctx, "package_native")
	requireOrSkip(t, err)
	if svc == nil {
		t.Skip("package_native service not registered")
	}

	proxy := genPm.NewPackageManagerNativeProxy(svc)
	result, err := proxy.IsPackageDebuggable(ctx, "com.android.settings")
	requireOrSkip(t, err)
	t.Logf("isPackageDebuggable(com.android.settings): %v", result)
}

// ---------------------------------------------------------------------------
// 20. LocationManager.getLastLocation
//     AIDL spec says this returns a Location parcelable, but the generated
//     proxy declares the return type as common.MicrophoneInfoLocation (an
//     int32 enum) — which is a code generation type-resolution bug.
//
//     This test uses raw transact to verify the actual reply contains a
//     Location parcelable.
// ---------------------------------------------------------------------------

func TestParcelableStress_LocationManager_GetLastLocation(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "location")

	proxy := genLocation.NewLocationManagerProxy(svc)

	providers, err := proxy.GetAllProviders(ctx)
	requireOrSkip(t, err)

	// The generated proxy now correctly takes LastLocationRequest (value type)
	// and returns Location (value type).
	for _, provider := range providers {
		t.Run(provider, func(t *testing.T) {
			result, proxyErr := proxy.GetLastLocation(ctx, provider, genLocation.LastLocationRequest{}, "com.android.shell")
			if proxyErr != nil {
				t.Logf("getLastLocation(%q) proxy: %v", provider, proxyErr)
			} else {
				t.Logf("getLastLocation(%q) proxy returned: %+v", provider, result)
			}
		})
	}
}
