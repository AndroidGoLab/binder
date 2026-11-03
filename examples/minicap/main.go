// The minicap example streams JPEG or raw RGBA8888 frames from a virtual display.
//
// Build with GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build ./examples/minicap.
// On a supported device, run as a user with SurfaceFlinger capture permission:
// minicap -P 1080x2400@360x800/0 -n minicap
// Use -raw to write tightly packed RGBA frames to stdout.
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

// captureBufferCount bounds the producer queue and each frame-skip pass.
//
// Example: a fast producer cannot keep the consumer in a skip loop forever.
const captureBufferCount = 4

// frameDelay returns the remaining interval after frame processing.
//
// Example: wait only the unused part of a 10 FPS interval.
func frameDelay(start, now time.Time, rate float64) time.Duration {
	remaining := start.Add(time.Duration(float64(time.Second) / rate)).Sub(now)
	if remaining < 0 {
		return 0
	}
	return remaining
}

// capture streams frames until canceled or an operation fails.
//
// Example: stream a scaled display to standard output.
func capture(ctx context.Context, out io.Writer, sourceWidth, sourceHeight, width, height int32, displayID int, skip bool, rate float64) (resultErr error) {
	session, err := gui.OpenCapture(ctx, gui.CaptureOptions{SourceWidth: sourceWidth, SourceHeight: sourceHeight, Width: width, Height: height, DisplayID: uint64(displayID)})
	if session != nil {
		defer func() {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
			defer cancel()
			resultErr = errors.Join(resultErr, session.Close(cleanupCtx))
		}()
	}
	if err != nil {
		return err
	}
	pixels := make([]byte, int(width)*int(height)*4)
	for {
		if server, ok := out.(*socketStream); ok {
			_, err := server.WaitForClient()
			if errors.Is(err, context.Canceled) {
				return nil
			}
			if err != nil {
				return err
			}
		}
		frame, err := session.Next(ctx)
		if errors.Is(err, context.Canceled) {
			return nil
		}
		if err != nil {
			return err
		}
		frameStart := time.Now()
		if skip {
			skipper := &queuedFrameSkipper{producer: session, current: frame}
			err := skipAtMost(ctx, captureBufferCount-1, skipper)
			frame = skipper.current
			if err != nil {
				cleanupCtx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
				releaseErr := releaseFrame(cleanupCtx, frame)
				cancel()
				if errors.Is(err, context.Canceled) {
					return releaseErr
				}
				return errors.Join(err, releaseErr)
			}
		}
		copyErr := frame.CopyRGBAInto(ctx, pixels)
		cleanupCtx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
		releaseErr := releaseFrame(cleanupCtx, frame)
		cancel()
		if errors.Is(copyErr, context.Canceled) && releaseErr == nil {
			return nil
		}
		if err := errors.Join(copyErr, releaseErr); err != nil {
			return err
		}
		err = writeFrame(out, pixels)
		switch {
		case errors.Is(err, errCaptureDone), errors.Is(err, context.Canceled):
			return nil
		case errors.Is(err, errPeerDisconnected):
			continue
		case err != nil:
			return fmt.Errorf("write frame: %w", err)
		}
		if rate > 0 {
			timer := time.NewTimer(frameDelay(frameStart, time.Now(), rate))
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				return nil
			}
		}
	}
}

// run selects an output mode and captures until interrupted.
//
// Example: use -s to write a single JPEG.
func run(ctx context.Context, args []string, out io.Writer) (resultErr error) {
	opts, err := parseOptions(args)
	if errors.Is(err, flag.ErrHelp) {
		_, err = io.WriteString(out, "Usage: minicap -P WIDTHxHEIGHT@WIDTHxHEIGHT/ROTATION [-n SOCKET] [-Q QUALITY] [-s|-t|-raw] [-S] [-r FPS] [-d DISPLAY] [-i]\n")
		return err
	}
	if err != nil {
		return err
	}
	if opts.info {
		return writeDisplayInfo(ctx, out, opts.displayID)
	}
	fmt.Fprintf(os.Stderr, "PID: %d\n", os.Getpid())
	sw, sh, w, h := opts.projection.captureSize()
	if opts.raw {
		if f, ok := out.(*os.File); ok {
			writer, err := newRawFileWriter(ctx, f)
			if err != nil {
				return err
			}
			defer func() { resultErr = errors.Join(resultErr, writer.Close()) }()
			out = writer
		}
	}
	switch {
	case opts.screenshot || opts.test:
		out = singleFrameWriter{out: out, width: w, height: h, quality: opts.quality, test: opts.test}
	case !opts.raw:
		server, err := newSocketStream(ctx, opts.socketName, opts.projection, w, h, opts.quality, os.Getpid())
		if err != nil {
			return err
		}
		defer server.Close()
		out = server
	}
	return capture(ctx, out, sw, sh, w, h, opts.displayID, opts.skip, opts.rate)
}

// main parses options and reports capture errors.
//
// Example: stream JPEG frames over the minicap abstract socket.
func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
