// The screen_capture example captures one scaled RGBA8888 display frame.
//
// Build and run on a Pixel 6a using the tested Android 14 r35 protocol,
// with capture permission:
//
//	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o capture ./examples/screen_capture
//	adb push capture /data/local/tmp/capture
//	adb shell su 0 /data/local/tmp/capture -android14-r35 -output /data/local/tmp/frame.rgba
//	adb exec-out su 0 cat /data/local/tmp/frame.rgba > frame.rgba
//
// The default scale of one third produces a 360 by 800 frame on a
// 1080 by 2400 display. The output is tightly packed RGBA8888. The flag
// selects the AOSP android-14.0.0_r35 wire protocol explicitly; Android
// API level alone does not identify that protocol.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/binder/versionaware"
	"github.com/AndroidGoLab/binder/kernelbinder"
	"github.com/AndroidGoLab/binder/screencapture"
	"github.com/AndroidGoLab/binder/servicemanager"
)

// run captures the first physical display and writes tightly packed pixels.
//
// Example: run with scale one third to reduce both dimensions by three.
func run(
	ctx context.Context,
	scale float32,
	output string,
) (resultErr error) {
	driver, err := kernelbinder.Open(ctx, binder.WithMapSize(4*1024*1024))
	if err != nil {
		return fmt.Errorf("open binder: %w", err)
	}
	defer // Close the driver even if capture fails.
	//
	// Example: report a close failure along with a capture failure.
	func() { resultErr = errors.Join(resultErr, driver.Close(context.Background())) }()
	transport, err := versionaware.NewTransport(ctx, driver, 0, versionaware.OptionCachePath(""), versionaware.OptionSurfaceComposerAndroid14R35())
	if err != nil {
		return fmt.Errorf("configure binder: %w", err)
	}
	remote, err := servicemanager.New(transport).GetService(ctx, "SurfaceFlingerAIDL")
	if err != nil {
		return fmt.Errorf("get SurfaceFlinger: %w", err)
	}
	capture, err := screencapture.New(remote)
	if err != nil {
		return err
	}
	defer // Stop the client before closing its transport.
	//
	// Example: a timed-out capture abandons its pending result.
	func() { resultErr = errors.Join(resultErr, capture.Close()) }()
	ids, err := capture.PhysicalDisplayIDs(ctx)
	if err != nil {
		return fmt.Errorf("list displays: %w", err)
	}
	if len(ids) == 0 {
		return errors.New("no physical displays")
	}
	frame, err := capture.CaptureDisplay(ctx, ids[0], scale)
	if err != nil {
		return fmt.Errorf("capture display: %w", err)
	}
	if err = os.WriteFile(output, frame.Pixels, 0600); err != nil {
		return fmt.Errorf("write frame: %w", err)
	}
	fmt.Printf("wrote %dx%d RGBA8888 to %s\n", frame.Width, frame.Height, output)
	return nil
}

// main parses capture options and reports any error.
//
// Example: capture -android14-r35 -scale 0.5 -output frame.rgba.
func main() {
	scale := flag.Float64("scale", 1.0/3, "display scale")
	output := flag.String("output", "frame.rgba", "raw RGBA8888 output file")
	r35 := flag.Bool("android14-r35", false, "use the android-14.0.0_r35 SurfaceFlinger protocol")
	flag.Parse()
	if !*r35 {
		fmt.Fprintln(os.Stderr, "select a supported SurfaceFlinger protocol with -android14-r35")
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := run(ctx, float32(*scale), *output); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
