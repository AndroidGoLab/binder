// Query Android power state: interactive, power save mode, battery saver, idle.
//
// Build:
//
//	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o power_status ./examples/power_status/
//	adb push power_status /data/local/tmp/ && adb shell /data/local/tmp/power_status
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/xaionaro-go/aidl/binder"
	genOs "github.com/xaionaro-go/aidl/android/os"
	"github.com/xaionaro-go/aidl/kernelbinder"
	"github.com/xaionaro-go/aidl/servicemanager"
)

func main() {
	ctx := context.Background()

	driver, err := kernelbinder.Open(ctx, binder.WithMapSize(128*1024))
	if err != nil {
		fmt.Fprintf(os.Stderr, "open binder: %v\n", err)
		os.Exit(1)
	}
	defer driver.Close(ctx)

	sm := servicemanager.New(driver)

	svc, err := sm.GetService(ctx, "power")
	if err != nil {
		fmt.Fprintf(os.Stderr, "get power service: %v\n", err)
		os.Exit(1)
	}

	power := genOs.NewPowerManagerProxy(svc)

	interactive, err := power.IsInteractive(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "IsInteractive: %v\n", err)
	} else {
		fmt.Printf("Interactive (screen on): %v\n", interactive)
	}

	powerSave, err := power.IsPowerSaveMode(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "IsPowerSaveMode: %v\n", err)
	} else {
		fmt.Printf("Power save mode:        %v\n", powerSave)
	}

	batterySaver, err := power.IsBatterySaverSupported(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "IsBatterySaverSupported: %v\n", err)
	} else {
		fmt.Printf("Battery saver supported: %v\n", batterySaver)
	}

	idle, err := power.IsDeviceIdleMode(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "IsDeviceIdleMode: %v\n", err)
	} else {
		fmt.Printf("Device idle (Doze):     %v\n", idle)
	}

	lowPower, err := power.IsLowPowerStandbyEnabled(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "IsLowPowerStandbyEnabled: %v\n", err)
	} else {
		fmt.Printf("Low power standby:      %v\n", lowPower)
	}
}
