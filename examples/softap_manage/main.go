// Query network management status via the INetworkManagementService system service.
//
// Uses INetworkManagementService via the "network_management" service to
// list network interfaces, check tethering and firewall/bandwidth status.
//
// Note: Some methods require root or AID_SYSTEM, and some tethering methods
// were removed in Android API 36+.
//
// Build:
//
//	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o build/softap_manage ./examples/softap_manage/
//	adb push softap_manage /data/local/tmp/ && adb shell /data/local/tmp/softap_manage
package main

import (
	"context"
	"fmt"
	"os"

	genOs "github.com/AndroidGoLab/binder/android/os"
	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/binder/versionaware"
	"github.com/AndroidGoLab/binder/kernelbinder"
	"github.com/AndroidGoLab/binder/servicemanager"
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

	svc, err := sm.GetService(ctx, servicemanager.NetworkmanagementService)
	if err != nil {
		fmt.Fprintf(os.Stderr, "get network_management service: %v\n", err)
		fmt.Fprintf(os.Stderr, "(INetworkManagementService not available or access denied)\n")
		os.Exit(1)
	}

	netMgr := genOs.NewNetworkManagementServiceProxy(svc)

	// Check tethering status (may be absent on API 36+).
	tethering, err := netMgr.IsTetheringStarted(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "IsTetheringStarted: %v\n", err)
		fmt.Fprintf(os.Stderr, "  (this method was removed in Android API 36)\n")
	} else {
		fmt.Printf("Tethering active: %v\n", tethering)
	}

	// List tethered interfaces (may be absent on API 36+).
	tethered, err := netMgr.ListTetheredInterfaces(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ListTetheredInterfaces: %v\n", err)
		fmt.Fprintf(os.Stderr, "  (this method was removed in Android API 36)\n")
	} else if len(tethered) == 0 {
		fmt.Println("Tethered interfaces: (none)")
	} else {
		fmt.Printf("Tethered interfaces: %v\n", tethered)
	}

	// List all network interfaces.
	ifaces, err := netMgr.ListInterfaces(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ListInterfaces: %v\n", err)
	} else {
		fmt.Printf("\nNetwork interfaces (%d):\n", len(ifaces))
		for _, iface := range ifaces {
			fmt.Printf("  %s\n", iface)
		}
	}

	// Check bandwidth control.
	fmt.Println()
	bwCtrl, err := netMgr.IsBandwidthControlEnabled(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "IsBandwidthControlEnabled: %v\n", err)
	} else {
		fmt.Printf("Bandwidth control enabled: %v\n", bwCtrl)
	}

	// Check firewall status (requires AID_SYSTEM).
	fwEnabled, err := netMgr.IsFirewallEnabled(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "IsFirewallEnabled: %v (requires AID_SYSTEM)\n", err)
	} else {
		fmt.Printf("Firewall enabled: %v\n", fwEnabled)
	}
}
