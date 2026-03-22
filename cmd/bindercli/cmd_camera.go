//go:build linux

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"sync"
	"time"

	gfxCommon "github.com/xaionaro-go/binder/android/hardware/graphics/common"

	fwkDevice "github.com/xaionaro-go/binder/android/frameworks/cameraservice/device"
	fwkService "github.com/xaionaro-go/binder/android/frameworks/cameraservice/service"
	"github.com/xaionaro-go/binder/binder"
	"github.com/xaionaro-go/binder/camera"
	"github.com/xaionaro-go/binder/camera/gralloc"
	cameraIGBP "github.com/xaionaro-go/binder/camera/igbp"

	"github.com/spf13/cobra"
)

func newCameraCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "camera",
		Short: "Camera capture commands",
	}

	cmd.AddCommand(newCameraRecordCmd())

	return cmd
}

func newCameraRecordCmd() *cobra.Command {
	var (
		width    int
		height   int
		cameraID string
		duration time.Duration
	)

	cmd := &cobra.Command{
		Use:   "record",
		Short: "Record raw YUV frames from a camera to stdout",
		Long: `Record captures camera frames and writes raw YUV (NV12/YCbCr_420_888)
data to stdout. Status messages go to stderr.

Example:
  bindercli camera record --width 1920 --height 1920 --duration 5s > output.yuv
  bindercli camera record | ffmpeg -f rawvideo -pix_fmt nv12 -s 1920x1920 -i - output.mp4`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runCameraRecord(
				cmd,
				int32(width),
				int32(height),
				cameraID,
				duration,
				os.Stdout,
			)
		},
	}

	cmd.Flags().IntVar(&width, "width", 1920, "capture width in pixels")
	cmd.Flags().IntVar(&height, "height", 1920, "capture height in pixels")
	cmd.Flags().StringVar(&cameraID, "camera", "0", "camera device ID")
	cmd.Flags().DurationVar(&duration, "duration", 10*time.Second, "recording duration")

	return cmd
}

// cameraCallback implements fwkDevice.ICameraDeviceCallback with
// stderr logging suitable for the CLI recording flow.
type cameraCallback struct {
	mu             sync.Mutex
	framesReceived int
}

func (c *cameraCallback) OnCaptureStarted(
	_ context.Context,
	extras fwkDevice.CaptureResultExtras,
	ts int64,
) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.framesReceived++
	fmt.Fprintf(os.Stderr, "  >> OnCaptureStarted: requestId=%d timestamp=%d (total=%d)\n",
		extras.RequestId, ts, c.framesReceived)
	return nil
}

func (c *cameraCallback) OnDeviceError(
	_ context.Context,
	code fwkDevice.ErrorCode,
	extras fwkDevice.CaptureResultExtras,
) error {
	fmt.Fprintf(os.Stderr, "  >> OnDeviceError: code=%d requestId=%d\n", code, extras.RequestId)
	return nil
}

func (c *cameraCallback) OnDeviceIdle(_ context.Context) error {
	fmt.Fprintln(os.Stderr, "  >> OnDeviceIdle")
	return nil
}

func (c *cameraCallback) OnPrepared(_ context.Context, streamId int32) error {
	fmt.Fprintf(os.Stderr, "  >> OnPrepared: stream %d\n", streamId)
	return nil
}

func (c *cameraCallback) OnRepeatingRequestError(
	_ context.Context,
	lastFrame int64,
	reqId int32,
) error {
	fmt.Fprintf(os.Stderr, "  >> OnRepeatingRequestError: frame=%d req=%d\n", lastFrame, reqId)
	return nil
}

func (c *cameraCallback) OnResultReceived(
	_ context.Context,
	meta fwkDevice.CaptureMetadataInfo,
	extras fwkDevice.CaptureResultExtras,
	_ []fwkDevice.PhysicalCaptureResultInfo,
) error {
	fmt.Fprintf(os.Stderr, "  >> OnResultReceived: requestId=%d frameNumber=%d tag=%d\n",
		extras.RequestId, extras.FrameNumber, meta.Tag)
	return nil
}

func (c *cameraCallback) OnClientSharedAccessPriorityChanged(
	_ context.Context,
	primary bool,
) error {
	fmt.Fprintf(os.Stderr, "  >> OnClientSharedAccessPriorityChanged: %v\n", primary)
	return nil
}

// runCameraRecord implements the recording flow: allocates gralloc
// buffers, connects to camera, configures a stream with the IGBP stub,
// and writes raw YUV frames to output until the duration expires.
func runCameraRecord(
	cmd *cobra.Command,
	width int32,
	height int32,
	cameraID string,
	duration time.Duration,
	output io.Writer,
) (_err error) {
	ctx := cmd.Context()

	// Disable GC early to prevent startup allocations (DEX parsing,
	// binder setup) from triggering expensive heap scans. The entire
	// camera record flow is short-lived and allocation patterns are
	// bounded, so GC is not needed.
	debug.SetGCPercent(-1)
	defer debug.SetGCPercent(100)

	// Use a larger map size for camera buffers.
	conn, err := OpenConn(ctx, cmd)
	if err != nil {
		return fmt.Errorf("opening binder connection: %w", err)
	}
	defer conn.Close(ctx)

	transport := conn.Transport

	// Step 0: Pre-allocate gralloc buffers (4 slots).
	fmt.Fprintln(os.Stderr, "=== Step 0: Allocate gralloc buffers ===")
	var grallocBufs [4]*gralloc.Buffer
	for i := range grallocBufs {
		buf, allocErr := gralloc.Allocate(
			ctx,
			conn.SM,
			width,
			height,
			gfxCommon.PixelFormatYcbcr420888,
			gfxCommon.BufferUsageCpuReadOften|gfxCommon.BufferUsageCameraOutput,
		)
		if allocErr != nil {
			return fmt.Errorf("allocating gralloc buffer %d: %w", i, allocErr)
		}
		// Pre-mmap the dmabuf so we can read frames without per-frame
		// mmap/munmap syscalls.
		if mmapErr := buf.Mmap(); mmapErr != nil {
			return fmt.Errorf("mmap gralloc buffer %d: %w", i, mmapErr)
		}
		grallocBufs[i] = buf
	}
	defer func() {
		for _, buf := range grallocBufs {
			if buf != nil {
				buf.Munmap()
			}
		}
	}()
	fmt.Fprintf(os.Stderr, "Allocated and mmap'd %d gralloc buffers\n", len(grallocBufs))

	// Connect to camera service.
	svc, err := conn.SM.GetService(ctx, "android.frameworks.cameraservice.service.ICameraService/default")
	if err != nil {
		return fmt.Errorf("getting camera service: %w", err)
	}
	fmt.Fprintln(os.Stderr, "Got frameworks camera service")

	proxy := fwkService.NewCameraServiceProxy(svc)
	cb := &cameraCallback{}
	stub := fwkDevice.NewCameraDeviceCallbackStub(cb)

	stubBinder := stub.AsBinder().(*binder.StubBinder)
	stubBinder.RegisterWithTransport(ctx, transport)
	time.Sleep(100 * time.Millisecond)
	fmt.Fprintln(os.Stderr, "Callback stub registered")

	// ConnectDevice.
	fmt.Fprintln(os.Stderr, "\nCalling ConnectDevice...")
	deviceUser, err := proxy.ConnectDevice(ctx, stub, cameraID)
	if err != nil {
		return fmt.Errorf("ConnectDevice: %w", err)
	}
	fmt.Fprintln(os.Stderr, "ConnectDevice succeeded!")

	defer func() {
		fmt.Fprintln(os.Stderr, "\n=== Disconnect ===")
		if disconnectErr := deviceUser.Disconnect(ctx); disconnectErr != nil {
			fmt.Fprintf(os.Stderr, "Disconnect: %v\n", disconnectErr)
		} else {
			fmt.Fprintln(os.Stderr, "Disconnect OK")
		}
	}()

	// Step 1: BeginConfigure.
	fmt.Fprintln(os.Stderr, "\n=== Step 1: BeginConfigure ===")
	if err = deviceUser.BeginConfigure(ctx); err != nil {
		return fmt.Errorf("BeginConfigure: %w", err)
	}
	fmt.Fprintln(os.Stderr, "BeginConfigure OK")

	// Step 2: CreateDefaultRequest.
	fmt.Fprintln(os.Stderr, "\n=== Step 2: CreateDefaultRequest (PREVIEW) ===")
	metadataBytes, err := camera.CreateDefaultRequest(ctx, deviceUser, fwkDevice.TemplateIdPREVIEW)
	if err != nil {
		return fmt.Errorf("CreateDefaultRequest: %w", err)
	}
	fmt.Fprintf(os.Stderr, "CreateDefaultRequest OK: metadata len=%d\n", len(metadataBytes))

	// Step 3: Create IGBP and CreateStream.
	fmt.Fprintln(os.Stderr, "\n=== Step 3: CreateStream with IGBP Surface ===")
	igbpStub := cameraIGBP.NewProducerStub(uint32(width), uint32(height), grallocBufs)
	igbpStubBinder := binder.NewStubBinder(igbpStub)
	igbpStubBinder.RegisterWithTransport(ctx, transport)

	streamId, err := camera.CreateStreamWithSurface(ctx, deviceUser, transport, igbpStubBinder, width, height)
	if err != nil {
		return fmt.Errorf("CreateStream: %w", err)
	}
	fmt.Fprintf(os.Stderr, "CreateStream OK: streamId=%d\n", streamId)

	// Step 4: EndConfigure.
	fmt.Fprintln(os.Stderr, "\n=== Step 4: EndConfigure ===")
	if err = deviceUser.EndConfigure(ctx, fwkDevice.StreamConfigurationModeNormalMode, fwkDevice.CameraMetadata{Metadata: []byte{}}, 0); err != nil {
		return fmt.Errorf("EndConfigure: %w", err)
	}
	fmt.Fprintln(os.Stderr, "EndConfigure OK")

	// Step 5: SubmitRequestList.
	fmt.Fprintln(os.Stderr, "\n=== Step 5: SubmitRequestList (repeating) ===")
	captureReq := fwkDevice.CaptureRequest{
		PhysicalCameraSettings: []fwkDevice.PhysicalCameraSettings{
			{
				Id: cameraID,
				Settings: fwkDevice.CaptureMetadataInfo{
					Tag:      fwkDevice.CaptureMetadataInfoTagMetadata,
					Metadata: fwkDevice.CameraMetadata{Metadata: metadataBytes},
				},
			},
		},
		StreamAndWindowIds: []fwkDevice.StreamAndWindowId{
			{StreamId: streamId, WindowId: 0},
		},
	}

	submitInfo, err := camera.SubmitRequest(ctx, deviceUser, captureReq, true)
	if err != nil {
		fmt.Fprintf(os.Stderr, "SubmitRequestList (repeating) FAILED: %v\n", err)
		submitInfo, err = camera.SubmitRequest(ctx, deviceUser, captureReq, false)
		if err != nil {
			return fmt.Errorf("SubmitRequestList: %w", err)
		}
	}
	fmt.Fprintf(os.Stderr, "SubmitRequestList OK: requestId=%d lastFrame=%d\n",
		submitInfo.RequestId, submitInfo.LastFrameNumber)

	// Wait for frames and write them to output.
	fmt.Fprintf(os.Stderr, "\nRecording for %s...\n", duration)

	frameCount := 0
	deadline := time.After(duration)
	// Use a reusable ticker instead of time.After per iteration,
	// which would allocate a new timer+channel each loop.
	pollTicker := time.NewTicker(200 * time.Millisecond)
	defer pollTicker.Stop()
	for {
		select {
		case <-deadline:
			fmt.Fprintf(os.Stderr, "Duration reached. Total frames written: %d\n", frameCount)
			return nil
		case slot := <-igbpStub.QueuedFrames():
			buf := igbpStub.SlotBuffer(slot)
			if buf == nil {
				fmt.Fprintf(os.Stderr, "  Frame from slot %d: no buffer\n", slot)
				continue
			}

			// Write directly from the persistent mmap to output,
			// avoiding an intermediate copy through a frame buffer.
			frameData := buf.MmapData
			if frameData == nil {
				fmt.Fprintf(os.Stderr, "  Frame from slot %d: buffer not mmap'd\n", slot)
				continue
			}

			if _, writeErr := output.Write(frameData); writeErr != nil {
				return fmt.Errorf("writing frame data: %w", writeErr)
			}
			frameCount++
			fmt.Fprintf(os.Stderr, "  Frame %d written (%d bytes)\n", frameCount, len(frameData))

		case <-pollTicker.C:
			// Periodic check: if no frames arrive at all, keep waiting
			// until the deadline.
		}
	}
}
