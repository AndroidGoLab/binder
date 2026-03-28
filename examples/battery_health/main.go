// Query battery health from the system battery properties service.
//
// Uses the generated IBatteryPropertiesRegistrar proxy via the
// "batteryproperties" binder service. Falls back to sysfs if binder
// access is denied.
//
// Build:
//
//	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o build/battery_health ./examples/battery_health/
//	adb push battery_health /data/local/tmp/ && adb shell /data/local/tmp/battery_health
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	genOs "github.com/AndroidGoLab/binder/android/os"
	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/binder/versionaware"
	"github.com/AndroidGoLab/binder/kernelbinder"
	"github.com/AndroidGoLab/binder/servicemanager"
)

// Android BatteryManager property IDs (from android.os.BatteryManager).
const (
	batteryPropertyChargeCounter = 1 // BATTERY_PROPERTY_CHARGE_COUNTER (uAh)
	batteryPropertyCurrentNow    = 2 // BATTERY_PROPERTY_CURRENT_NOW (uA)
	batteryPropertyCurrentAvg    = 3 // BATTERY_PROPERTY_CURRENT_AVERAGE (uA)
	batteryPropertyCapacity      = 4 // BATTERY_PROPERTY_CAPACITY (%)
	batteryPropertyStatus        = 6 // BATTERY_PROPERTY_STATUS
)

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
		fmt.Fprintf(os.Stderr, "version-aware transport: %v\n", err)
		os.Exit(1)
	}

	sm := servicemanager.New(transport)
	proxy, err := genOs.GetBatteryPropertiesRegistrar(ctx, sm)
	if err != nil {
		fmt.Fprintf(os.Stderr, "get batteryproperties service: %v\n", err)
		fmt.Fprintln(os.Stderr, "Falling back to sysfs...")
		printSysfs()
		os.Exit(0)
	}

	binderOK := false

	type propQuery struct {
		name string
		id   int32
		unit string
	}

	queries := []propQuery{
		{"Battery level", batteryPropertyCapacity, "%"},
		{"Charge counter", batteryPropertyChargeCounter, " uAh"},
		{"Current draw", batteryPropertyCurrentNow, " uA"},
		{"Current average", batteryPropertyCurrentAvg, " uA"},
		{"Battery status", batteryPropertyStatus, ""},
	}

	for _, q := range queries {
		status, err := proxy.GetProperty(ctx, q.id, genOs.BatteryProperty{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "GetProperty(%s): %v\n", q.name, err)
			continue
		}
		binderOK = true
		if q.id == batteryPropertyStatus {
			fmt.Printf("  %-20s %s (%d)\n", q.name+":", statusToString(int32(status)), status)
		} else {
			fmt.Printf("  %-20s %d%s\n", q.name+":", status, q.unit)
		}
	}

	if !binderOK {
		fmt.Fprintln(os.Stderr, "\nBinder calls failed; falling back to sysfs...")
		printSysfs()
	}
}

func statusToString(s int32) string {
	switch s {
	case 1:
		return "Unknown"
	case 2:
		return "Charging"
	case 3:
		return "Discharging"
	case 4:
		return "Not charging"
	case 5:
		return "Full"
	default:
		return fmt.Sprintf("Unknown(%d)", s)
	}
}

func printSysfs() {
	paths := map[string]string{
		"Battery level":  "/sys/class/power_supply/battery/capacity",
		"Battery status": "/sys/class/power_supply/battery/status",
		"Charge counter": "/sys/class/power_supply/battery/charge_counter",
		"Current draw":   "/sys/class/power_supply/battery/current_now",
		"Voltage":        "/sys/class/power_supply/battery/voltage_now",
		"Temperature":    "/sys/class/power_supply/battery/temp",
		"Health":         "/sys/class/power_supply/battery/health",
		"Technology":     "/sys/class/power_supply/battery/technology",
	}

	for name, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		fmt.Printf("  %-20s %s\n", name+":", strings.TrimSpace(string(data)))
	}
}
