// Query Bluetooth adapter status and scan for BLE devices via binder.
//
// Demonstrates:
//   - IBluetoothManager proxy: adapter state
//   - RegisterAdapter → IBluetooth proxy: GetBluetoothGatt, GetBluetoothScan
//   - IBluetoothScan proxy: RegisterScanner, StartScan, StopScan
//   - BLE scan callback delivery
//
// Build:
//
//	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o build/bluetooth_status ./examples/bluetooth_status/
//	adb push build/bluetooth_status /data/local/tmp/ && adb shell /data/local/tmp/bluetooth_status
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	genBluetooth "github.com/AndroidGoLab/binder/android/bluetooth"
	genLE "github.com/AndroidGoLab/binder/android/bluetooth/le"
	"github.com/AndroidGoLab/binder/android/content"
	genOs "github.com/AndroidGoLab/binder/android/os"
	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/binder/versionaware"
	"github.com/AndroidGoLab/binder/kernelbinder"
	"github.com/AndroidGoLab/binder/servicemanager"
)

type noopManagerCallback struct{}

func (noopManagerCallback) OnBluetoothServiceUp(context.Context, binder.IBinder) error { return nil }
func (noopManagerCallback) OnBluetoothServiceDown(context.Context) error               { return nil }
func (noopManagerCallback) OnBluetoothOn(context.Context) error                        { return nil }
func (noopManagerCallback) OnBluetoothOff(context.Context) error                       { return nil }

type scanSpy struct {
	registeredCh chan int32
	results      chan genLE.ScanResult
}

func (s *scanSpy) OnScannerRegistered(_ context.Context, status, scannerID int32) error {
	if status != 0 {
		fmt.Fprintf(os.Stderr, "scanner registration failed: status=%d\n", status)
	}
	select {
	case s.registeredCh <- scannerID:
	default:
	}
	return nil
}
func (s *scanSpy) OnScanResult(_ context.Context, result genLE.ScanResult) error {
	select {
	case s.results <- result:
	default:
	}
	return nil
}
func (s *scanSpy) OnBatchScanResults(context.Context, []genLE.ScanResult) error { return nil }
func (s *scanSpy) OnFoundOrLost(context.Context, bool, genLE.ScanResult) error  { return nil }
func (s *scanSpy) OnScanManagerErrorCallback(context.Context, int32) error       { return nil }

func shellAttribution() content.AttributionSource {
	return content.AttributionSource{
		AttributionSourceState: content.AttributionSourceState{
			Pid:         int32(os.Getpid()),
			Uid:         int32(os.Getuid()),
			PackageName: "com.android.shell",
		},
	}
}

func main() {
	ctx := context.Background()

	driver, err := kernelbinder.Open(ctx, binder.WithMapSize(128*1024))
	if err != nil {
		fmt.Fprintf(os.Stderr, "open binder: %v\n", err)
		os.Exit(1)
	}
	defer driver.Close(ctx)

	transport, err := versionaware.NewTransport(ctx, driver, 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "transport: %v\n", err)
		os.Exit(1)
	}
	defer transport.Close(ctx)

	sm := servicemanager.New(transport)

	btMgrSvc, err := sm.GetService(ctx, servicemanager.ServiceName("bluetooth_manager"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "bluetooth_manager: %v\n", err)
		os.Exit(1)
	}
	mgr := genBluetooth.NewBluetoothManagerProxy(btMgrSvc)

	state, _ := mgr.GetState(ctx)
	fmt.Printf("Bluetooth state:   %d\n", state)

	callback := genBluetooth.NewBluetoothManagerCallbackStub(noopManagerCallback{})
	btAdapterBinder, err := mgr.RegisterAdapter(ctx, callback)
	if err != nil || btAdapterBinder == nil {
		fmt.Println("IBluetooth:        not available")
		os.Exit(0)
	}
	btProxy := genBluetooth.NewBluetoothProxy(btAdapterBinder)
	fmt.Printf("IBluetooth:        handle %d\n", btAdapterBinder.Handle())

	gattBinder, err := btProxy.GetBluetoothGatt(ctx)
	if err == nil && gattBinder != nil {
		fmt.Printf("IBluetoothGatt:    handle %d\n", gattBinder.Handle())
	}

	scanBinder, err := btProxy.GetBluetoothScan(ctx)
	if err != nil || scanBinder == nil {
		fmt.Println("IBluetoothScan:    not available")
		os.Exit(0)
	}
	scanProxy := genBluetooth.NewBluetoothScanProxy(scanBinder)
	fmt.Printf("IBluetoothScan:    handle %d\n", scanBinder.Handle())

	spy := &scanSpy{
		registeredCh: make(chan int32, 1),
		results:      make(chan genLE.ScanResult, 100),
	}
	scanCallback := genLE.NewScannerCallbackStub(spy)

	if err := scanProxy.RegisterScanner(ctx, scanCallback, genOs.WorkSource{}, shellAttribution()); err != nil {
		fmt.Fprintf(os.Stderr, "registerScanner: %v\n", err)
		os.Exit(1)
	}

	var scannerID int32
	select {
	case scannerID = <-spy.registeredCh:
		fmt.Printf("Scanner registered: id=%d\n", scannerID)
	case <-time.After(5 * time.Second):
		fmt.Fprintln(os.Stderr, "scanner registration timed out")
		os.Exit(1)
	}

	ss := genLE.ScanSettings{
		CallbackType:          1,
		MatchMode:             1,
		NumOfMatchesPerFilter: 3,
		Phy:                   255,
	}
	if err := scanProxy.StartScan(ctx, scannerID, ss, nil, shellAttribution()); err != nil {
		fmt.Fprintf(os.Stderr, "startScan: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Scanning for BLE devices (3s)...")
	deadline := time.After(3 * time.Second)
	count := 0
	rssiMin, rssiMax := int32(0), int32(-128)
loop:
	for {
		select {
		case result := <-spy.results:
			count++
			if result.Rssi < rssiMin {
				rssiMin = result.Rssi
			}
			if result.Rssi > rssiMax {
				rssiMax = result.Rssi
			}
		case <-deadline:
			break loop
		}
	}
	fmt.Printf("BLE scan results:  %d callbacks in 3s\n", count)
	if count > 0 {
		fmt.Printf("RSSI range:        %d to %d dBm\n", rssiMin, rssiMax)
	}

	if err := scanProxy.StopScan(ctx, scannerID, shellAttribution()); err != nil {
		fmt.Fprintf(os.Stderr, "stopScan: %v\n", err)
	}
}
