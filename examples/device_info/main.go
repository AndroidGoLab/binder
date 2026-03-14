// Query device information: thermal status, GNSS hardware, input devices,
// network interfaces, vibrator capabilities, screensaver state.
//
// Build:
//
//	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o device_info ./examples/device_info/
//	adb push device_info /data/local/tmp/ && adb shell /data/local/tmp/device_info
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/xaionaro-go/aidl/binder"
	"github.com/xaionaro-go/aidl/android/hardware/input"
	"github.com/xaionaro-go/aidl/android/location"
	genOs "github.com/xaionaro-go/aidl/android/os"
	"github.com/xaionaro-go/aidl/android/service/dreams"
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

	// Thermal status
	thermalSvc, err := sm.GetService(ctx, "thermalservice")
	if err == nil {
		thermal := genOs.NewThermalServiceProxy(thermalSvc)

		status, err := thermal.GetCurrentThermalStatus(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "thermal status: %v\n", err)
		} else {
			statusNames := []string{"none", "light", "moderate", "severe", "critical", "emergency", "shutdown"}
			name := "unknown"
			if int(status) < len(statusNames) {
				name = statusNames[status]
			}
			fmt.Printf("Thermal status:    %s (%d)\n", name, status)
		}

		headroom, err := thermal.GetThermalHeadroom(ctx, 10)
		if err != nil {
			fmt.Fprintf(os.Stderr, "thermal headroom: %v\n", err)
		} else {
			fmt.Printf("Thermal headroom:  %.2f (10s forecast)\n", headroom)
		}
	}

	// GNSS / Location hardware
	locSvc, err := sm.GetService(ctx, "location")
	if err == nil {
		loc := location.NewLocationManagerProxy(locSvc)

		year, err := loc.GetGnssYearOfHardware(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "GNSS year: %v\n", err)
		} else {
			fmt.Printf("GNSS hw year:      %d\n", year)
		}

		model, err := loc.GetGnssHardwareModelName(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "GNSS model: %v\n", err)
		} else {
			fmt.Printf("GNSS hw model:     %s\n", model)
		}

		geocode, err := loc.IsGeocodeAvailable(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "geocode: %v\n", err)
		} else {
			fmt.Printf("Geocode available: %v\n", geocode)
		}

		providers, err := loc.GetAllProviders(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "providers: %v\n", err)
		} else {
			fmt.Printf("Location providers: %v\n", providers)
		}
	}

	// Input devices
	inputSvc, err := sm.GetService(ctx, "input")
	if err == nil {
		inp := input.NewInputManagerProxy(inputSvc)

		ids, err := inp.GetInputDeviceIds(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "input device IDs: %v\n", err)
		} else {
			fmt.Printf("Input devices:     %d devices (IDs: %v)\n", len(ids), ids)
		}

		speed, err := inp.GetMousePointerSpeed(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "mouse speed: %v\n", err)
		} else {
			fmt.Printf("Mouse speed:       %d\n", speed)
		}
	}

	// Vibrator
	vibSvc, err := sm.GetService(ctx, "vibrator_manager")
	if err == nil {
		vib := genOs.NewVibratorManagerServiceProxy(vibSvc)

		vibIds, err := vib.GetVibratorIds(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "vibrator IDs: %v\n", err)
		} else {
			fmt.Printf("Vibrators:         %v\n", vibIds)
		}

		caps, err := vib.GetCapabilities(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "vibrator caps: %v\n", err)
		} else {
			fmt.Printf("Vibrator caps:     0x%x\n", caps)
		}
	}

	// Network interfaces
	netSvc, err := sm.GetService(ctx, "network_management")
	if err == nil {
		net := genOs.NewNetworkManagementServiceProxy(netSvc)

		ifaces, err := net.ListInterfaces(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "network interfaces: %v\n", err)
		} else {
			fmt.Printf("Network interfaces: %v\n", ifaces)
		}

		bw, err := net.IsBandwidthControlEnabled(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "bandwidth control: %v\n", err)
		} else {
			fmt.Printf("Bandwidth control: %v\n", bw)
		}
	}

	// Screensaver / Dreams
	dreamSvc, err := sm.GetService(ctx, "dreams")
	if err == nil {
		dream := dreams.NewDreamManagerProxy(dreamSvc)

		dreaming, err := dream.IsDreaming(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "dreaming: %v\n", err)
		} else {
			fmt.Printf("Screensaver on:    %v\n", dreaming)
		}
	}
}
