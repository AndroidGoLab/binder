// Query storage and USB state: last fstrim, USB functions, USB speed.
//
// Build:
//
//	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o build/storage_info ./examples/storage_info/
//	adb push storage_info /data/local/tmp/ && adb shell /data/local/tmp/storage_info
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/binder/versionaware"
	"github.com/AndroidGoLab/binder/android/hardware/usb"
	"github.com/AndroidGoLab/binder/android/os/storage"
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

	// Storage Manager
	mountSvc, err := sm.GetService(ctx, servicemanager.ServiceName("mount"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "get mount service: %v\n", err)
	} else {
		store := storage.NewStorageManagerProxy(mountSvc)

		lastMaint, err := store.LastMaintenance(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "LastMaintenance: %v\n", err)
		} else {
			t := time.UnixMilli(lastMaint)
			fmt.Printf("Last fstrim:       %s (%s ago)\n", t.Format(time.RFC3339), time.Since(t).Round(time.Second))
		}
	}

	// USB Manager
	usbMgr, err := usb.GetUsbManager(ctx, sm)
	if err != nil {
		fmt.Fprintf(os.Stderr, "get usb service: %v\n", err)
	} else {
		funcs, err := usbMgr.GetCurrentFunctions(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "GetCurrentFunctions: %v\n", err)
		} else {
			fmt.Printf("USB functions:     0x%x", funcs)
			var names []string
			if funcs&1 != 0 {
				names = append(names, "MTP")
			}
			if funcs&4 != 0 {
				names = append(names, "PTP")
			}
			if funcs&8 != 0 {
				names = append(names, "MIDI")
			}
			if funcs&32 != 0 {
				names = append(names, "ACCESSORY")
			}
			if funcs&64 != 0 {
				names = append(names, "AUDIO_SOURCE")
			}
			if funcs&512 != 0 {
				names = append(names, "NCM")
			}
			if len(names) > 0 {
				fmt.Printf(" (%v)", names)
			}
			fmt.Println()
		}

		speed, err := usbMgr.GetCurrentUsbSpeed(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "GetCurrentUsbSpeed: %v\n", err)
		} else {
			fmt.Printf("USB speed:         %d\n", speed)
		}
	}
}
