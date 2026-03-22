// Test camera ConnectDevice with a local callback stub.
//
// This example demonstrates the server-side binder integration:
// a local ICameraDeviceCallback stub is created, passed to
// ConnectDevice, and the binder driver handles it as a local
// binder object (BINDER_TYPE_BINDER).
//
// Build:
//
//	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o build/camera_connect ./examples/camera_connect/
//	adb push build/camera_connect /data/local/tmp/ && adb shell /data/local/tmp/camera_connect
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/xaionaro-go/binder/android/frameworks/cameraservice/device"
	"github.com/xaionaro-go/binder/android/frameworks/cameraservice/service"
	"github.com/xaionaro-go/binder/binder"
	"github.com/xaionaro-go/binder/binder/versionaware"
	"github.com/xaionaro-go/binder/kernelbinder"
	"github.com/xaionaro-go/binder/servicemanager"
)

// noopCallback is a no-op ICameraDeviceCallbackServer implementation.
type noopCallback struct{}

func (c *noopCallback) OnCaptureStarted(
	_ context.Context,
	_ device.CaptureResultExtras,
	_ int64,
) error {
	return nil
}

func (c *noopCallback) OnDeviceError(
	_ context.Context,
	_ device.ErrorCode,
	_ device.CaptureResultExtras,
) error {
	return nil
}

func (c *noopCallback) OnDeviceIdle(_ context.Context) error {
	return nil
}

func (c *noopCallback) OnPrepared(_ context.Context, _ int32) error {
	return nil
}

func (c *noopCallback) OnRepeatingRequestError(
	_ context.Context,
	_ int64,
	_ int32,
) error {
	return nil
}

func (c *noopCallback) OnResultReceived(
	_ context.Context,
	_ device.CaptureMetadataInfo,
	_ device.CaptureResultExtras,
	_ []device.PhysicalCaptureResultInfo,
) error {
	return nil
}

func (c *noopCallback) OnClientSharedAccessPriorityChanged(
	_ context.Context,
	_ bool,
) error {
	return nil
}

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

	// Look up the camera service.
	cameraSvc, err := sm.GetService(ctx, "android.frameworks.cameraservice.service.ICameraService/default")
	if err != nil {
		fmt.Fprintf(os.Stderr, "get camera service: %v\n", err)
		os.Exit(1)
	}

	cameraProxy := service.NewCameraServiceProxy(cameraSvc)

	// Create a local callback stub.
	callback := device.NewCameraDeviceCallbackStub(&noopCallback{})
	fmt.Println("Created callback stub, calling ConnectDevice...")

	// Try to connect. This will likely fail with a permission error,
	// but the important thing is that the binder plumbing works
	// (the stub is written as BINDER_TYPE_BINDER, not BINDER_TYPE_HANDLE).
	deviceUser, err := cameraProxy.ConnectDevice(ctx, callback, "0")
	if err != nil {
		fmt.Fprintf(os.Stderr, "ConnectDevice error (expected): %v\n", err)
		// Let deferred drv.Close run for clean shutdown.
		return
	}

	fmt.Printf("ConnectDevice succeeded: %v\n", deviceUser)

	// Disconnect the camera device before closing the binder driver.
	// This tells the camera service to release the callback, so the
	// binder driver's read loop is not stuck waiting for transactions.
	if err := deviceUser.Disconnect(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Disconnect: %v\n", err)
	}
}
