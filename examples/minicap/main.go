// The minicap example streams fixed-size RGBA8888 frames from a virtual display.
//
// Build with GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build ./examples/minicap.
// On a supported device, run as a user with SurfaceFlinger capture permission:
// minicap -source-width 1080 -source-height 2400 -width 360 -height 800 > frames.rgba
// Each frame contains width*height*4 bytes with no padding. Android 14 Pixel
// gchips and Android 10 Qualcomm SDM845 are the supported allocator layouts.
// The Android 10 path has source-based tests but has not been tested on a device;
// unsupported handles are rejected.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/AndroidGoLab/binder/android/gui"
	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/binder/versionaware"
	"github.com/AndroidGoLab/binder/gralloc"
	"github.com/AndroidGoLab/binder/igbp"
	"github.com/AndroidGoLab/binder/kernelbinder"
	"github.com/AndroidGoLab/binder/servicemanager"
)

// writeFrame writes one complete frame or reports a short write.
//
// Example: append a tightly packed frame to a stream.
func writeFrame(w io.Writer, pixels []byte) error {
	n, err := w.Write(pixels)
	if err != nil {
		return err
	}
	if n != len(pixels) {
		return io.ErrShortWrite
	}
	return nil
}

// frameReleaser releases an acquired buffer after its fence signals.
//
// Example: stop waiting when cleanup times out.
type frameReleaser interface{ Release(context.Context) error }

// releaseFrame waits for a frame release within the caller’s cleanup deadline.
//
// Example: report an unsignaled fence when the context expires.
func releaseFrame(ctx context.Context, frame frameReleaser) error { return frame.Release(ctx) }

// cleanupTimeout bounds waits for compositor and fence cleanup.
//
// Example: return control even if a graphics fence never signals.
const cleanupTimeout = 5 * time.Second

// capture streams frames until canceled or an operation fails.
//
// Example: stream a scaled display to standard output.
func capture(ctx context.Context, out io.Writer, sourceWidth, sourceHeight, width, height int32) (resultErr error) {
	driver, err := kernelbinder.Open(ctx, binder.WithMapSize(4*1024*1024))
	if err != nil {
		return fmt.Errorf("open binder: %w", err)
	}
	var stub *binder.StubBinder
	defer func() {
		resultErr = errors.Join(resultErr, driver.Close(context.Background()))
		if stub != nil {
			resultErr = errors.Join(resultErr, stub.Release(context.Background()))
		}
	}()
	transport, err := versionaware.NewTransport(ctx, driver, 0, versionaware.OptionCachePath(""))
	if err != nil {
		return fmt.Errorf("configure binder: %w", err)
	}
	api := transport.APILevel()
	if api == 34 {
		if err := transport.CheckSurfaceComposerAndroid14R35(); err != nil {
			return err
		}
	}
	sm := servicemanager.New(transport)
	raw, err := sm.GetService(ctx, "SurfaceFlinger")
	if err != nil {
		return fmt.Errorf("get SurfaceFlinger: %w", err)
	}
	var aidl binder.IBinder
	if api == 34 {
		aidl, err = sm.GetService(ctx, "SurfaceFlingerAIDL")
		if err != nil {
			return fmt.Errorf("get SurfaceFlinger AIDL: %w", err)
		}
	}
	buffers, err := gralloc.AllocateRGBA(ctx, sm, api, width, height, 4)
	if err != nil {
		return fmt.Errorf("allocate capture buffers: %w", err)
	}
	safeToFree := true
	defer func() {
		if safeToFree {
			resultErr = errors.Join(resultErr, gralloc.CloseBuffers(buffers))
		}
	}()
	producer, err := igbp.NewFrameProducer(api, buffers)
	if err != nil {
		return err
	}
	stub = binder.NewStubBinder(producer)
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
		defer cancel()
		err := producer.Close(cleanupCtx)
		if err != nil {
			safeToFree = false
		}
		resultErr = errors.Join(resultErr, err)
	}()
	display, err := gui.CreateVirtualDisplay(ctx, raw, aidl, transport, api, stub, sourceWidth, sourceHeight, width, height)
	if err != nil {
		safeToFree = false
		return err
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
		defer cancel()
		err := display.Close(cleanupCtx)
		if err != nil {
			safeToFree = false
		}
		resultErr = errors.Join(resultErr, err)
	}()
	for {
		frame, err := producer.Next(ctx)
		if errors.Is(err, context.Canceled) {
			return nil
		}
		if err != nil {
			return err
		}
		pixels, copyErr := frame.CopyRGBA(ctx)
		cleanupCtx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
		releaseErr := releaseFrame(cleanupCtx, frame)
		cancel()
		if releaseErr != nil {
			safeToFree = false
		}
		if errors.Is(copyErr, context.Canceled) && releaseErr == nil {
			return nil
		}
		if err := errors.Join(copyErr, releaseErr); err != nil {
			return err
		}
		if err := writeFrame(out, pixels); err != nil {
			return fmt.Errorf("write frame: %w", err)
		}
	}
}

// main parses display dimensions and streams frames until interrupted.
//
// Example: pipe the raw frames to a compatible consumer.
func main() {
	sw := flag.Int("source-width", 1080, "physical display width")
	sh := flag.Int("source-height", 2400, "physical display height")
	w := flag.Int("width", 360, "output width")
	h := flag.Int("height", 800, "output height")
	flag.Parse()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := capture(ctx, os.Stdout, int32(*sw), int32(*sh), int32(*w), int32(*h)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
