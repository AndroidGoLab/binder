//go:build e2e

package e2e

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/xaionaro-go/binder/servicemanager"

	genNfc "github.com/xaionaro-go/binder/android/nfc"
	genBluetooth "github.com/xaionaro-go/binder/android/bluetooth"
	genSensorSvc "github.com/xaionaro-go/binder/android/frameworks/sensorservice"
	genDrm "github.com/xaionaro-go/binder/android/hardware/drm"
	genInput "github.com/xaionaro-go/binder/android/hardware/input"
	genLocation "github.com/xaionaro-go/binder/android/location"
	genOs "github.com/xaionaro-go/binder/android/os"
	genOsStorage "github.com/xaionaro-go/binder/android/os/storage"
)

// TestDeviceHardware_GPS verifies GPS/GNSS hardware info via the location service.
func TestDeviceHardware_GPS(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "location")

	proxy := genLocation.NewLocationManagerProxy(svc)

	year, err := proxy.GetGnssYearOfHardware(ctx)
	requireOrSkip(t, err)
	t.Logf("GetGnssYearOfHardware: %d", year)

	model, err := proxy.GetGnssHardwareModelName(ctx)
	requireOrSkip(t, err)
	t.Logf("GetGnssHardwareModelName: %q", model)

	providers, err := proxy.GetAllProviders(ctx)
	requireOrSkip(t, err)
	t.Logf("GetAllProviders: %v", providers)
}

// TestDeviceHardware_Sensors verifies sensor list via android.frameworks.sensorservice.
func TestDeviceHardware_Sensors(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "android.frameworks.sensorservice.ISensorManager/default")

	proxy := genSensorSvc.NewSensorManagerProxy(svc)

	sensors, err := proxy.GetSensorList(ctx)
	requireOrSkip(t, err)
	t.Logf("GetSensorList: %d sensors", len(sensors))
}

// TestDeviceHardware_Bluetooth verifies Bluetooth adapter state via the bluetooth_manager service.
func TestDeviceHardware_Bluetooth(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "bluetooth_manager")

	proxy := genBluetooth.NewBluetoothManagerProxy(svc)

	state, err := proxy.GetState(ctx)
	requireOrSkip(t, err)
	t.Logf("GetState: %d", state)
}

// TestDeviceHardware_NFC checks whether NFC is enabled via the nfc service.
func TestDeviceHardware_NFC(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "nfc")

	proxy := genNfc.NewNfcAdapterProxy(svc)

	state, err := proxy.GetState(ctx)
	requireOrSkip(t, err)
	t.Logf("GetState: %d", state)
}

// TestDeviceHardware_Vibrator retrieves vibrator IDs and capabilities via vibrator_manager.
func TestDeviceHardware_Vibrator(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "vibrator_manager")

	proxy := genOs.NewVibratorManagerServiceProxy(svc)

	ids, err := proxy.GetVibratorIds(ctx)
	requireOrSkip(t, err)
	t.Logf("GetVibratorIds: count=%d ids=%v", len(ids), ids)
}

// TestDeviceHardware_DRM checks supported DRM crypto schemes via the widevine DRM factory.
func TestDeviceHardware_DRM(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "android.hardware.drm.IDrmFactory/widevine")

	proxy := genDrm.NewDrmFactoryProxy(svc)

	schemes, err := proxy.GetSupportedCryptoSchemes(ctx)
	requireOrSkip(t, err)
	t.Logf("GetSupportedCryptoSchemes: %d UUIDs, %d MIME types", len(schemes.Uuids), len(schemes.MimeTypes))
}

// TestDeviceHardware_Connectivity verifies the connectivity service is reachable.
// IConnectivityManager is a Java-only AIDL interface not in the version
// tables, so we cannot resolve transaction codes. Use CheckService + IsAlive
// instead (same approach as the WiFi test in device_features_test.go).
func TestDeviceHardware_Connectivity(t *testing.T) {
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

// TestDeviceHardware_Storage reads the last maintenance timestamp from the mount service.
func TestDeviceHardware_Storage(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "mount")

	proxy := genOsStorage.NewStorageManagerProxy(svc)

	last, err := proxy.LastMaintenance(ctx)
	requireOrSkip(t, err)
	t.Logf("LastMaintenance: %d", last)
}

// TestDeviceHardware_Input retrieves input device IDs via the input service.
func TestDeviceHardware_Input(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "input")

	proxy := genInput.NewInputManagerProxy(svc)

	ids, err := proxy.GetInputDeviceIds(ctx)
	requireOrSkip(t, err)
	t.Logf("GetInputDeviceIds: count=%d ids=%v", len(ids), ids)
}

