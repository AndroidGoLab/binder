// Query battery health from the hardware HAL: capacity, charge status, current.
//
// Build:
//
//	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o battery_health ./examples/battery_health/
//	adb push battery_health /data/local/tmp/ && adb shell /data/local/tmp/battery_health
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/xaionaro-go/aidl/binder"
	"github.com/xaionaro-go/aidl/android/hardware/health"
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

	svc, err := sm.GetService(ctx, "android.hardware.health.IHealth/default")
	if err != nil {
		fmt.Fprintf(os.Stderr, "get health HAL: %v (may be blocked by SELinux)\n", err)
		os.Exit(1)
	}

	h := health.NewHealthProxy(svc)

	capacity, err := h.GetCapacity(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "GetCapacity: %v\n", err)
	} else {
		fmt.Printf("Battery level:    %d%%\n", capacity)
	}

	status, err := h.GetChargeStatus(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "GetChargeStatus: %v\n", err)
	} else {
		statusName := "unknown"
		switch status {
		case health.BatteryStatusUNKNOWN:
			statusName = "unknown"
		case health.BatteryStatusCHARGING:
			statusName = "charging"
		case health.BatteryStatusDISCHARGING:
			statusName = "discharging"
		case health.BatteryStatusNotCharging:
			statusName = "not charging"
		case health.BatteryStatusFULL:
			statusName = "full"
		}
		fmt.Printf("Charge status:    %s (%d)\n", statusName, status)
	}

	current, err := h.GetCurrentNowMicroamps(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "GetCurrentNowMicroamps: %v\n", err)
	} else {
		fmt.Printf("Current draw:     %d µA\n", current)
	}

	counter, err := h.GetChargeCounterUah(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "GetChargeCounterUah: %v\n", err)
	} else {
		fmt.Printf("Charge counter:   %d µAh\n", counter)
	}
}
