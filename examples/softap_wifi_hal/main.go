// Query WiFi HAL for chip info and AP interface state.
//
// This example shows how to interact with the WiFi HAL to query chip
// capabilities, list AP interfaces, and get AP interface details.
// The WiFi HAL controls the low-level WiFi driver and is the foundation
// for both STA (client) and AP (hotspot) modes.
//
// The generated code returns typed interfaces (IWifiChip, IWifiApIface)
// directly from method calls — no manual proxy construction needed for
// sub-objects.
//
// Note: The WiFi HAL is only available on devices with real WiFi hardware.
//
// Build:
//
//	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o build/softap_wifi_hal ./examples/softap_wifi_hal/
//	adb push softap_wifi_hal /data/local/tmp/ && adb shell /data/local/tmp/softap_wifi_hal
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/xaionaro-go/binder/binder"
	"github.com/xaionaro-go/binder/binder/versionaware"
	"github.com/xaionaro-go/binder/android/hardware/wifi"
	"github.com/xaionaro-go/binder/kernelbinder"
	"github.com/xaionaro-go/binder/servicemanager"
)

const wifiHalService = servicemanager.ServiceName("android.hardware.wifi.IWifi/default")

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

	svc, err := sm.GetService(ctx, wifiHalService)
	if err != nil {
		fmt.Fprintf(os.Stderr, "get WiFi HAL: %v\n", err)
		fmt.Fprintf(os.Stderr, "(WiFi HAL not available — no WiFi hardware or SELinux denial)\n")
		os.Exit(1)
	}

	wifiHal := wifi.NewWifiProxy(svc)

	// Check if WiFi is started.
	started, err := wifiHal.IsStarted(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "IsStarted: %v\n", err)
	} else {
		fmt.Printf("WiFi HAL started: %v\n", started)
	}

	// List available WiFi chips.
	chipIds, err := wifiHal.GetChipIds(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "GetChipIds: %v\n", err)
		return
	}

	fmt.Printf("WiFi chips: %v\n\n", chipIds)

	for _, chipId := range chipIds {
		printChipInfo(ctx, wifiHal, chipId)
	}
}

func printChipInfo(
	ctx context.Context,
	wifiHal wifi.IWifi,
	chipId int32,
) {
	fmt.Printf("=== Chip %d ===\n", chipId)

	// GetChip returns IWifiChip directly — the generated proxy
	// handles binder handle acquisition internally.
	chip, err := wifiHal.GetChip(ctx, chipId)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  GetChip(%d): %v\n", chipId, err)
		return
	}

	// Get feature set bitmask.
	features, err := chip.GetFeatureSet(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  GetFeatureSet: %v\n", err)
	} else {
		fmt.Printf("  Feature set: 0x%x\n", features)
	}

	chipId2, err := chip.GetId(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  GetId: %v\n", err)
	} else {
		fmt.Printf("  Chip ID: %d\n", chipId2)
	}

	// List AP interfaces — these are the SoftAP interfaces.
	apNames, err := chip.GetApIfaceNames(ctx)
	switch {
	case err != nil:
		fmt.Fprintf(os.Stderr, "  GetApIfaceNames: %v\n", err)
	case len(apNames) == 0:
		fmt.Printf("  AP interfaces: (none — no hotspot active)\n")
	default:
		fmt.Printf("  AP interfaces:\n")
		for _, name := range apNames {
			printApIfaceInfo(ctx, chip, name)
		}
	}

	// List STA interfaces for context.
	staNames, err := chip.GetStaIfaceNames(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  GetStaIfaceNames: %v\n", err)
	} else {
		fmt.Printf("  STA interfaces: %v\n", staNames)
	}
}

func printApIfaceInfo(
	ctx context.Context,
	chip wifi.IWifiChip,
	name string,
) {
	fmt.Printf("    - %s\n", name)

	// GetApIface returns IWifiApIface directly.
	apIface, err := chip.GetApIface(ctx, name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "      GetApIface(%s): %v\n", name, err)
		return
	}

	ifName, _ := apIface.GetName(ctx)
	fmt.Printf("      Name: %s\n", ifName)

	mac, err := apIface.GetFactoryMacAddress(ctx)
	if err == nil {
		fmt.Printf("      Factory MAC: %x\n", mac)
	}

	bridged, err := apIface.GetBridgedInstances(ctx)
	if err == nil && len(bridged) > 0 {
		fmt.Printf("      Bridged instances: %v\n", bridged)
	}

	mlo, err := apIface.UsesMlo(ctx)
	if err == nil {
		fmt.Printf("      Uses MLO: %v\n", mlo)
	}
}
