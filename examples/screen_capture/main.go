// The screen_capture example captures one scaled RGBA8888 display frame.
//
// Build and run on a Pixel 6a using the tested Android 14 r35 protocol,
// with capture permission:
//
//	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o capture ./examples/screen_capture
//	adb push capture /data/local/tmp/capture
//	adb shell su 0 /data/local/tmp/capture -output /data/local/tmp/frame.rgba
//	adb exec-out su 0 cat /data/local/tmp/frame.rgba > frame.rgba
//
// The default scale of one third produces a 360 by 800 frame on a
// 1080 by 2400 display. The output is tightly packed RGBA8888. The library
// detects the supported native wire protocol and reports unsupported devices.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"time"

	"github.com/AndroidGoLab/binder/android/gui"
	gfxCommon "github.com/AndroidGoLab/binder/android/hardware/graphics/common"
	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/binder/versionaware"
	"github.com/AndroidGoLab/binder/kernelbinder"
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
	if scale <= 0 || math.IsNaN(float64(scale)) || math.IsInf(float64(scale), 0) {
		return fmt.Errorf("invalid display scale %v", scale)
	}
	driver, err := kernelbinder.Open(ctx, binder.WithMapSize(4*1024*1024))
	if err != nil {
		return fmt.Errorf("open binder: %w", err)
	}
	defer // Close the driver even if capture fails.
	//
	// Example: report a close failure along with a capture failure.
	func() { resultErr = errors.Join(resultErr, driver.Close(context.Background())) }()
	transport, err := versionaware.NewTransport(ctx, driver, 0, versionaware.OptionCachePath(""))
	if err != nil {
		return fmt.Errorf("configure binder: %w", err)
	}
	remote, err := servicemanager.New(transport).GetService(ctx, "SurfaceFlingerAIDL")
	if err != nil {
		return fmt.Errorf("get SurfaceFlinger: %w", err)
	}
	if _, err := remote.ResolveCode(ctx, gui.DescriptorISurfaceComposer, gui.MethodISurfaceComposerCaptureDisplayById); err != nil {
		return fmt.Errorf("resolve display capture: %w", err)
	}
	composer := gui.NewSurfaceComposerProxy(remote)
	ids, err := composer.GetPhysicalDisplayIds(ctx)
	if err != nil {
		return fmt.Errorf("list displays: %w", err)
	}
	if len(ids) == 0 {
		return errors.New("no physical displays")
	}
	args := gui.CaptureArgs{PixelFormat: int32(gfxCommon.PixelFormatRgba8888), FrameScaleX: scale, FrameScaleY: scale, Uid: -1}
	frame, err := composer.CaptureDisplayByIdSync(ctx, ids[0], args)
	if err != nil {
		return fmt.Errorf("capture display: %w", err)
	}
	defer // Release the result's native descriptors after copying its pixels.
	//
	// Example: preserve cleanup errors after a successful capture.
	func() { resultErr = errors.Join(resultErr, frame.Close()) }()
	pixels, err := frame.CopyRGBA(ctx)
	if err != nil {
		return fmt.Errorf("copy display pixels: %w", err)
	}
	if err = os.WriteFile(output, pixels, 0600); err != nil {
		return fmt.Errorf("write frame: %w", err)
	}
	fmt.Printf("wrote %dx%d RGBA8888 to %s\n", frame.Buffer.Width, frame.Buffer.Height, output)
	return nil
}

// main parses capture options and reports any error.
//
// Example: capture -scale 0.5 -output frame.rgba.
func main() {
	scale := flag.Float64("scale", 1.0/3, "display scale")
	output := flag.String("output", "frame.rgba", "raw RGBA8888 output file")
	flag.Parse()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := run(ctx, float32(*scale), *output); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
