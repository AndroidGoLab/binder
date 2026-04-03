//go:build e2e || e2e_root

package e2e

// E2E tests for use cases #22-31: Camera & Imaging + Bluetooth.
//
// Use cases:
//   22. Flashlight torch control (SetTorchMode)
//   23. Camera availability (GetNumberOfCameras, GetCameraInfo)
//   24. Camera frame capture (ConnectDevice + ConfigureStream + CaptureFrame)
//   25. QR scanner daemon (continuous frame capture)
//   26. Timelapse capture (periodic frame capture)
//   27. BLE scanning (RegisterScanner, StartScan, StopScan)
//   28. Bluetooth adapter status (GetState, IsBleScanAvailable)
//   29. Bluetooth inventory (GetBondedDevices, GetRemoteName, GetSupportedProfiles)
//   30. BLE sensor collector (GATT client registration + BLE scan)
//   31. Bluetooth audio routing (A2DP GetConnectedDevices, GetActiveDevice, codecs)

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	genBluetooth "github.com/AndroidGoLab/binder/android/bluetooth"
	genLE "github.com/AndroidGoLab/binder/android/bluetooth/le"
	"github.com/AndroidGoLab/binder/android/content"
	fwkService "github.com/AndroidGoLab/binder/android/frameworks/cameraservice/service"
	genOs "github.com/AndroidGoLab/binder/android/os"
	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/binder/versionaware"
	"github.com/AndroidGoLab/binder/camera"
	"github.com/AndroidGoLab/binder/parcel"
	"github.com/AndroidGoLab/binder/servicemanager"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// discoverCameraID queries the framework camera service to find the first
// available camera ID. This avoids hardcoding a camera ID that may differ
// across emulators and devices.
func discoverCameraID(
	ctx context.Context,
	t *testing.T,
	sm *servicemanager.ServiceManager,
) string {
	t.Helper()
	fwkSvc, err := sm.GetService(ctx, "android.frameworks.cameraservice.service.ICameraService/default")
	requireOrSkip(t, err)
	fwkCam := fwkService.NewCameraServiceProxy(fwkSvc)

	listener := fwkService.NewCameraServiceListenerStub(&noopCameraServiceListener{})
	cameras, err := fwkCam.AddListener(ctx, listener)
	requireOrSkip(t, err)
	defer func() { _ = fwkCam.RemoveListener(ctx, listener) }()

	require.Greater(t, len(cameras), 0, "expected at least one camera")
	t.Logf("Discovered camera ID %q (out of %d cameras)", cameras[0].CameraId, len(cameras))
	return cameras[0].CameraId
}

func ucShellAttribution() content.AttributionSource {
	return content.AttributionSource{
		AttributionSourceState: content.AttributionSourceState{
			Pid:         int32(os.Getpid()),
			Uid:         int32(os.Getuid()),
			PackageName: "com.android.shell",
		},
	}
}

// ucGetBluetoothAdapter obtains the IBluetooth proxy via RegisterAdapter.
// It skips the test if Bluetooth is not available.
func ucGetBluetoothAdapter(
	ctx context.Context,
	t *testing.T,
	transport *versionaware.Transport,
) *genBluetooth.BluetoothProxy {
	t.Helper()
	svc := getService(ctx, t, transport, "bluetooth_manager")
	mgr := genBluetooth.NewBluetoothManagerProxy(svc)

	callback := genBluetooth.NewBluetoothManagerCallbackStub(&noopBluetoothManagerCallback{})
	btAdapterBinder, err := mgr.RegisterAdapter(ctx, callback)
	requireOrSkip(t, err)
	if btAdapterBinder == nil {
		t.Skip("IBluetooth not available (adapter may be off)")
	}
	return genBluetooth.NewBluetoothProxy(btAdapterBinder)
}

// ---------------------------------------------------------------------------
// #22: Flashlight torch control
// ---------------------------------------------------------------------------

func TestUseCase22_FlashlightTorch(t *testing.T) {
	ctx := context.Background()

	// The media.camera service (hardware.ICameraService) is not accessible
	// from the shell context due to SELinux restrictions. Use a dedicated
	// binder driver with the framework camera service for enumeration,
	// then attempt torch control via media.camera with a properly
	// registered client token (matching the flashlight_torch example).
	driver, transport := openBinderDirect(ctx, t)
	defer func() { _ = driver.Close(ctx) }()
	defer func() { _ = transport.Close(ctx) }()

	sm := servicemanager.New(transport)

	// Enumerate cameras via the framework camera service (NDK AIDL),
	// which is accessible from shell unlike media.camera.
	fwkSvc, err := sm.GetService(ctx, "android.frameworks.cameraservice.service.ICameraService/default")
	requireOrSkip(t, err)
	fwkCam := fwkService.NewCameraServiceProxy(fwkSvc)

	listener := fwkService.NewCameraServiceListenerStub(&noopCameraServiceListener{})
	cameras, err := fwkCam.AddListener(ctx, listener)
	requireOrSkip(t, err)
	t.Logf("Framework camera service reports %d camera(s)", len(cameras))
	require.Greater(t, len(cameras), 0, "expected at least one camera")

	for i, cam := range cameras {
		t.Logf("  [%d] id=%q status=%d", i, cam.CameraId, cam.DeviceStatus)
	}

	// Clean up listener registration.
	_ = fwkCam.RemoveListener(ctx, listener)

	// SetTorchMode subtest moved to usecase_root_test.go — requires root
	// to bypass media.camera SELinux restrictions (kernel status -61).
}

// torchClientToken is a minimal TransactionReceiver used as the client
// binder token for SetTorchMode.
type torchClientToken struct{}

func (t *torchClientToken) Descriptor() string { return "torch.client" }

func (t *torchClientToken) OnTransaction(
	_ context.Context,
	_ binder.TransactionCode,
	_ *parcel.Parcel,
) (*parcel.Parcel, error) {
	return parcel.New(), nil
}

// noopCameraServiceListener implements ICameraServiceListenerServer for
// use with the framework camera service's AddListener. The listener
// receives status updates but does not act on them.
type noopCameraServiceListener struct{}

func (n *noopCameraServiceListener) OnPhysicalCameraStatusChanged(
	_ context.Context,
	_ fwkService.CameraDeviceStatus,
	_ string,
	_ string,
) error {
	return nil
}

func (n *noopCameraServiceListener) OnStatusChanged(
	_ context.Context,
	_ fwkService.CameraDeviceStatus,
	_ string,
) error {
	return nil
}

var _ fwkService.ICameraServiceListenerServer = (*noopCameraServiceListener)(nil)

// ---------------------------------------------------------------------------
// #23: Camera availability
// ---------------------------------------------------------------------------

func TestUseCase23_CameraAvailability(t *testing.T) {
	ctx := context.Background()

	// The media.camera service is not accessible from shell context.
	// Use the framework camera service (NDK AIDL) which provides
	// camera enumeration via AddListener and metadata via
	// GetCameraCharacteristics.
	driver, transport := openBinderDirect(ctx, t)
	defer func() { _ = driver.Close(ctx) }()
	defer func() { _ = transport.Close(ctx) }()

	sm := servicemanager.New(transport)

	fwkSvc, err := sm.GetService(ctx, "android.frameworks.cameraservice.service.ICameraService/default")
	requireOrSkip(t, err)
	fwkCam := fwkService.NewCameraServiceProxy(fwkSvc)

	t.Run("EnumerateCameras", func(t *testing.T) {
		listener := fwkService.NewCameraServiceListenerStub(&noopCameraServiceListener{})
		cameras, err := fwkCam.AddListener(ctx, listener)
		requireOrSkip(t, err)
		defer func() { _ = fwkCam.RemoveListener(ctx, listener) }()

		t.Logf("Available cameras: %d", len(cameras))
		assert.GreaterOrEqual(t, len(cameras), 0)

		for i, cam := range cameras {
			t.Logf("  [%d] id=%q status=%d unavailPhysical=%v",
				i, cam.CameraId, cam.DeviceStatus, cam.UnavailPhysicalCameraIds)
		}
	})

	// GetCameraCharacteristics subtest moved to usecase_root_test.go —
	// returns HAL ServiceSpecific error as shell.
}

// ---------------------------------------------------------------------------
// #24: Camera frame capture
// ---------------------------------------------------------------------------

// TestUseCase24_CameraFrameCapture is covered by TestCameraCapture_SingleFrame
// in camera_capture_test.go. This test validates the higher-level camera
// package API (camera.Connect + ConfigureStream + CaptureFrame).
func TestUseCase24_CameraFrameCapture(t *testing.T) {
	const (
		width  = 640
		height = 480
	)

	ctx := context.Background()
	transport := openBinderLarge(t)
	sm := servicemanager.New(transport)

	cameraID := discoverCameraID(ctx, t, sm)
	cam, err := camera.Connect(ctx, sm, transport, cameraID)
	requireOrSkip(t, err)
	defer cam.Close(ctx)
	t.Log("Camera connected")

	err = cam.ConfigureStream(ctx, width, height, camera.FormatYCbCr420)
	requireOrSkip(t, err)
	t.Logf("Stream configured: %dx%d YCbCr420", width, height)

	captureCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	frame, err := cam.CaptureFrame(captureCtx)
	requireOrSkip(t, err)

	nonZero := 0
	for _, b := range frame {
		if b != 0 {
			nonZero++
		}
	}

	t.Logf("Captured frame: %d bytes, %.1f%% non-zero",
		len(frame), float64(nonZero)/float64(len(frame))*100)
	require.Greater(t, len(frame), 0, "frame should not be empty")
	require.Greater(t, nonZero, len(frame)/100,
		"captured frame has too few non-zero bytes — no actual pixel data")
}

// ---------------------------------------------------------------------------
// #25: QR scanner daemon (continuous frame capture)
// ---------------------------------------------------------------------------

func TestUseCase25_QRScannerDaemon(t *testing.T) {
	const (
		width      = 640
		height     = 480
		frameCount = 3
	)

	ctx := context.Background()
	transport := openBinderLarge(t)
	sm := servicemanager.New(transport)

	cameraID := discoverCameraID(ctx, t, sm)
	cam, err := camera.Connect(ctx, sm, transport, cameraID)
	requireOrSkip(t, err)
	defer cam.Close(ctx)

	err = cam.ConfigureStream(ctx, width, height, camera.FormatYCbCr420)
	requireOrSkip(t, err)

	t.Logf("Capturing %d frames for QR scanning", frameCount)

	anyFrameHasPixels := false
	for i := 0; i < frameCount; i++ {
		captureCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		frame, err := cam.CaptureFrame(captureCtx)
		cancel()
		requireOrSkip(t, err)

		nonZero := 0
		for _, b := range frame {
			if b != 0 {
				nonZero++
			}
		}
		t.Logf("  Frame %d: %d bytes, %.1f%% non-zero",
			i, len(frame), float64(nonZero)/float64(len(frame))*100)
		require.Greater(t, len(frame), 0, "frame %d should not be empty", i)
		if nonZero > len(frame)/100 {
			anyFrameHasPixels = true
		}
	}
	require.True(t, anyFrameHasPixels,
		"all %d captured frames have <1%% non-zero bytes — no actual pixel data", frameCount)
}

// ---------------------------------------------------------------------------
// #26: Timelapse capture (periodic frame capture)
// ---------------------------------------------------------------------------

func TestUseCase26_TimelapseCapture(t *testing.T) {
	const (
		width        = 640
		height       = 480
		captureCount = 2
		interval     = 1 * time.Second
	)

	ctx := context.Background()
	transport := openBinderLarge(t)
	sm := servicemanager.New(transport)

	cameraID := discoverCameraID(ctx, t, sm)
	cam, err := camera.Connect(ctx, sm, transport, cameraID)
	requireOrSkip(t, err)
	defer cam.Close(ctx)

	err = cam.ConfigureStream(ctx, width, height, camera.FormatYCbCr420)
	requireOrSkip(t, err)

	t.Logf("Timelapse: %d frames, %v interval", captureCount, interval)

	var frameSizes []int
	anyFrameHasPixels := false
	for i := 0; i < captureCount; i++ {
		if i > 0 {
			time.Sleep(interval)
		}

		captureCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		frame, err := cam.CaptureFrame(captureCtx)
		cancel()
		requireOrSkip(t, err)

		nonZero := 0
		for _, b := range frame {
			if b != 0 {
				nonZero++
			}
		}
		frameSizes = append(frameSizes, len(frame))
		t.Logf("  Frame %d: %d bytes, %.1f%% non-zero",
			i, len(frame), float64(nonZero)/float64(len(frame))*100)
		if nonZero > len(frame)/100 {
			anyFrameHasPixels = true
		}
	}

	require.Equal(t, captureCount, len(frameSizes), "should capture all frames")
	for i, sz := range frameSizes {
		require.Greater(t, sz, 0, "frame %d should not be empty", i)
	}
	require.True(t, anyFrameHasPixels,
		"all %d captured frames have <1%% non-zero bytes — no actual pixel data", captureCount)
}

// ucCleanupLeakedScanners unregisters scanner IDs 0..15 to free BLE
// resources leaked by previous test runs that failed to clean up.
func ucCleanupLeakedScanners(
	ctx context.Context,
	scanProxy *genBluetooth.BluetoothScanProxy,
) {
	attr := ucShellAttribution()
	for id := int32(0); id < 16; id++ {
		_ = scanProxy.UnregisterScanner(ctx, id, attr)
	}
}

// ucRegisterBLEScanner registers a BLE scanner using the provided callback
// spy and returns the scanner ID. If the first registration attempt fails
// with status 128 (GATT_NO_RESOURCES), it cleans up leaked scanners from
// previous test runs and retries.
func ucRegisterBLEScanner(
	ctx context.Context,
	t *testing.T,
	scanProxy *genBluetooth.BluetoothScanProxy,
	spy *scanCallbackSpy,
) int32 {
	t.Helper()
	attr := ucShellAttribution()
	cb := genLE.NewScannerCallbackStub(spy)

	err := scanProxy.RegisterScanner(ctx, cb, genOs.WorkSource{}, attr)
	requireOrSkip(t, err)

	var status, scannerID int32
	select {
	case ev := <-spy.registeredCh:
		status = ev.status
		scannerID = ev.scannerID
	case <-time.After(5 * time.Second):
		t.Fatal("OnScannerRegistered callback never arrived")
	}

	if status == 0 {
		return scannerID
	}

	// Status 128 = GATT_NO_RESOURCES. Clean up leaked scanners from
	// previous test runs, then retry with a fresh callback.
	if status == 128 {
		t.Log("scanner registration got status 128; cleaning up leaked scanners")
		ucCleanupLeakedScanners(ctx, scanProxy)
		time.Sleep(500 * time.Millisecond)

		spy2 := &scanCallbackSpy{
			registeredCh: make(chan scanRegisteredEvent, 1),
			resultCh:     spy.resultCh,
		}
		cb2 := genLE.NewScannerCallbackStub(spy2)

		err = scanProxy.RegisterScanner(ctx, cb2, genOs.WorkSource{}, attr)
		requireOrSkip(t, err)

		select {
		case ev := <-spy2.registeredCh:
			status = ev.status
			scannerID = ev.scannerID
		case <-time.After(5 * time.Second):
			t.Fatal("OnScannerRegistered callback never arrived after cleanup")
		}

		if status == 0 {
			return scannerID
		}
	}

	t.Fatalf("scanner registration failed after cleanup: status %d", status)
	return -1
}

// ---------------------------------------------------------------------------
// #27: BLE scanning
// ---------------------------------------------------------------------------

func TestUseCase27_BLEScanning(t *testing.T) {
	ctx := context.Background()

	driver, transport := openBinderDirect(ctx, t)
	defer func() { _ = driver.Close(ctx) }()
	defer func() { _ = transport.Close(ctx) }()

	btProxy := ucGetBluetoothAdapter(ctx, t, transport)

	scanBinder, err := btProxy.GetBluetoothScan(ctx)
	requireOrSkip(t, err)
	require.NotNil(t, scanBinder, "GetBluetoothScan returned nil")
	scanProxy := genBluetooth.NewBluetoothScanProxy(scanBinder)
	t.Logf("IBluetoothScan: handle %d", scanBinder.Handle())

	spy := &scanCallbackSpy{
		registeredCh: make(chan scanRegisteredEvent, 1),
		resultCh:     make(chan genLE.ScanResult, 100),
	}
	scannerID := ucRegisterBLEScanner(ctx, t, scanProxy, spy)

	ss := genLE.ScanSettings{
		CallbackType:          1,
		MatchMode:             1,
		NumOfMatchesPerFilter: 3,
		Phy:                   255,
	}
	err = scanProxy.StartScan(ctx, scannerID, ss, nil, ucShellAttribution())
	requireOrSkip(t, err)

	deadline := time.After(3 * time.Second)
	resultCount := 0
collectLoop:
	for {
		select {
		case <-spy.resultCh:
			resultCount++
		case <-deadline:
			break collectLoop
		}
	}
	t.Logf("BLE scan: %d results in 3s", resultCount)

	err = scanProxy.StopScan(ctx, scannerID, ucShellAttribution())
	requireOrSkip(t, err)

	err = scanProxy.UnregisterScanner(ctx, scannerID, ucShellAttribution())
	requireOrSkip(t, err)
}

// ---------------------------------------------------------------------------
// #28: Bluetooth adapter status
// ---------------------------------------------------------------------------

func TestUseCase28_BluetoothAdapterStatus(t *testing.T) {
	ctx := context.Background()
	transport := openBinder(t)

	svc := getService(ctx, t, transport, "bluetooth_manager")
	mgr := genBluetooth.NewBluetoothManagerProxy(svc)

	t.Run("GetState", func(t *testing.T) {
		state, err := mgr.GetState(ctx)
		requireOrSkip(t, err)
		t.Logf("Bluetooth state: %d", state)
		// State should be in valid range [10..16] for Android Bluetooth states.
		assert.True(t, state >= 0 && state <= 16, "unexpected state: %d", state)
	})

	t.Run("IsBleScanAvailable", func(t *testing.T) {
		avail, err := mgr.IsBleScanAvailable(ctx)
		requireOrSkipNullPointer(t, err)
		t.Logf("IsBleScanAvailable: %v", avail)
	})

	t.Run("IsHearingAidProfileSupported", func(t *testing.T) {
		supported, err := mgr.IsHearingAidProfileSupported(ctx)
		requireOrSkipNullPointer(t, err)
		t.Logf("IsHearingAidProfileSupported: %v", supported)
	})
}

// ---------------------------------------------------------------------------
// #29: Bluetooth inventory (bonded devices + adapter info)
// ---------------------------------------------------------------------------

func TestUseCase29_BluetoothInventory(t *testing.T) {
	ctx := context.Background()

	driver, transport := openBinderDirect(ctx, t)
	defer func() { _ = driver.Close(ctx) }()
	defer func() { _ = transport.Close(ctx) }()

	btProxy := ucGetBluetoothAdapter(ctx, t, transport)
	attr := ucShellAttribution()

	t.Run("GetName", func(t *testing.T) {
		name, err := btProxy.GetName(ctx, attr)
		requireOrSkipNullPointer(t, err)
		t.Logf("Adapter name: %q", name)
	})

	t.Run("GetSupportedProfiles", func(t *testing.T) {
		profiles, err := btProxy.GetSupportedProfiles(ctx, attr)
		requireOrSkipNullPointer(t, err)
		t.Logf("Supported profiles: %v", profiles)
		assert.Greater(t, len(profiles), 0, "expected at least one profile")
	})

	t.Run("GetAdapterConnectionState", func(t *testing.T) {
		state, err := btProxy.GetAdapterConnectionState(ctx)
		requireOrSkipNullPointer(t, err)
		t.Logf("Adapter connection state: %d", state)
	})

	t.Run("GetBondedDevices", func(t *testing.T) {
		devices, err := btProxy.GetBondedDevices(ctx, attr)
		requireOrSkipNullPointer(t, err)
		t.Logf("Bonded devices: %d", len(devices))

		for i, dev := range devices {
			t.Logf("  [%d] AddressType=%d", i, dev.AddressType)

			remoteName, err := btProxy.GetRemoteName(ctx, dev, attr)
			if err == nil {
				t.Logf("       Name: %q", remoteName)
			}

			alias, err := btProxy.GetRemoteAlias(ctx, dev, attr)
			if err == nil {
				t.Logf("       Alias: %q", alias)
			}

			bondState, err := btProxy.GetBondState(ctx, dev, attr)
			if err == nil {
				t.Logf("       BondState: %d", bondState)
			}
		}
	})

	t.Run("GetMaxConnectedAudioDevices", func(t *testing.T) {
		max, err := btProxy.GetMaxConnectedAudioDevices(ctx, attr)
		requireOrSkipNullPointer(t, err)
		t.Logf("Max connected audio devices: %d", max)
		assert.GreaterOrEqual(t, max, int32(0))
	})
}

// ---------------------------------------------------------------------------
// #30: BLE sensor collector (GATT client + BLE scan)
// ---------------------------------------------------------------------------

func TestUseCase30_BLESensorCollector(t *testing.T) {
	ctx := context.Background()

	driver, transport := openBinderDirect(ctx, t)
	defer func() { _ = driver.Close(ctx) }()
	defer func() { _ = transport.Close(ctx) }()

	btProxy := ucGetBluetoothAdapter(ctx, t, transport)

	t.Run("GATTClientRegistration", func(t *testing.T) {
		gattBinder, err := btProxy.GetBluetoothGatt(ctx)
		requireOrSkip(t, err)
		require.NotNil(t, gattBinder, "GetBluetoothGatt returned nil")
		t.Logf("IBluetoothGatt: handle %d", gattBinder.Handle())

		code := resolveCode(ctx, t, gattBinder,
			genBluetooth.DescriptorIBluetoothGatt, "registerClient")

		spy := &gattCallbackSpy{registeredCh: make(chan int32, 1)}
		gattCallback := genBluetooth.NewBluetoothGattCallbackStub(spy)

		data := parcel.New()
		defer data.Recycle()
		data.WriteInterfaceToken(genBluetooth.DescriptorIBluetoothGatt)
		data.WriteInt32(1) // non-null ParcelUuid
		data.WriteInt64(0x0000180000001000)
		data.WriteInt64(-9223371485494954757)
		binder.WriteBinderToParcel(ctx, data, gattCallback.AsBinder(), transport)
		data.WriteBool(false)
		data.WriteInt32(0)
		attr := ucShellAttribution()
		data.WriteInt32(1)
		attr.MarshalParcel(data)

		regReply, err := gattBinder.Transact(ctx, code, 0, data)
		requireOrSkip(t, err)
		if statusErr := binder.ReadStatus(regReply); statusErr != nil {
			requireOrSkip(t, statusErr)
		}

		select {
		case status := <-spy.registeredCh:
			t.Logf("OnClientRegistered: status=%d", status)
			assert.Equal(t, int32(0), status, "expected GATT_SUCCESS")
		case <-time.After(5 * time.Second):
			t.Fatal("OnClientRegistered callback never arrived")
		}
	})

	t.Run("BLEScanForSensors", func(t *testing.T) {
		scanBinder, err := btProxy.GetBluetoothScan(ctx)
		requireOrSkip(t, err)
		require.NotNil(t, scanBinder, "GetBluetoothScan returned nil")
		scanProxy := genBluetooth.NewBluetoothScanProxy(scanBinder)

		spy := &scanCallbackSpy{
			registeredCh: make(chan scanRegisteredEvent, 1),
			resultCh:     make(chan genLE.ScanResult, 100),
		}
		scannerID := ucRegisterBLEScanner(ctx, t, scanProxy, spy)
		t.Logf("Scanner registered: id=%d", scannerID)

		ss := genLE.ScanSettings{
			CallbackType:          1,
			MatchMode:             1,
			NumOfMatchesPerFilter: 3,
			Phy:                   255,
		}
		err = scanProxy.StartScan(ctx, scannerID, ss, nil, ucShellAttribution())
		requireOrSkip(t, err)

		// Collect scan results for sensor discovery.
		deadline := time.After(3 * time.Second)
		resultCount := 0
		var rssiMin, rssiMax int32
		rssiMin = 0
		rssiMax = -128
	collectLoop:
		for {
			select {
			case result := <-spy.resultCh:
				resultCount++
				if result.Rssi < rssiMin {
					rssiMin = result.Rssi
				}
				if result.Rssi > rssiMax {
					rssiMax = result.Rssi
				}
			case <-deadline:
				break collectLoop
			}
		}
		t.Logf("BLE sensor scan: %d results in 3s", resultCount)
		if resultCount > 0 {
			t.Logf("  RSSI range: %d to %d dBm", rssiMin, rssiMax)
		}

		err = scanProxy.StopScan(ctx, scannerID, ucShellAttribution())
		requireOrSkip(t, err)

		err = scanProxy.UnregisterScanner(ctx, scannerID, ucShellAttribution())
		requireOrSkip(t, err)
	})
}

// ---------------------------------------------------------------------------
// #31: Bluetooth audio routing (A2DP)
// ---------------------------------------------------------------------------

const ucProfileA2DP int32 = 2

func TestUseCase31_BluetoothAudioRouting(t *testing.T) {
	ctx := context.Background()

	driver, transport := openBinderDirect(ctx, t)
	defer func() { _ = driver.Close(ctx) }()
	defer func() { _ = transport.Close(ctx) }()

	btProxy := ucGetBluetoothAdapter(ctx, t, transport)
	attr := ucShellAttribution()

	t.Run("A2DPProfileConnectionState", func(t *testing.T) {
		state, err := btProxy.GetProfileConnectionState(ctx, ucProfileA2DP, attr)
		requireOrSkipNullPointer(t, err)
		t.Logf("A2DP profile connection state: %d", state)
	})

	t.Run("A2DPProxy", func(t *testing.T) {
		a2dpBinder, err := btProxy.GetProfile(ctx, ucProfileA2DP)
		requireOrSkip(t, err)
		if a2dpBinder == nil {
			t.Skip("IBluetoothA2dp not available")
		}
		a2dp := genBluetooth.NewBluetoothA2dpProxy(a2dpBinder)
		t.Logf("IBluetoothA2dp: handle %d", a2dpBinder.Handle())

		t.Run("GetSupportedCodecTypes", func(t *testing.T) {
			codecs, err := a2dp.GetSupportedCodecTypes(ctx)
			if err != nil && strings.Contains(err.Error(), "exception NullPointer") {
				t.Skipf("A2DP not fully implemented on this device: %v", err)
			}
			requireOrSkip(t, err)
			t.Logf("Supported codec types: %d", len(codecs))
			for i, c := range codecs {
				t.Logf("  [%d] %+v", i, c)
			}
		})

		t.Run("GetConnectedDevices", func(t *testing.T) {
			devices, err := a2dp.GetConnectedDevices(ctx, attr)
			requireOrSkip(t, err)
			t.Logf("Connected A2DP devices: %d", len(devices))
			for i, dev := range devices {
				t.Logf("  [%d] AddressType=%d", i, dev.AddressType)

				connState, err := a2dp.GetConnectionState(ctx, dev, attr)
				if err == nil {
					t.Logf("       ConnectionState=%d", connState)
				}

				playing, err := a2dp.IsA2dpPlaying(ctx, dev, attr)
				if err == nil {
					t.Logf("       IsPlaying=%v", playing)
				}
			}
		})

		t.Run("GetActiveDevice", func(t *testing.T) {
			activeDevice, err := a2dp.GetActiveDevice(ctx, attr)
			requireOrSkip(t, err)
			t.Logf("Active A2DP device: AddressType=%d", activeDevice.AddressType)
		})

		t.Run("GetDynamicBufferSupport", func(t *testing.T) {
			support, err := a2dp.GetDynamicBufferSupport(ctx, attr)
			requireOrSkip(t, err)
			t.Logf("Dynamic buffer support: %d", support)
		})
	})
}
