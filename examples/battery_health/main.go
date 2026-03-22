// Query battery health from the system battery properties service.
//
// Uses IBatteryPropertiesRegistrar via the "batteryproperties" service to query
// battery properties via raw binder transact. Falls back to reading sysfs
// if binder access is denied.
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

	"github.com/xaionaro-go/binder/binder"
	"github.com/xaionaro-go/binder/binder/versionaware"
	"github.com/xaionaro-go/binder/kernelbinder"
	"github.com/xaionaro-go/binder/parcel"
	"github.com/xaionaro-go/binder/servicemanager"
)

// Android BatteryManager property IDs (from android.os.BatteryManager).
const (
	batteryPropertyChargeCounter = 1 // BATTERY_PROPERTY_CHARGE_COUNTER (µAh)
	batteryPropertyCurrentNow    = 2 // BATTERY_PROPERTY_CURRENT_NOW (µA)
	batteryPropertyCurrentAvg    = 3 // BATTERY_PROPERTY_CURRENT_AVERAGE (µA)
	batteryPropertyCapacity      = 4 // BATTERY_PROPERTY_CAPACITY (%)
	batteryPropertyStatus        = 6 // BATTERY_PROPERTY_STATUS
)

const descriptorBatteryProps = "android.os.IBatteryPropertiesRegistrar"

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

	svc, err := sm.GetService(ctx, servicemanager.ServiceName("batteryproperties"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "get batteryproperties service: %v\n", err)
		fmt.Fprintln(os.Stderr, "Falling back to sysfs...")
		printSysfs()
		os.Exit(0)
	}

	binderOK := false

	// Query each property using raw binder transact to properly read
	// the BatteryProperty out-parameter.
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
		val, status, err := getProperty(ctx, svc, q.id)
		if err != nil {
			fmt.Fprintf(os.Stderr, "GetProperty(%s): %v\n", q.name, err)
			continue
		}
		if status != 0 {
			fmt.Fprintf(os.Stderr, "GetProperty(%s): status=%d\n", q.name, status)
			continue
		}
		binderOK = true
		if q.id == batteryPropertyStatus {
			fmt.Printf("  %-20s %s (%d)\n", q.name+":", statusToString(int32(val)), val)
		} else {
			fmt.Printf("  %-20s %d%s\n", q.name+":", val, q.unit)
		}
	}

	if !binderOK {
		fmt.Fprintln(os.Stderr, "\nBinder calls failed; falling back to sysfs...")
		printSysfs()
	}
}

// getProperty performs a raw binder transact to IBatteryPropertiesRegistrar.getProperty,
// properly reading both the BatteryProperty out-parameter and the status code.
func getProperty(
	ctx context.Context,
	remote binder.IBinder,
	propertyID int32,
) (value int64, status int32, err error) {
	data := parcel.New()
	defer data.Recycle()
	data.WriteInterfaceToken(descriptorBatteryProps)
	data.WriteInt32(propertyID)

	code, err := remote.ResolveCode(ctx, descriptorBatteryProps, "getProperty")
	if err != nil {
		return 0, 0, fmt.Errorf("resolving getProperty: %w", err)
	}

	reply, err := remote.Transact(ctx, code, 0, data)
	if err != nil {
		return 0, 0, err
	}
	defer reply.Recycle()

	if err = binder.ReadStatus(reply); err != nil {
		return 0, 0, err
	}

	// Read the BatteryProperty out-parameter (nullable Parcelable).
	nullInd, err := reply.ReadInt32()
	if err != nil {
		return 0, 0, fmt.Errorf("reading null indicator: %w", err)
	}
	if nullInd != 0 {
		// BatteryProperty: { int64 ValueLong; String ValueString; }
		value, err = reply.ReadInt64()
		if err != nil {
			return 0, 0, fmt.Errorf("reading ValueLong: %w", err)
		}
		_, err = reply.ReadString() // ValueString (usually empty)
		if err != nil {
			return 0, 0, fmt.Errorf("reading ValueString: %w", err)
		}
	}

	// Read the return value (status code: 0 = success).
	status, err = reply.ReadInt32()
	if err != nil {
		return 0, 0, fmt.Errorf("reading status: %w", err)
	}

	return value, status, nil
}

func statusToString(status int32) string {
	switch status {
	case 1:
		return "unknown"
	case 2:
		return "charging"
	case 3:
		return "discharging"
	case 4:
		return "not charging"
	case 5:
		return "full"
	default:
		return fmt.Sprintf("unknown(%d)", status)
	}
}

func printSysfs() {
	base := "/sys/class/power_supply/"
	entries, err := os.ReadDir(base)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read sysfs: %v\n", err)
		return
	}

	for _, entry := range entries {
		dir := base + entry.Name() + "/"
		typ := readSysfs(dir + "type")
		if typ == "" {
			continue
		}
		fmt.Printf("\n=== %s (type: %s) ===\n", entry.Name(), typ)

		for _, attr := range []string{
			"status", "capacity", "charge_counter",
			"current_now", "current_avg", "voltage_now",
			"temp", "health", "technology",
		} {
			val := readSysfs(dir + attr)
			if val != "" {
				fmt.Printf("  %-18s %s\n", attr+":", val)
			}
		}
	}
}

func readSysfs(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
