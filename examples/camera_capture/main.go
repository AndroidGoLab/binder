// Camera frame capture using gralloc-allocated buffers.
//
// This example connects to the camera via binder, allocates real gralloc
// buffers through IAllocator, captures frames, and verifies the pixel data.
//
// Build:
//
//	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o build/camera_capture ./examples/camera_capture/
//	adb push build/camera_capture /data/local/tmp/ && adb shell /data/local/tmp/camera_capture
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/binder/versionaware"
	"github.com/AndroidGoLab/binder/camera"
	"github.com/AndroidGoLab/binder/kernelbinder"
	"github.com/AndroidGoLab/binder/servicemanager"
)

func run(ctx context.Context) error {
	const (
		cameraID = "0"
		width    = 640
		height   = 480
	)

	drv, err := kernelbinder.Open(ctx, binder.WithMapSize(4*1024*1024))
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer drv.Close(ctx)

	transport, err := versionaware.NewTransport(ctx, drv, 0)
	if err != nil {
		return fmt.Errorf("transport: %w", err)
	}
	sm := servicemanager.New(transport)

	cam, err := camera.Connect(ctx, sm, transport, cameraID)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer cam.Close(ctx)

	if err := cam.ConfigureStream(ctx, width, height, camera.FormatYCbCr420); err != nil {
		return fmt.Errorf("configure stream: %w", err)
	}

	captureCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	frame, err := cam.CaptureFrame(captureCtx)
	if err != nil {
		return fmt.Errorf("capture frame: %w", err)
	}

	nonZero := 0
	for _, b := range frame {
		if b != 0 {
			nonZero++
		}
	}

	fmt.Printf("Captured %dx%d frame, %d bytes, %.1f%% non-zero pixels\n",
		width, height, len(frame), float64(nonZero)/float64(len(frame))*100)

	return nil
}

func main() {
	ctx := context.Background()
	if err := run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: %v\n", err)
		os.Exit(1)
	}
}
