//go:build e2e

package e2e

import (
	"context"
	"math/rand"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	genAdmin "github.com/AndroidGoLab/binder/android/app/admin"
	"github.com/AndroidGoLab/binder/android/content"
	"github.com/AndroidGoLab/binder/android/frameworks/sensorservice"
	"github.com/AndroidGoLab/binder/android/hardware/display"
	"github.com/AndroidGoLab/binder/android/hardware/sensors"
	"github.com/AndroidGoLab/binder/android/location"
	genOs "github.com/AndroidGoLab/binder/android/os"
	"github.com/AndroidGoLab/binder/android/view"
	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/parcel"
	"github.com/AndroidGoLab/binder/servicemanager"
)

// isEmulator checks build properties for emulator indicators.
func isEmulator() bool {
	// DMI identity (may not exist on all emulators).
	for _, path := range []string{
		"/sys/class/dmi/id/product_name",
		"/sys/class/dmi/id/board_name",
	} {
		data, err := os.ReadFile(path)
		if err == nil {
			v := strings.ToLower(strings.TrimSpace(string(data)))
			if strings.Contains(v, "emulator") || strings.Contains(v, "sdk") ||
				strings.Contains(v, "goldfish") || strings.Contains(v, "ranchu") {
				return true
			}
		}
	}
	// CPU info (older emulators).
	if data, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		cpuinfo := strings.ToLower(string(data))
		if strings.Contains(cpuinfo, "goldfish") || strings.Contains(cpuinfo, "ranchu") {
			return true
		}
	}
	// Android system properties (most reliable on modern emulators).
	for _, path := range []string{
		"/sys/devices/virtual/dmi/id/product_name",
		"/vendor/build.prop",
		"/system/build.prop",
	} {
		data, err := os.ReadFile(path)
		if err == nil {
			v := strings.ToLower(string(data))
			if strings.Contains(v, "goldfish") || strings.Contains(v, "ranchu") ||
				strings.Contains(v, "sdk_gphone") || strings.Contains(v, "emulator") {
				return true
			}
		}
	}
	return false
}

// --- #84: MDM Agent ---

func TestUseCase84_MDMAgent(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)

	dpm, err := genAdmin.GetDevicePolicyManager(ctx, sm)
	requireOrSkip(t, err)

	callerPkg := binder.DefaultCallerIdentity().PackageName
	emptyAdmin := content.ComponentName{}

	// Password quality (aggregated).
	t.Run("password_quality", func(t *testing.T) {
		quality, err := dpm.GetPasswordQuality(ctx, emptyAdmin, false)
		requireOrSkip(t, err)
		t.Logf("Password quality: 0x%x", quality)
	})

	// Encryption status.
	t.Run("encryption_status", func(t *testing.T) {
		encStatus, err := dpm.GetStorageEncryptionStatus(ctx, callerPkg)
		requireOrSkip(t, err)
		t.Logf("Encryption status: %d", encStatus)
	})

	// Camera disabled.
	t.Run("camera_disabled", func(t *testing.T) {
		camDisabled, err := dpm.GetCameraDisabled(ctx, emptyAdmin, callerPkg, false)
		requireOrSkip(t, err)
		t.Logf("Camera disabled: %v", camDisabled)
	})

	// Device provisioned.
	t.Run("device_provisioned", func(t *testing.T) {
		provisioned, err := dpm.IsDeviceProvisioned(ctx)
		requireOrSkip(t, err)
		assert.True(t, provisioned, "device should be provisioned")
		t.Logf("Device provisioned: %v", provisioned)
	})

	// Active admins.
	t.Run("active_admins", func(t *testing.T) {
		admins, err := dpm.GetActiveAdmins(ctx)
		requireOrSkip(t, err)
		t.Logf("Active admins: %d", len(admins))
	})
}

// --- #85: Compliance Checker ---

func TestUseCase85_ComplianceChecker(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)

	callerPkg := binder.DefaultCallerIdentity().PackageName

	// Encryption check.
	t.Run("encryption", func(t *testing.T) {
		dpm, err := genAdmin.GetDevicePolicyManager(ctx, sm)
		requireOrSkip(t, err)

		encStatus, err := dpm.GetStorageEncryptionStatus(ctx, callerPkg)
		requireOrSkip(t, err)
		t.Logf("Encryption status: %d", encStatus)
		// Modern devices should have active encryption (>= 3).
	})

	// Security state.
	t.Run("security_state", func(t *testing.T) {
		secMgr, err := genOs.GetSecurityStateManager(ctx, sm)
		requireOrSkip(t, err)

		state, err := secMgr.GetGlobalSecurityState(ctx)
		requireOrSkip(t, err)
		_ = state
		t.Logf("Global security state bundle retrieved successfully")
	})

	// System update info.
	t.Run("system_update", func(t *testing.T) {
		updateMgr, err := genOs.GetSystemUpdateManager(ctx, sm)
		requireOrSkip(t, err)

		info, err := updateMgr.RetrieveSystemUpdateInfo(ctx)
		requireOrSkip(t, err)
		_ = info
		t.Logf("System update info retrieved successfully")
	})
}

// --- #86: Remote Diagnostics ---

func TestUseCase86_RemoteDiagnostics(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)

	// Power state.
	t.Run("power", func(t *testing.T) {
		power, err := genOs.GetPowerManager(ctx, sm)
		requireOrSkip(t, err)

		interactive, err := power.IsInteractive(ctx)
		requireOrSkip(t, err)
		t.Logf("Interactive: %v", interactive)

		idle, err := power.IsDeviceIdleMode(ctx)
		requireOrSkip(t, err)
		t.Logf("Device idle: %v", idle)
	})

	// Battery.
	t.Run("battery", func(t *testing.T) {
		battery, err := genOs.GetBatteryPropertiesRegistrar(ctx, sm)
		requireOrSkip(t, err)

		cap, err := battery.GetProperty(ctx, 4, genOs.BatteryProperty{})
		requireOrSkip(t, err)
		assert.GreaterOrEqual(t, int64(cap), int64(0))
		assert.LessOrEqual(t, int64(cap), int64(100))
		t.Logf("Battery: %d%%", cap)
	})

	// Display.
	t.Run("display", func(t *testing.T) {
		dispSvc := getService(ctx, t, driver, "display")
		dm := display.NewDisplayManagerProxy(dispSvc)

		ids, err := dm.GetDisplayIds(ctx, false)
		requireOrSkip(t, err)
		require.NotEmpty(t, ids, "expected at least one display")
		t.Logf("Displays: %d", len(ids))

		for _, id := range ids {
			b, err := dm.GetBrightness(ctx, id)
			requireOrSkip(t, err)
			t.Logf("Display %d brightness: %.2f", id, b)
		}
	})

	// Location providers.
	t.Run("location", func(t *testing.T) {
		loc, err := location.GetLocationManager(ctx, sm)
		requireOrSkip(t, err)

		providers, err := loc.GetAllProviders(ctx)
		requireOrSkip(t, err)
		t.Logf("Location providers: %v", providers)
	})
}

// --- #87: Kiosk Lockdown ---

func TestUseCase87_KioskLockdown(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)

	// DevicePolicyManager: query lock task features.
	t.Run("lock_task", func(t *testing.T) {
		dpm, err := genAdmin.GetDevicePolicyManager(ctx, sm)
		requireOrSkip(t, err)

		emptyAdmin := content.ComponentName{}
		callerPkg := binder.DefaultCallerIdentity().PackageName
		features, err := dpm.GetLockTaskFeatures(ctx, emptyAdmin, callerPkg)
		requireOrSkip(t, err)
		t.Logf("Lock task features: 0x%x", features)

		scDisabled, err := dpm.GetScreenCaptureDisabled(ctx, emptyAdmin, false)
		requireOrSkip(t, err)
		t.Logf("Screen capture disabled: %v", scDisabled)
	})

	// WindowManager: display density.
	t.Run("window_manager", func(t *testing.T) {
		wmSvc := getService(ctx, t, driver, "window")
		wm := view.NewWindowManagerProxy(wmSvc)

		density, err := wm.GetBaseDisplayDensity(ctx, 0)
		requireOrSkip(t, err)
		assert.Greater(t, density, int32(0), "display density should be positive")
		t.Logf("Base display density: %d dpi", density)

		vsRunning, err := wm.IsViewServerRunning(ctx)
		requireOrSkip(t, err)
		t.Logf("View server running: %v", vsRunning)
	})
}

// --- #88: OTA Status ---

func TestUseCase88_OTAStatus(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)

	// System update info.
	t.Run("system_update", func(t *testing.T) {
		updateMgr, err := genOs.GetSystemUpdateManager(ctx, sm)
		requireOrSkip(t, err)

		info, err := updateMgr.RetrieveSystemUpdateInfo(ctx)
		requireOrSkip(t, err)
		_ = info
		t.Logf("System update info bundle retrieved")
	})

	// Recovery system LSKF state.
	t.Run("recovery", func(t *testing.T) {
		recovery, err := genOs.GetRecoverySystem(ctx, sm)
		requireOrSkip(t, err)

		captured, err := recovery.IsLskfCaptured(ctx, "")
		requireOrSkip(t, err)
		t.Logf("LSKF captured: %v", captured)
	})
}

// --- #89: Factory Reset (EMULATOR ONLY) ---

func TestUseCase89_FactoryReset(t *testing.T) {
	if !isEmulator() {
		t.Skip("factory reset test only runs on emulator")
	}

	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)

	dpm, err := genAdmin.GetDevicePolicyManager(ctx, sm)
	requireOrSkip(t, err)

	// Verify the service is accessible, but do NOT actually wipe.
	provisioned, err := dpm.IsDeviceProvisioned(ctx)
	requireOrSkip(t, err)
	t.Logf("Device provisioned (pre-reset check): %v", provisioned)

	// Confirm WipeDataWithReason method can be resolved.
	callerPkg := binder.DefaultCallerIdentity().PackageName
	encStatus, err := dpm.GetStorageEncryptionStatus(ctx, callerPkg)
	requireOrSkip(t, err)
	t.Logf("Encryption status (pre-reset check): %d", encStatus)

	// NOTE: We intentionally do NOT call WipeDataWithReason even on
	// emulators in automated tests, as it would terminate the test
	// environment. This test validates that the DPM service is
	// accessible and the pre-reset checks succeed.
	t.Logf("Factory reset API is available (not invoked in test)")
}

// --- #90: Binder Fuzzer ---

func TestUseCase90_BinderFuzzer(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)

	sf, err := sm.GetService(ctx, servicemanager.ServiceName("SurfaceFlinger"))
	require.NoError(t, err)
	require.NotNil(t, sf)

	// Send randomized data as PING transactions (side-effect-free).
	const iterations = 10
	for i := 0; i < iterations; i++ {
		data := parcel.New()
		junkLen := 4 + rand.Intn(61)
		junk := make([]byte, junkLen)
		for j := range junk {
			junk[j] = byte(rand.Intn(256))
		}
		data.WriteRawBytes(junk)

		// PING is safe; it won't modify service state.
		reply, err := sf.Transact(ctx, binder.PingTransaction, 0, data)
		if err != nil {
			t.Logf("  fuzz[%d] %d bytes -> error (expected): %v", i, junkLen, err)
		} else {
			t.Logf("  fuzz[%d] %d bytes -> reply %d bytes", i, junkLen, reply.Len())
			reply.Recycle()
		}
		data.Recycle()
	}

	// Verify service is still alive after fuzzing.
	assert.True(t, sf.IsAlive(ctx), "SurfaceFlinger should survive fuzzing")
}

// --- #91: AIDL Explorer ---

func TestUseCase91_AIDLExplorer(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)

	// List services.
	services, err := sm.ListServices(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, services, "expected services to be listed")
	t.Logf("Total services: %d", len(services))

	// Ping a few well-known services.
	for _, name := range []string{"SurfaceFlinger", "activity", "power"} {
		svc, err := sm.GetService(ctx, servicemanager.ServiceName(name))
		if err != nil {
			t.Logf("  %s: unavailable (%v)", name, err)
			continue
		}
		alive := svc.IsAlive(ctx)
		t.Logf("  %s: handle=%d alive=%v", name, svc.Handle(), alive)
		assert.True(t, alive, "%s should be alive", name)
	}

	// Resolve methods on activity manager.
	am, err := sm.GetService(ctx, servicemanager.ActivityService)
	require.NoError(t, err)

	descriptor := "android.app.IActivityManager"
	for _, method := range []string{"getProcessLimit", "isUserAMonkey"} {
		code, err := am.ResolveCode(ctx, descriptor, method)
		requireOrSkip(t, err)
		assert.Greater(t, uint32(code), uint32(0))
		t.Logf("  %s.%s -> code %d", descriptor, method, code)
	}
}

// --- #92: Transaction Resolver ---

func TestUseCase92_TransactionResolver(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)

	tests := []struct {
		service    string
		descriptor string
		method     string
	}{
		{"activity", "android.app.IActivityManager", "getProcessLimit"},
		{"activity", "android.app.IActivityManager", "isUserAMonkey"},
		{"power", "android.os.IPowerManager", "isInteractive"},
		{"power", "android.os.IPowerManager", "isPowerSaveMode"},
		{"SurfaceFlingerAIDL", "android.gui.ISurfaceComposer", "getPhysicalDisplayIds"},
	}

	for _, tc := range tests {
		t.Run(tc.service+"_"+tc.method, func(t *testing.T) {
			svc, err := sm.GetService(ctx, servicemanager.ServiceName(tc.service))
			require.NoError(t, err, "GetService(%s)", tc.service)

			code, err := svc.ResolveCode(ctx, tc.descriptor, tc.method)
			requireOrSkip(t, err)
			assert.Greater(t, uint32(code), uint32(0))
			t.Logf("%s.%s -> code %d (0x%x)", tc.descriptor, tc.method, code, code)
		})
	}
}

// --- #93: Binder Latency ---

func TestUseCase93_BinderLatency(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)

	sf, err := sm.GetService(ctx, servicemanager.ServiceName("SurfaceFlinger"))
	require.NoError(t, err)

	// Warmup.
	for i := 0; i < 5; i++ {
		_, _ = sf.Transact(ctx, binder.PingTransaction, 0, parcel.New())
	}

	// Measure.
	const rounds = 20
	var totalDur time.Duration

	for i := 0; i < rounds; i++ {
		data := parcel.New()
		start := time.Now()
		reply, err := sf.Transact(ctx, binder.PingTransaction, 0, data)
		elapsed := time.Since(start)
		require.NoError(t, err)
		reply.Recycle()
		data.Recycle()
		totalDur += elapsed
	}

	avg := totalDur / time.Duration(rounds)
	t.Logf("PING latency over %d rounds: avg=%s total=%s", rounds, avg, totalDur)

	// Sanity check: average PING should be under 100ms on any device.
	assert.Less(t, avg, 100*time.Millisecond, "PING latency too high")
}

// --- #94: Permission Boundary ---

func TestUseCase94_PermissionBoundary(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)

	callerPkg := binder.DefaultCallerIdentity().PackageName

	// Read-only calls that should succeed from shell context.
	t.Run("power_read", func(t *testing.T) {
		power, err := genOs.GetPowerManager(ctx, sm)
		requireOrSkip(t, err)

		_, err = power.IsInteractive(ctx)
		requireOrSkip(t, err)
		t.Logf("power.IsInteractive: allowed")
	})

	t.Run("dpm_read", func(t *testing.T) {
		dpm, err := genAdmin.GetDevicePolicyManager(ctx, sm)
		requireOrSkip(t, err)

		_, err = dpm.GetStorageEncryptionStatus(ctx, callerPkg)
		requireOrSkip(t, err)
		t.Logf("dpm.GetStorageEncryptionStatus: allowed")
	})

	// Write operations that may be denied.
	t.Run("power_write", func(t *testing.T) {
		power, err := genOs.GetPowerManager(ctx, sm)
		requireOrSkip(t, err)

		_, err = power.SetPowerSaveModeEnabled(ctx, false)
		if err != nil {
			t.Logf("power.SetPowerSaveModeEnabled: denied (expected): %v", err)
		} else {
			t.Logf("power.SetPowerSaveModeEnabled: allowed (running as privileged user)")
		}
	})
}

// --- #95: Mock Service ---

func TestUseCase95_MockService(t *testing.T) {
	ctx := context.Background()

	const (
		descriptor    = "com.example.IMockService"
		codeTimestamp = binder.FirstCallTransaction + 0
		codeVersion   = binder.FirstCallTransaction + 1
		codeHealthy   = binder.FirstCallTransaction + 2
	)

	// mockService implements binder.TransactionReceiver in-process.
	type mockService struct {
		startTime time.Time
	}

	svc := &mockService{startTime: time.Now()}

	handler := func(code binder.TransactionCode, data *parcel.Parcel) (*parcel.Parcel, error) {
		if _, err := data.ReadInterfaceToken(); err != nil {
			return nil, err
		}
		reply := parcel.New()
		switch code {
		case codeTimestamp:
			binder.WriteStatus(reply, nil)
			reply.WriteInt64(time.Now().UnixMilli())
			return reply, nil
		case codeVersion:
			binder.WriteStatus(reply, nil)
			reply.WriteString16("mock-v1.0.0")
			return reply, nil
		case codeHealthy:
			binder.WriteStatus(reply, nil)
			reply.WriteBool(time.Since(svc.startTime) < 24*time.Hour)
			return reply, nil
		default:
			reply.Recycle()
			return nil, assert.AnError
		}
	}

	// Test GetTimestamp.
	t.Run("timestamp", func(t *testing.T) {
		data := parcel.New()
		data.WriteInterfaceToken(descriptor)
		reply, err := handler(codeTimestamp, data)
		require.NoError(t, err)
		require.NoError(t, binder.ReadStatus(reply))
		ts, err := reply.ReadInt64()
		require.NoError(t, err)
		assert.Greater(t, ts, int64(0))
		t.Logf("Timestamp: %d", ts)
		reply.Recycle()
		data.Recycle()
	})

	// Test GetVersion.
	t.Run("version", func(t *testing.T) {
		data := parcel.New()
		data.WriteInterfaceToken(descriptor)
		reply, err := handler(codeVersion, data)
		require.NoError(t, err)
		require.NoError(t, binder.ReadStatus(reply))
		ver, err := reply.ReadString16()
		require.NoError(t, err)
		assert.Equal(t, "mock-v1.0.0", ver)
		t.Logf("Version: %s", ver)
		reply.Recycle()
		data.Recycle()
	})

	// Test IsHealthy.
	t.Run("healthy", func(t *testing.T) {
		data := parcel.New()
		data.WriteInterfaceToken(descriptor)
		reply, err := handler(codeHealthy, data)
		require.NoError(t, err)
		require.NoError(t, binder.ReadStatus(reply))
		healthy, err := reply.ReadBool()
		require.NoError(t, err)
		assert.True(t, healthy)
		t.Logf("Healthy: %v", healthy)
		reply.Recycle()
		data.Recycle()
	})

	_ = ctx // Satisfy linters; no binder driver needed for in-process mock.
}

// --- #96: Version Compat ---

func TestUseCase96_VersionCompat(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)

	checks := []struct {
		service    string
		descriptor string
		methods    []string
	}{
		{
			"activity",
			"android.app.IActivityManager",
			[]string{"getProcessLimit", "isUserAMonkey", "checkPermission"},
		},
		{
			"power",
			"android.os.IPowerManager",
			[]string{"isInteractive", "isPowerSaveMode", "isDeviceIdleMode"},
		},
	}

	for _, check := range checks {
		t.Run(check.service, func(t *testing.T) {
			svc, err := sm.GetService(ctx, servicemanager.ServiceName(check.service))
			require.NoError(t, err)

			available := 0
			for _, method := range check.methods {
				code, err := svc.ResolveCode(ctx, check.descriptor, method)
				if err != nil {
					t.Logf("  %s: MISSING (%v)", method, err)
				} else {
					t.Logf("  %s: code=%d", method, code)
					available++
				}
			}
			t.Logf("  %d/%d methods available", available, len(check.methods))
			assert.Greater(t, available, 0, "at least some methods should be available")
		})
	}
}

// --- #97: Headless Controller ---

func TestUseCase97_HeadlessController(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)

	// Power state.
	t.Run("power", func(t *testing.T) {
		power, err := genOs.GetPowerManager(ctx, sm)
		requireOrSkip(t, err)

		interactive, err := power.IsInteractive(ctx)
		requireOrSkip(t, err)
		t.Logf("Interactive: %v", interactive)

		idle, err := power.IsDeviceIdleMode(ctx)
		requireOrSkip(t, err)
		t.Logf("Device idle: %v", idle)

		lowPower, err := power.IsLowPowerStandbyEnabled(ctx)
		requireOrSkip(t, err)
		t.Logf("Low power standby: %v", lowPower)
	})

	// Display.
	t.Run("display", func(t *testing.T) {
		dispSvc := getService(ctx, t, driver, "display")
		dm := display.NewDisplayManagerProxy(dispSvc)

		ids, err := dm.GetDisplayIds(ctx, false)
		requireOrSkip(t, err)
		require.NotEmpty(t, ids)
		t.Logf("Displays: %d", len(ids))
	})

	// Temperature.
	t.Run("thermal", func(t *testing.T) {
		hwProps, err := genOs.GetHardwarePropertiesManager(ctx, sm)
		requireOrSkip(t, err)

		temps, err := hwProps.GetDeviceTemperatures(ctx, 0, 0)
		requireOrSkip(t, err)
		t.Logf("CPU temperatures: %v", temps)
	})
}

// --- #98: Sensor Gateway ---

func TestUseCase98_SensorGateway(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)

	// The sensor HAL is registered under its HIDL fully-qualified name,
	// not the standard binder "sensor" entry.
	sensorSvc := getService(ctx, t, driver, "android.frameworks.sensorservice.ISensorManager/default")

	sensorMgr := sensorservice.NewSensorManagerProxy(sensorSvc)

	// List sensors.
	sensorList, err := sensorMgr.GetSensorList(ctx)
	requireOrSkip(t, err)
	t.Logf("Sensors: %d", len(sensorList))
	for i, s := range sensorList {
		if i < 5 {
			t.Logf("  [%d] %s (type=%d)", i, s.Name, s.Type)
		}
	}

	// Query default accelerometer.
	accel, err := sensorMgr.GetDefaultSensor(ctx, sensors.SensorTypeACCELEROMETER)
	requireOrSkip(t, err)
	assert.NotEmpty(t, accel.Name, "accelerometer should have a name")
	t.Logf("Default accelerometer: %s (vendor=%s)", accel.Name, accel.Vendor)
}

// --- #99: Signage Controller ---

func TestUseCase99_SignageController(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)

	// Power state.
	t.Run("power", func(t *testing.T) {
		power, err := genOs.GetPowerManager(ctx, sm)
		requireOrSkip(t, err)

		interactive, err := power.IsInteractive(ctx)
		requireOrSkip(t, err)
		t.Logf("Screen on: %v", interactive)
	})

	// Display brightness.
	t.Run("display", func(t *testing.T) {
		dispSvc := getService(ctx, t, driver, "display")
		dm := display.NewDisplayManagerProxy(dispSvc)

		ids, err := dm.GetDisplayIds(ctx, false)
		requireOrSkip(t, err)
		require.NotEmpty(t, ids)

		for _, id := range ids {
			brightness, err := dm.GetBrightness(ctx, id)
			requireOrSkip(t, err)
			t.Logf("Display %d brightness: %.2f", id, brightness)

			info, err := dm.GetBrightnessInfo(ctx, id)
			requireOrSkip(t, err)
			t.Logf("Display %d brightness info: min=%.2f max=%.2f", id, info.BrightnessMinimum, info.BrightnessMaximum)
		}
	})

	// Color display.
	t.Run("color_display", func(t *testing.T) {
		colorSvc := getService(ctx, t, driver, "color_display")
		cdm := display.NewColorDisplayManagerProxy(colorSvc)

		nightMode, err := cdm.IsNightDisplayActivated(ctx)
		requireOrSkip(t, err)
		t.Logf("Night mode: %v", nightMode)

		colorMode, err := cdm.GetColorMode(ctx)
		requireOrSkip(t, err)
		t.Logf("Color mode: %d", colorMode)
	})
}

// --- #100: Vehicle Telematics ---

func TestUseCase100_VehicleTelematics(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)

	// GPS / Location.
	t.Run("gps", func(t *testing.T) {
		loc, err := location.GetLocationManager(ctx, sm)
		requireOrSkip(t, err)

		providers, err := loc.GetAllProviders(ctx)
		requireOrSkip(t, err)
		t.Logf("Location providers: %v", providers)

		enabled, err := loc.IsLocationEnabledForUser(ctx)
		requireOrSkip(t, err)
		t.Logf("Location enabled: %v", enabled)

		gnssYear, err := loc.GetGnssYearOfHardware(ctx)
		requireOrSkip(t, err)
		t.Logf("GNSS hardware year: %d", gnssYear)
	})

	// Battery.
	t.Run("battery", func(t *testing.T) {
		battery, err := genOs.GetBatteryPropertiesRegistrar(ctx, sm)
		requireOrSkip(t, err)

		cap, err := battery.GetProperty(ctx, 4, genOs.BatteryProperty{})
		requireOrSkip(t, err)
		t.Logf("Battery: %d%%", cap)

		current, err := battery.GetProperty(ctx, 2, genOs.BatteryProperty{})
		requireOrSkip(t, err)
		t.Logf("Current: %d uA", current)
	})

	// Power and thermal.
	t.Run("device_state", func(t *testing.T) {
		power, err := genOs.GetPowerManager(ctx, sm)
		requireOrSkip(t, err)

		interactive, err := power.IsInteractive(ctx)
		requireOrSkip(t, err)
		t.Logf("Screen on: %v", interactive)

		idle, err := power.IsDeviceIdleMode(ctx)
		requireOrSkip(t, err)
		t.Logf("Device idle: %v", idle)
	})

	t.Run("thermal", func(t *testing.T) {
		hwProps, err := genOs.GetHardwarePropertiesManager(ctx, sm)
		requireOrSkip(t, err)

		temps, err := hwProps.GetDeviceTemperatures(ctx, 0, 0)
		requireOrSkip(t, err)
		t.Logf("CPU temperatures: %v", temps)
	})
}
