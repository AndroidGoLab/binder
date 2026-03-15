// Query tethering offload statistics from the hardware offload HAL.
//
// When SoftAP is active, the tethering offload HAL accelerates packet
// forwarding in hardware. This example shows how to query forwarded
// traffic statistics for each upstream interface.
//
// Build:
//
//	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o build/softap_tether_offload ./examples/softap_tether_offload/
//	adb push softap_tether_offload /data/local/tmp/ && adb shell /data/local/tmp/softap_tether_offload
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/xaionaro-go/binder/binder"
	"github.com/xaionaro-go/binder/binder/versionaware"
	genOs "github.com/xaionaro-go/binder/android/os"
	"github.com/xaionaro-go/binder/android/hardware/tetheroffload"
	"github.com/xaionaro-go/binder/kernelbinder"
	"github.com/xaionaro-go/binder/servicemanager"
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

	// First list network interfaces to know which upstreams exist.
	netSvc, err := sm.GetService(ctx, servicemanager.ServiceName("network_management"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "get network_management: %v\n", err)
	} else {
		netMgr := genOs.NewNetworkManagementServiceProxy(netSvc)

		ifaces, err := netMgr.ListInterfaces(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ListInterfaces: %v\n", err)
		} else {
			fmt.Printf("Network interfaces: %v\n\n", ifaces)
		}
	}

	// Try to access the tethering offload HAL.
	offloadSvc, err := sm.GetService(ctx, servicemanager.ServiceName("android.hardware.tetheroffload.IOffload/default"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "get tether offload HAL: %v\n", err)
		fmt.Fprintf(os.Stderr, "(offload HAL not available — no hardware offload support or SELinux denial)\n")
		fmt.Fprintf(os.Stderr, "\nOn devices with offload support, this queries forwarded traffic stats:\n")
		fmt.Fprintf(os.Stderr, "  offload.GetForwardedStats(ctx, \"wlan0\") → {RxBytes, TxBytes}\n")
		os.Exit(0)
	}

	offload := tetheroffload.NewOffloadProxy(offloadSvc)

	// Query forwarded stats for common upstream interfaces.
	upstreams := []string{"wlan0", "eth0", "rmnet0", "rmnet_data0"}

	fmt.Println("Tethering offload forwarded traffic:")
	for _, iface := range upstreams {
		stats, err := offload.GetForwardedStats(ctx, iface)
		if err != nil {
			continue
		}
		fmt.Printf("  %-15s RX: %d bytes  TX: %d bytes\n",
			iface, stats.RxBytes, stats.TxBytes)
	}
}
