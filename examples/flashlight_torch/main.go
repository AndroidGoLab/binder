// Toggle the Android flashlight/torch on and off via the camera service binder.
//
// Uses ICameraService.SetTorchMode to control the torch for camera "0".
// By default, turns the torch ON for 3 seconds, then turns it OFF.
// Pass "on" or "off" as a command-line argument to set a specific state.
//
// Build:
//
//	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o build/flashlight_torch ./examples/flashlight_torch/
//	adb push build/flashlight_torch /data/local/tmp/ && adb shell /data/local/tmp/flashlight_torch
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/AndroidGoLab/binder/android/hardware"
	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/binder/versionaware"
	"github.com/AndroidGoLab/binder/kernelbinder"
	"github.com/AndroidGoLab/binder/servicemanager"
)

func main() {
	ctx := context.Background()

	drv, err := kernelbinder.Open(ctx, binder.WithMapSize(128*1024))
	if err != nil {
		fmt.Fprintf(os.Stderr, "open binder: %v\n", err)
		os.Exit(1)
	}
	defer drv.Close(ctx)

	transport, err := versionaware.NewTransport(ctx, drv, 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "version-aware transport: %v\n", err)
		os.Exit(1)
	}

	sm := servicemanager.New(transport)

	svc, err := sm.GetService(ctx, "media.camera")
	if err != nil {
		fmt.Fprintf(os.Stderr, "get media.camera service: %v\n", err)
		os.Exit(1)
	}

	cam := hardware.NewCameraServiceProxy(svc)

	// Report the number of available cameras.
	numCameras, err := cam.GetNumberOfCameras(ctx, hardware.ICameraServiceCameraTypeBackwardCompatible)
	if err != nil {
		fmt.Fprintf(os.Stderr, "GetNumberOfCameras: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Number of cameras: %d\n", numCameras)

	// Determine the desired action from command-line arguments.
	action := "toggle" // default: on, wait, off
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "on":
			action = "on"
		case "off":
			action = "off"
		default:
			fmt.Fprintf(os.Stderr, "usage: %s [on|off]\n", os.Args[0])
			os.Exit(1)
		}
	}

	const cameraID = "0"

	switch action {
	case "on":
		fmt.Printf("Turning torch ON for camera %s\n", cameraID)
		if err := cam.SetTorchMode(ctx, cameraID, true, nil); err != nil {
			fmt.Fprintf(os.Stderr, "SetTorchMode(on): %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Torch is ON.")

	case "off":
		fmt.Printf("Turning torch OFF for camera %s\n", cameraID)
		if err := cam.SetTorchMode(ctx, cameraID, false, nil); err != nil {
			fmt.Fprintf(os.Stderr, "SetTorchMode(off): %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Torch is OFF.")

	default: // toggle
		fmt.Printf("Turning torch ON for camera %s\n", cameraID)
		if err := cam.SetTorchMode(ctx, cameraID, true, nil); err != nil {
			fmt.Fprintf(os.Stderr, "SetTorchMode(on): %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Torch is ON. Waiting 3 seconds...")

		time.Sleep(3 * time.Second)

		fmt.Printf("Turning torch OFF for camera %s\n", cameraID)
		if err := cam.SetTorchMode(ctx, cameraID, false, nil); err != nil {
			fmt.Fprintf(os.Stderr, "SetTorchMode(off): %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Torch is OFF.")
	}
}
