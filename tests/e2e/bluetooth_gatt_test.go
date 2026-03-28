//go:build e2e

package e2e

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
	genOs "github.com/AndroidGoLab/binder/android/os"
	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/parcel"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func requireOrSkipNullPointer(t *testing.T, err error) {
	t.Helper()
	if err != nil && strings.Contains(err.Error(), "exception NullPointer") {
		t.Skipf("NullPointer (Bluetooth adapter likely off): %v", err)
	}
	requireOrSkip(t, err)
}

func shellAttribution() content.AttributionSource {
	return content.AttributionSource{
		AttributionSourceState: content.AttributionSourceState{
			Pid:         int32(os.Getpid()),
			Uid:         int32(os.Getuid()),
			PackageName: "com.android.shell",
		},
	}
}

// ---------------------------------------------------------------------------
// Callback stubs
// ---------------------------------------------------------------------------

type noopBluetoothManagerCallback struct{}

func (n *noopBluetoothManagerCallback) OnBluetoothServiceUp(context.Context, binder.IBinder) error {
	return nil
}
func (n *noopBluetoothManagerCallback) OnBluetoothServiceDown(context.Context) error { return nil }
func (n *noopBluetoothManagerCallback) OnBluetoothOn(context.Context) error          { return nil }
func (n *noopBluetoothManagerCallback) OnBluetoothOff(context.Context) error         { return nil }

var _ genBluetooth.IBluetoothManagerCallbackServer = (*noopBluetoothManagerCallback)(nil)

type gattCallbackSpy struct {
	registeredCh chan int32
}

func (s *gattCallbackSpy) OnClientRegistered(_ context.Context, status int32) error {
	select {
	case s.registeredCh <- status:
	default:
	}
	return nil
}
func (s *gattCallbackSpy) OnClientConnectionState(context.Context, int32, bool, genBluetooth.BluetoothDevice) error {
	return nil
}
func (s *gattCallbackSpy) OnPhyUpdate(context.Context, genBluetooth.BluetoothDevice, int32, int32, int32) error {
	return nil
}
func (s *gattCallbackSpy) OnPhyRead(context.Context, genBluetooth.BluetoothDevice, int32, int32, int32) error {
	return nil
}
func (s *gattCallbackSpy) OnSearchComplete(context.Context, genBluetooth.BluetoothDevice, []genBluetooth.BluetoothGattService, int32) error {
	return nil
}
func (s *gattCallbackSpy) OnCharacteristicRead(context.Context, genBluetooth.BluetoothDevice, int32, int32, []byte) error {
	return nil
}
func (s *gattCallbackSpy) OnCharacteristicWrite(context.Context, genBluetooth.BluetoothDevice, int32, int32, []byte) error {
	return nil
}
func (s *gattCallbackSpy) OnExecuteWrite(context.Context, genBluetooth.BluetoothDevice, int32) error {
	return nil
}
func (s *gattCallbackSpy) OnDescriptorRead(context.Context, genBluetooth.BluetoothDevice, int32, int32, []byte) error {
	return nil
}
func (s *gattCallbackSpy) OnDescriptorWrite(context.Context, genBluetooth.BluetoothDevice, int32, int32, []byte) error {
	return nil
}
func (s *gattCallbackSpy) OnNotify(context.Context, genBluetooth.BluetoothDevice, int32, []byte) error {
	return nil
}
func (s *gattCallbackSpy) OnReadRemoteRssi(context.Context, genBluetooth.BluetoothDevice, int32, int32) error {
	return nil
}
func (s *gattCallbackSpy) OnConfigureMTU(context.Context, genBluetooth.BluetoothDevice, int32, int32) error {
	return nil
}
func (s *gattCallbackSpy) OnConnectionUpdated(context.Context, genBluetooth.BluetoothDevice, int32, int32, int32, int32) error {
	return nil
}
func (s *gattCallbackSpy) OnServiceChanged(context.Context, genBluetooth.BluetoothDevice) error {
	return nil
}
func (s *gattCallbackSpy) OnSubrateChange(context.Context, genBluetooth.BluetoothDevice, int32, int32) error {
	return nil
}

var _ genBluetooth.IBluetoothGattCallbackServer = (*gattCallbackSpy)(nil)

type scanCallbackSpy struct {
	registeredCh chan scanRegisteredEvent
	resultCh     chan genLE.ScanResult
}

type scanRegisteredEvent struct {
	status    int32
	scannerID int32
}

func (s *scanCallbackSpy) OnScannerRegistered(_ context.Context, status, scannerID int32) error {
	select {
	case s.registeredCh <- scanRegisteredEvent{status: status, scannerID: scannerID}:
	default:
	}
	return nil
}
func (s *scanCallbackSpy) OnScanResult(_ context.Context, result genLE.ScanResult) error {
	select {
	case s.resultCh <- result:
	default:
	}
	return nil
}
func (s *scanCallbackSpy) OnBatchScanResults(context.Context, []genLE.ScanResult) error { return nil }
func (s *scanCallbackSpy) OnFoundOrLost(context.Context, bool, genLE.ScanResult) error  { return nil }
func (s *scanCallbackSpy) OnScanManagerErrorCallback(context.Context, int32) error       { return nil }

var _ genLE.IScannerCallbackServer = (*scanCallbackSpy)(nil)

// ---------------------------------------------------------------------------
// TestBluetoothGATT_FullPipeline
// ---------------------------------------------------------------------------

func TestBluetoothGATT_FullPipeline(t *testing.T) {
	ctx := context.Background()

	driver, transport := openBinderDirect(ctx, t)
	defer func() { _ = driver.Close(ctx) }()
	defer func() { _ = transport.Close(ctx) }()

	svc := getService(ctx, t, transport, "bluetooth_manager")
	mgr := genBluetooth.NewBluetoothManagerProxy(svc)

	t.Run("ManagerState", func(t *testing.T) {
		state, err := mgr.GetState(ctx)
		requireOrSkip(t, err)
		t.Logf("Bluetooth state: %d", state)
		assert.True(t, state >= 0 && state <= 16, "unexpected state: %d", state)
	})

	t.Run("ManagerCapabilities", func(t *testing.T) {
		bleScanAvail, err := mgr.IsBleScanAvailable(ctx)
		requireOrSkipNullPointer(t, err)
		t.Logf("IsBleScanAvailable: %v", bleScanAvail)

		hearingAid, err := mgr.IsHearingAidProfileSupported(ctx)
		requireOrSkipNullPointer(t, err)
		t.Logf("IsHearingAidProfileSupported: %v", hearingAid)
	})

	t.Run("RegisterAdapter", func(t *testing.T) {
		callback := genBluetooth.NewBluetoothManagerCallbackStub(&noopBluetoothManagerCallback{})

		btAdapterBinder, err := mgr.RegisterAdapter(ctx, callback)
		requireOrSkip(t, err)
		require.NotNil(t, btAdapterBinder, "RegisterAdapter returned nil")
		btProxy := genBluetooth.NewBluetoothProxy(btAdapterBinder)
		t.Logf("IBluetooth handle: %d", btAdapterBinder.Handle())

		t.Run("GATTClientLifecycle", func(t *testing.T) {
			gattBinder, err := btProxy.GetBluetoothGatt(ctx)
			requireOrSkip(t, err)
			require.NotNil(t, gattBinder, "GetBluetoothGatt returned nil")
			t.Logf("IBluetoothGatt handle: %d", gattBinder.Handle())

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
			attr := shellAttribution()
			data.WriteInt32(1)
			attr.MarshalParcel(data)

			regReply, err := gattBinder.Transact(ctx, code, 0, data)
			requireOrSkip(t, err)
			if statusErr := binder.ReadStatus(regReply); statusErr != nil {
				t.Fatalf("registerClient error: %v", statusErr)
			}

			select {
			case status := <-spy.registeredCh:
				t.Logf("OnClientRegistered: status=%d", status)
				assert.Equal(t, int32(0), status, "expected GATT_SUCCESS")
			case <-time.After(5 * time.Second):
				t.Fatal("OnClientRegistered callback never arrived")
			}
		})

		t.Run("BLEScan", func(t *testing.T) {
			scanBinder, err := btProxy.GetBluetoothScan(ctx)
			requireOrSkip(t, err)
			require.NotNil(t, scanBinder, "GetBluetoothScan returned nil")
			scanProxy := genBluetooth.NewBluetoothScanProxy(scanBinder)
			t.Logf("IBluetoothScan handle: %d", scanBinder.Handle())

			t.Run("NumHwTrackFilters", func(t *testing.T) {
				count, err := scanProxy.NumHwTrackFiltersAvailable(ctx, shellAttribution())
				requireOrSkip(t, err)
				t.Logf("numHwTrackFiltersAvailable: %d", count)
				assert.True(t, count >= 0, "negative filter count")
			})

			t.Run("ScanForDevices", func(t *testing.T) {
				spy := &scanCallbackSpy{
					registeredCh: make(chan scanRegisteredEvent, 1),
					resultCh:     make(chan genLE.ScanResult, 10),
				}
				scanCallback := genLE.NewScannerCallbackStub(spy)

				err := scanProxy.RegisterScanner(ctx, scanCallback, genOs.WorkSource{}, shellAttribution())
				requireOrSkip(t, err)

				var scannerID int32
				select {
				case event := <-spy.registeredCh:
					t.Logf("OnScannerRegistered: status=%d scannerID=%d", event.status, event.scannerID)
					if event.status != 0 {
						t.Skipf("scanner registration failed: status %d", event.status)
					}
					scannerID = event.scannerID
				case <-time.After(5 * time.Second):
					t.Fatal("OnScannerRegistered callback never arrived")
				}

				ss := genLE.ScanSettings{
					CallbackType:          1,
					MatchMode:             1,
					NumOfMatchesPerFilter: 3,
					Phy:                   255,
				}
				err = scanProxy.StartScan(ctx, scannerID, ss, nil, shellAttribution())
				requireOrSkip(t, err)
				t.Log("startScan sent, waiting for BLE devices...")

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
				t.Logf("received %d scan result callbacks in 3s", resultCount)

				err = scanProxy.StopScan(ctx, scannerID, shellAttribution())
				requireOrSkip(t, err)
				t.Log("stopScan sent")
			})
		})
	})
}
