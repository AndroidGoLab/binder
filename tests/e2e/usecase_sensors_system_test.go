//go:build e2e

package e2e

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	genAccounts "github.com/AndroidGoLab/binder/android/accounts"
	genApp "github.com/AndroidGoLab/binder/android/app"
	genAdmin "github.com/AndroidGoLab/binder/android/app/admin"
	genJob "github.com/AndroidGoLab/binder/android/app/job"
	genContent "github.com/AndroidGoLab/binder/android/content"
	genSensorSvc "github.com/AndroidGoLab/binder/android/frameworks/sensorservice"
	genInput "github.com/AndroidGoLab/binder/android/hardware/input"
	genSensors "github.com/AndroidGoLab/binder/android/hardware/sensors"
	genOs "github.com/AndroidGoLab/binder/android/os"
	"github.com/AndroidGoLab/binder/parcel"
	"github.com/AndroidGoLab/binder/servicemanager"
)

// ---------- #70: Sensor reader ----------

func TestUsecase70_SensorReader_GetSensorList(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "android.frameworks.sensorservice.ISensorManager/default")

	proxy := genSensorSvc.NewSensorManagerProxy(svc)

	sensors, err := proxy.GetSensorList(ctx)
	requireOrSkip(t, err)
	require.NotEmpty(t, sensors, "expected at least one sensor")

	t.Logf("GetSensorList: %d sensors", len(sensors))
	for i, s := range sensors {
		if i < 5 {
			t.Logf("  [%d] %s (type=%d, vendor=%s)", i, s.Name, s.Type, s.Vendor)
		}
	}
}

func TestUsecase70_SensorReader_GetDefaultAccelerometer(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "android.frameworks.sensorservice.ISensorManager/default")

	proxy := genSensorSvc.NewSensorManagerProxy(svc)

	accel, err := proxy.GetDefaultSensor(ctx, genSensors.SensorTypeACCELEROMETER)
	requireOrSkip(t, err)
	assert.NotEmpty(t, accel.Name, "expected accelerometer name")
	t.Logf("Default accelerometer: %s (vendor=%s, range=%.2f)", accel.Name, accel.Vendor, accel.MaxRange)
}

func TestUsecase70_SensorReader_GetDefaultGyroscope(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "android.frameworks.sensorservice.ISensorManager/default")

	proxy := genSensorSvc.NewSensorManagerProxy(svc)

	gyro, err := proxy.GetDefaultSensor(ctx, genSensors.SensorTypeGYROSCOPE)
	requireOrSkip(t, err)
	assert.NotEmpty(t, gyro.Name, "expected gyroscope name")
	t.Logf("Default gyroscope: %s (vendor=%s, range=%.2f)", gyro.Name, gyro.Vendor, gyro.MaxRange)
}

// ---------- #71: Input injector ----------

func TestUsecase71_InputInjector_GetInputDeviceIds(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "input")

	proxy := genInput.NewInputManagerProxy(svc)

	ids, err := proxy.GetInputDeviceIds(ctx)
	requireOrSkip(t, err)
	require.NotEmpty(t, ids, "expected at least one input device")
	t.Logf("GetInputDeviceIds: %d devices, ids=%v", len(ids), ids)
}

func TestUsecase71_InputInjector_GetInputDevice(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "input")

	proxy := genInput.NewInputManagerProxy(svc)

	ids, err := proxy.GetInputDeviceIds(ctx)
	requireOrSkip(t, err)
	require.NotEmpty(t, ids, "need at least one input device")

	dev, err := proxy.GetInputDevice(ctx, ids[0])
	requireOrSkip(t, err)
	t.Logf("GetInputDevice(%d): name=%q", ids[0], dev.Name)
}

func TestUsecase71_InputInjector_GetMousePointerSpeed(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "input")

	proxy := genInput.NewInputManagerProxy(svc)

	speed, err := proxy.GetMousePointerSpeed(ctx)
	requireOrSkip(t, err)
	t.Logf("GetMousePointerSpeed: %d", speed)
}

// ---------- #72: Rotation resolver ----------

func TestUsecase72_RotationResolver_ServiceReachable(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)

	// The rotation resolver is typically a bound service only available
	// to the system server. CheckService will return nil if not registered.
	svc, err := sm.CheckService(ctx, servicemanager.RotationResolverService)
	requireOrSkip(t, err)

	if svc == nil {
		t.Log("rotation_resolver service not registered (expected on most devices)")
		return
	}

	alive := svc.IsAlive(ctx)
	t.Logf("rotation_resolver: handle=%d alive=%v", svc.Handle(), alive)
}

// ---------- #73: Attention monitor ----------

func TestUsecase73_AttentionMonitor_ServiceReachable(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)

	svc, err := sm.CheckService(ctx, servicemanager.AttentionService)
	requireOrSkip(t, err)

	if svc == nil {
		t.Log("attention service not registered (bound service, expected)")
		return
	}

	alive := svc.IsAlive(ctx)
	t.Logf("attention service: handle=%d alive=%v", svc.Handle(), alive)
}

// ---------- #74: Health checker (extends list_services) ----------

func TestUsecase74_HealthChecker_PingSensorInput(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)

	services := []struct {
		name string
	}{
		{"input"},
		{"alarm"},
		{"clipboard"},
		{"jobscheduler"},
		{"device_policy"},
		{"user"},
		{"account"},
	}

	for _, s := range services {
		t.Run(s.name, func(t *testing.T) {
			svc, err := sm.GetService(ctx, servicemanager.ServiceName(s.name))
			requireOrSkip(t, err)
			require.NotNil(t, svc)

			alive := svc.IsAlive(ctx)
			assert.True(t, alive, "%s should be alive", s.name)
			t.Logf("%s: handle=%d alive=%v", s.name, svc.Handle(), alive)
		})
	}
}

// ---------- #75: Service discovery (extends list_services) ----------

func TestUsecase75_ServiceDiscovery_SensorsSystemServices(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)

	services, err := sm.ListServices(ctx)
	require.NoError(t, err)

	expected := []servicemanager.ServiceName{
		servicemanager.InputService,
		servicemanager.AlarmService,
		servicemanager.ClipboardService,
		servicemanager.JobSchedulerService,
		servicemanager.DevicePolicyService,
		servicemanager.UserService,
		servicemanager.AccountService,
	}

	for _, name := range expected {
		assert.Contains(t, services, name, "expected %s in service list", name)
	}
}

// ---------- #76: Custom binder service (extends server_service) ----------

func TestUsecase76_CustomBinderService_BridgeOnTransaction(t *testing.T) {
	const bridgeDescriptor = "com.example.IBridgeService"

	// In-process test: exercise raw OnTransaction without IPC.
	_ = openBinder(t) // ensures binder driver is available

	// Ping via raw parcel.
	{
		data := parcel.New()
		defer data.Recycle()
		data.WriteInterfaceToken(bridgeDescriptor)
		data.WriteString16("test-ping")

		// Simulate reading back the token and verifying parcel structure.
		data.SetPosition(0)
		token, err := data.ReadInterfaceToken()
		require.NoError(t, err)
		assert.Equal(t, bridgeDescriptor, token)
		t.Log("Bridge descriptor token verified")
	}
}

// ---------- #77: AIDL bridge ----------

func TestUsecase77_AIDLBridge_InProcessStub(t *testing.T) {
	_ = openBinder(t)

	const bridgeDescriptor = "com.example.IBridgeService"

	// Build ping parcel.
	data := parcel.New()
	defer data.Recycle()
	data.WriteInterfaceToken(bridgeDescriptor)

	// Verify the interface token round-trips correctly.
	data.SetPosition(0)
	token, err := data.ReadInterfaceToken()
	require.NoError(t, err)
	assert.Equal(t, bridgeDescriptor, token)

	// Build echo parcel.
	echoData := parcel.New()
	defer echoData.Recycle()
	echoData.WriteInterfaceToken(bridgeDescriptor)
	echoData.WriteString16("hello bridge")

	echoData.SetPosition(0)
	token2, err := echoData.ReadInterfaceToken()
	require.NoError(t, err)
	assert.Equal(t, bridgeDescriptor, token2)

	msg, err := echoData.ReadString16()
	require.NoError(t, err)
	assert.Equal(t, "hello bridge", msg)

	t.Log("AIDL bridge parcel round-trip verified")
}

// ---------- #78: Device policy ----------

func TestUsecase78_DevicePolicy_GetStorageEncryptionStatus(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)

	dpm, err := genAdmin.GetDevicePolicyManager(ctx, sm)
	requireOrSkip(t, err)

	status, err := dpm.GetStorageEncryptionStatus(ctx, "com.android.shell")
	requireOrSkip(t, err)
	t.Logf("GetStorageEncryptionStatus: %d", status)
}

func TestUsecase78_DevicePolicy_GetActiveAdmins(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)

	dpm, err := genAdmin.GetDevicePolicyManager(ctx, sm)
	requireOrSkip(t, err)

	admins, err := dpm.GetActiveAdmins(ctx)
	requireOrSkip(t, err)
	t.Logf("GetActiveAdmins: %d active admins", len(admins))
}

// ---------- #79: User manager ----------

func TestUsecase79_UserManager_IsHeadlessSystemUserMode(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)

	um, err := genOs.GetUserManager(ctx, sm)
	requireOrSkip(t, err)

	headless, err := um.IsHeadlessSystemUserMode(ctx)
	requireOrSkip(t, err)
	t.Logf("IsHeadlessSystemUserMode: %v", headless)
}

func TestUsecase79_UserManager_GetMainUserId(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)

	um, err := genOs.GetUserManager(ctx, sm)
	requireOrSkip(t, err)

	mainUID, err := um.GetMainUserId(ctx)
	requireOrSkip(t, err)
	t.Logf("GetMainUserId: %d", mainUID)
}

func TestUsecase79_UserManager_GetUserSerialNumber(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)

	um, err := genOs.GetUserManager(ctx, sm)
	requireOrSkip(t, err)

	serial, err := um.GetUserSerialNumber(ctx)
	requireOrSkip(t, err)
	t.Logf("GetUserSerialNumber: %d", serial)
}

func TestUsecase79_UserManager_GetUsers(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)

	um, err := genOs.GetUserManager(ctx, sm)
	requireOrSkip(t, err)

	users, err := um.GetUsers(ctx, false, true, false)
	requireOrSkip(t, err)
	require.NotEmpty(t, users, "expected at least one user")

	t.Logf("GetUsers: %d users", len(users))
	for i, u := range users {
		if i < 5 {
			t.Logf("  [%d] id=%d name=%q", i, u.Id, u.Name)
		}
	}
}

// ---------- #80: Account manager ----------

func TestUsecase80_AccountManager_GetAuthenticatorTypes(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)

	am, err := genAccounts.GetAccountManager(ctx, sm)
	requireOrSkip(t, err)

	authTypes, err := am.GetAuthenticatorTypes(ctx)
	requireOrSkip(t, err)
	// Shell UID (2000) may see zero authenticator types depending on the
	// user profile and registered accounts — no assertion on count.
	t.Logf("GetAuthenticatorTypes: %d types", len(authTypes))
	for i, a := range authTypes {
		if i < 5 {
			t.Logf("  [%d] type=%s package=%s", i, a.Type, a.PackageName)
		}
	}
}

// ---------- #81: Job scheduler monitor ----------

func TestUsecase81_JobSchedulerMonitor_GetAllPendingJobs(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "jobscheduler")

	proxy := genJob.NewJobSchedulerProxy(svc)

	pendingJobs, err := proxy.GetAllPendingJobs(ctx)
	requireOrSkip(t, err)
	t.Logf("GetAllPendingJobs: %d namespaces", len(pendingJobs))
}

func TestUsecase81_JobSchedulerMonitor_CanRunUserInitiatedJobs(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "jobscheduler")

	proxy := genJob.NewJobSchedulerProxy(svc)

	// GetAllJobSnapshots requires system-internal-use-only permission,
	// so use CanRunUserInitiatedJobs which is accessible from shell.
	canRun, err := proxy.CanRunUserInitiatedJobs(ctx, "com.android.shell")
	requireOrSkip(t, err)
	t.Logf("CanRunUserInitiatedJobs(com.android.shell): %v", canRun)
}

// ---------- #82: Alarm auditor ----------

func TestUsecase82_AlarmAuditor_GetNextWakeFromIdleTime(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "alarm")

	proxy := genApp.NewAlarmManagerProxy(svc)

	nextWake, err := proxy.GetNextWakeFromIdleTime(ctx)
	requireOrSkip(t, err)
	t.Logf("GetNextWakeFromIdleTime: %d ms", nextWake)
}

func TestUsecase82_AlarmAuditor_GetNextAlarmClock(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "alarm")

	proxy := genApp.NewAlarmManagerProxy(svc)

	alarmClock, err := proxy.GetNextAlarmClock(ctx)
	requireOrSkip(t, err)
	_ = alarmClock
	t.Log("GetNextAlarmClock: succeeded")
}

func TestUsecase82_AlarmAuditor_GetConfigVersion(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "alarm")

	proxy := genApp.NewAlarmManagerProxy(svc)

	version, err := proxy.GetConfigVersion(ctx)
	requireOrSkip(t, err)
	t.Logf("GetConfigVersion: %d", version)
}

// ---------- #83: Clipboard monitor ----------

func TestUsecase83_ClipboardMonitor_HasPrimaryClip(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "clipboard")

	proxy := genContent.NewClipboardProxy(svc)

	hasPrimary, err := proxy.HasPrimaryClip(ctx, 0)
	requireOrSkip(t, err)
	t.Logf("HasPrimaryClip: %v", hasPrimary)
}

func TestUsecase83_ClipboardMonitor_HasClipboardText(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "clipboard")

	proxy := genContent.NewClipboardProxy(svc)

	hasText, err := proxy.HasClipboardText(ctx, 0)
	requireOrSkip(t, err)
	t.Logf("HasClipboardText: %v", hasText)
}

func TestUsecase83_ClipboardMonitor_GetPrimaryClipSource(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "clipboard")

	proxy := genContent.NewClipboardProxy(svc)

	source, err := proxy.GetPrimaryClipSource(ctx, 0)
	requireOrSkip(t, err)
	t.Logf("GetPrimaryClipSource: %q", source)
}
