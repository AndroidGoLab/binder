// Query display configuration: IDs, brightness, night mode, color mode.
//
// Build:
//
//	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o build/display_info ./examples/display_info/
//	adb push display_info /data/local/tmp/ && adb shell /data/local/tmp/display_info
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/binder/versionaware"
	"github.com/AndroidGoLab/binder/android/hardware/display"
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

	// Display Manager — display IDs and brightness
	svc, err := sm.GetService(ctx, servicemanager.DisplayService)
	if err != nil {
		fmt.Fprintf(os.Stderr, "get display service: %v\n", err)
		os.Exit(1)
	}

	dm := display.NewDisplayManagerProxy(svc)

	ids, err := dm.GetDisplayIds(ctx, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "GetDisplayIds: %v\n", err)
	} else {
		fmt.Printf("Display IDs: %v\n", ids)
		for _, id := range ids {
			brightness, err := dm.GetBrightness(ctx, id)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  Display %d brightness: %v\n", id, err)
			} else {
				fmt.Printf("  Display %d brightness: %.2f\n", id, brightness)
			}
		}
	}

	// Color Display Manager — night mode and color settings
	colorSvc, err := sm.GetService(ctx, servicemanager.ColorDisplayService)
	if err != nil {
		fmt.Fprintf(os.Stderr, "get color_display service: %v\n", err)
		return
	}

	cdm := display.NewColorDisplayManagerProxy(colorSvc)

	nightMode, err := cdm.IsNightDisplayActivated(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "IsNightDisplayActivated: %v\n", err)
	} else {
		fmt.Printf("\nNight mode active: %v\n", nightMode)
	}

	colorMode, err := cdm.GetColorMode(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "GetColorMode: %v\n", err)
	} else {
		fmt.Printf("Color mode:        %d\n", colorMode)
	}

	managed, err := cdm.IsDeviceColorManaged(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "IsDeviceColorManaged: %v\n", err)
	} else {
		fmt.Printf("Color managed:     %v\n", managed)
	}

	nightTemp, err := cdm.GetNightDisplayColorTemperature(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "GetNightDisplayColorTemperature: %v\n", err)
	} else {
		fmt.Printf("Night color temp:  %d K\n", nightTemp)
	}
}
