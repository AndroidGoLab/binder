// Query WiFi information from the wificond system service.
//
// Uses IWificond via the "wifinl80211" service to list WiFi interfaces,
// available channels per band, and PHY capabilities.
//
// Build:
//
//	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o build/softap_wifi_hal ./examples/softap_wifi_hal/
//	adb push softap_wifi_hal /data/local/tmp/ && adb shell /data/local/tmp/softap_wifi_hal
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/AndroidGoLab/binder/android/net/wifi/nl80211"
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

	svc, err := sm.GetService(ctx, servicemanager.WifiNl80211Service)
	if err != nil {
		fmt.Fprintf(os.Stderr, "get wifinl80211 service: %v\n", err)
		fmt.Fprintf(os.Stderr, "(wificond not available or access denied by SELinux)\n")
		os.Exit(1)
	}

	wificond := nl80211.NewWificondProxy(svc)

	// List client (STA) interfaces.
	clientIfaces, err := wificond.GetClientInterfaces(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "GetClientInterfaces: %v\n", err)
	} else {
		fmt.Printf("Client interfaces: %d\n", len(clientIfaces))
		for i, iface := range clientIfaces {
			fmt.Printf("  [%d] binder handle present\n", i)
			_ = iface // IBinder handles
		}
	}

	// List AP interfaces.
	apIfaces, err := wificond.GetApInterfaces(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "GetApInterfaces: %v\n", err)
	} else {
		fmt.Printf("AP interfaces:     %d\n", len(apIfaces))
		for i, iface := range apIfaces {
			fmt.Printf("  [%d] binder handle present\n", i)
			_ = iface
		}
	}

	// Available 2.4 GHz channels.
	fmt.Println()
	ch2g, err := wificond.GetAvailable2gChannels(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "GetAvailable2gChannels: %v\n", err)
	} else {
		fmt.Printf("Available 2.4 GHz channels: %v\n", ch2g)
	}

	// Available 5 GHz non-DFS channels.
	ch5g, err := wificond.GetAvailable5gNonDFSChannels(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "GetAvailable5gNonDFSChannels: %v\n", err)
	} else {
		fmt.Printf("Available 5 GHz (non-DFS) channels: %v\n", ch5g)
	}

	// Available DFS channels.
	chDFS, err := wificond.GetAvailableDFSChannels(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "GetAvailableDFSChannels: %v\n", err)
	} else {
		fmt.Printf("Available DFS channels: %v\n", chDFS)
	}

	// Available 6 GHz channels.
	ch6g, err := wificond.GetAvailable6gChannels(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "GetAvailable6gChannels: %v\n", err)
	} else {
		fmt.Printf("Available 6 GHz channels: %v\n", ch6g)
	}

	// Available 60 GHz channels.
	ch60g, err := wificond.GetAvailable60gChannels(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "GetAvailable60gChannels: %v\n", err)
	} else {
		fmt.Printf("Available 60 GHz channels: %v\n", ch60g)
	}

	// PHY capabilities for common interface names.
	fmt.Println()
	for _, ifName := range []string{"wlan0", "wlan1", "wlan2"} {
		caps, err := wificond.GetDeviceWiphyCapabilities(ctx, ifName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "GetDeviceWiphyCapabilities(%s): %v\n", ifName, err)
			continue
		}
		fmt.Printf("PHY capabilities for %s: %+v\n", ifName, caps)
	}
}
