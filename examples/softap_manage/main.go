// Manage SoftAP (WiFi hotspot) via the hostapd HAL.
//
// This example demonstrates how to start and stop a WiFi access point,
// force-disconnect clients, and configure AP parameters using the
// android.hardware.wifi.hostapd HAL interface.
//
// Note: The hostapd HAL is only available on devices with real WiFi hardware.
// It may be blocked by SELinux for unprivileged binaries. On a real device,
// run as root or with the appropriate SELinux context.
//
// Build:
//
//	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o softap_manage ./examples/softap_manage/
//	adb push softap_manage /data/local/tmp/ && adb shell /data/local/tmp/softap_manage
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/xaionaro-go/aidl/binder"
	"github.com/xaionaro-go/aidl/android/hardware/wifi/hostapd"
	"github.com/xaionaro-go/aidl/kernelbinder"
	"github.com/xaionaro-go/aidl/servicemanager"
)

const halServiceName = "android.hardware.wifi.hostapd.IHostapd/default"

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <command> [args...]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nCommands:\n")
		fmt.Fprintf(os.Stderr, "  start <iface> <ssid> <passphrase>  Start AP on interface\n")
		fmt.Fprintf(os.Stderr, "  stop <iface>                       Stop AP on interface\n")
		fmt.Fprintf(os.Stderr, "  kick <iface> <mac_hex>             Disconnect a client\n")
		fmt.Fprintf(os.Stderr, "  terminate                          Terminate hostapd daemon\n")
		fmt.Fprintf(os.Stderr, "\nExample:\n")
		fmt.Fprintf(os.Stderr, "  %s start wlan1 MyHotspot s3cretP4ss\n", os.Args[0])
		os.Exit(1)
	}

	ctx := context.Background()

	driver, err := kernelbinder.Open(ctx, binder.WithMapSize(128*1024))
	if err != nil {
		fmt.Fprintf(os.Stderr, "open binder: %v\n", err)
		os.Exit(1)
	}
	defer driver.Close(ctx)

	sm := servicemanager.New(driver)

	svc, err := sm.GetService(ctx, halServiceName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "get hostapd HAL: %v\n", err)
		fmt.Fprintf(os.Stderr, "(hostapd HAL not available — no WiFi hardware or SELinux denial)\n")
		os.Exit(1)
	}

	ap := hostapd.NewHostapdProxy(svc)

	switch os.Args[1] {
	case "start":
		cmdStart(ctx, ap)
	case "stop":
		cmdStop(ctx, ap)
	case "kick":
		cmdKick(ctx, ap)
	case "terminate":
		cmdTerminate(ctx, ap)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}

func cmdStart(
	ctx context.Context,
	ap *hostapd.HostapdProxy,
) {
	if len(os.Args) < 5 {
		fmt.Fprintf(os.Stderr, "usage: start <iface> <ssid> <passphrase>\n")
		os.Exit(1)
	}

	ifaceName := os.Args[2]
	ssid := os.Args[3]
	passphrase := os.Args[4]

	ifaceParams := hostapd.IfaceParams{
		Name: ifaceName,
		HwModeParams: hostapd.HwModeParams{
			Enable80211N:  true,
			Enable80211AC: true,
			Enable80211AX: true,
		},
		ChannelParams: []hostapd.ChannelParams{
			{
				BandMask:  hostapd.BandMaskBand2Ghz,
				EnableAcs: true, // Let the driver pick the best channel.
			},
		},
	}

	nwParams := hostapd.NetworkParams{
		Ssid:           []byte(ssid),
		IsHidden:       false,
		EncryptionType: hostapd.EncryptionTypeWpa3SaeTransition,
		Passphrase:     passphrase,
	}

	err := ap.AddAccessPoint(ctx, ifaceParams, nwParams)
	if err != nil {
		fmt.Fprintf(os.Stderr, "AddAccessPoint: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Access point started on %s\n", ifaceName)
	fmt.Printf("  SSID:       %s\n", ssid)
	fmt.Printf("  Security:   WPA3-SAE Transition\n")
	fmt.Printf("  Band:       2.4 GHz (ACS)\n")
}

func cmdStop(
	ctx context.Context,
	ap *hostapd.HostapdProxy,
) {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "usage: stop <iface>\n")
		os.Exit(1)
	}

	ifaceName := os.Args[2]
	err := ap.RemoveAccessPoint(ctx, ifaceName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "RemoveAccessPoint: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Access point stopped on %s\n", ifaceName)
}

func cmdKick(
	ctx context.Context,
	ap *hostapd.HostapdProxy,
) {
	if len(os.Args) < 4 {
		fmt.Fprintf(os.Stderr, "usage: kick <iface> <mac_hex>\n")
		fmt.Fprintf(os.Stderr, "  mac_hex: 6-byte MAC as hex, e.g. aabbccddeeff\n")
		os.Exit(1)
	}

	ifaceName := os.Args[2]
	macHex := os.Args[3]

	mac, err := hexToBytes(macHex)
	if err != nil || len(mac) != 6 {
		fmt.Fprintf(os.Stderr, "invalid MAC address: %s (need 12 hex chars)\n", macHex)
		os.Exit(1)
	}

	err = ap.ForceClientDisconnect(
		ctx,
		ifaceName,
		mac,
		hostapd.Ieee80211ReasonCodeWlanReasonDisassocApBusy,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ForceClientDisconnect: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Disconnected client %x from %s\n", mac, ifaceName)
}

func cmdTerminate(
	ctx context.Context,
	ap *hostapd.HostapdProxy,
) {
	err := ap.Terminate(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Terminate: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("hostapd daemon terminated")
}

func hexToBytes(s string) ([]byte, error) {
	if len(s)%2 != 0 {
		return nil, fmt.Errorf("odd length hex string")
	}
	b := make([]byte, len(s)/2)
	for i := range b {
		_, err := fmt.Sscanf(s[2*i:2*i+2], "%02x", &b[i])
		if err != nil {
			return nil, err
		}
	}
	return b, nil
}
