// Query tethering and network interface status via INetworkManagementService.
//
// Uses the "network_management" system service to list network interfaces,
// check tethering status, and query interface configurations.
// Replaces the previous vendor offload HAL approach with system-level APIs.
//
// Note: Some tethering methods were removed in Android API 36+.
//
// Build:
//
//	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o build/softap_tether_offload ./examples/softap_tether_offload/
//	adb push softap_tether_offload /data/local/tmp/ && adb shell /data/local/tmp/softap_tether_offload
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
		os.Exit(1)
	}

	netMgr := genOs.NewNetworkManagementServiceProxy(svc)

	// List all network interfaces.
	ifaces, err := netMgr.ListInterfaces(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ListInterfaces: %v\n", err)
	} else {
		fmt.Printf("Network interfaces (%d):\n", len(ifaces))
		for _, iface := range ifaces {
			fmt.Printf("  %s\n", iface)
		}
	}

	// Check tethering status (may be removed in API 36+).
	fmt.Println()
	tethering, err := netMgr.IsTetheringStarted(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "IsTetheringStarted: %v\n", err)
		fmt.Fprintf(os.Stderr, "  (this method was removed in Android API 36)\n")
	} else {
		fmt.Printf("Tethering active: %v\n", tethering)
	}

	// List tethered interfaces (may be removed in API 36+).
	tethered, err := netMgr.ListTetheredInterfaces(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ListTetheredInterfaces: %v\n", err)
		fmt.Fprintf(os.Stderr, "  (this method was removed in Android API 36)\n")
	} else if len(tethered) == 0 {
		fmt.Println("Tethered interfaces: (none)")
	} else {
		fmt.Printf("Tethered interfaces: %v\n", tethered)
	}

	// Check bandwidth control.
	fmt.Println()
	bwCtrl, err := netMgr.IsBandwidthControlEnabled(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "IsBandwidthControlEnabled: %v\n", err)
	} else {
		fmt.Printf("Bandwidth control enabled: %v\n", bwCtrl)
	}

	// Check IP forwarding (may be removed in API 36+).
	fwd, err := netMgr.GetIpForwardingEnabled(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "GetIpForwardingEnabled: %v\n", err)
	} else {
		fmt.Printf("IP forwarding enabled: %v\n", fwd)
	}

	// Try to read network statistics from procfs as a complement.
	fmt.Println()
	printNetStats(ifaces)
}

// printNetStats reads basic traffic counters from /proc/net/dev for the
// given interfaces.
func printNetStats(ifaces []string) {
	data, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		fmt.Fprintf(os.Stderr, "read /proc/net/dev: %v\n", err)
		return
	}

	// Build a set of interfaces we care about.
	ifaceSet := make(map[string]bool, len(ifaces))
	for _, iface := range ifaces {
		ifaceSet[iface] = true
	}

	fmt.Println("Network traffic statistics:")
	fmt.Printf("  %-15s %15s %15s\n", "Interface", "RX bytes", "TX bytes")
	fmt.Printf("  %-15s %15s %15s\n", "---------", "--------", "--------")

	lines := splitLines(string(data))
	for _, line := range lines[2:] { // skip header lines
		var iface string
		var rxBytes, txBytes int64
		var dummy int64
		// Format: iface: rx_bytes rx_packets ... tx_bytes tx_packets ...
		n, _ := fmt.Sscanf(line, " %s %d %d %d %d %d %d %d %d %d",
			&iface, &rxBytes, &dummy, &dummy, &dummy, &dummy, &dummy, &dummy, &dummy, &txBytes)
		if n < 10 {
			continue
		}
		// Remove trailing colon from iface name.
		if len(iface) > 0 && iface[len(iface)-1] == ':' {
			iface = iface[:len(iface)-1]
		}
		if !ifaceSet[iface] {
			continue
		}
		if rxBytes == 0 && txBytes == 0 {
			continue
		}
		fmt.Printf("  %-15s %15d %15d\n", iface, rxBytes, txBytes)
	}
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
