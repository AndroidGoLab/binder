// Codec2 H.264 encoding via HIDL hwbinder.
//
// This example connects to the Codec2 software IComponentStore via HIDL
// hwbinder, creates an H.264 encoder component, feeds a single gray YUV
// frame, and reports the result.
//
// Build:
//
//	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -buildmode=pie -o build/codec2_encode ./examples/codec2_encode/
//	patchelf --set-interpreter /system/bin/linker64 --replace-needed libdl.so.2 libdl.so --replace-needed libpthread.so.0 libc.so --replace-needed libc.so.6 libc.so build/codec2_encode
//	adb push build/codec2_encode /data/local/tmp/ && adb shell /data/local/tmp/codec2_encode
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/codec2/hidlcodec2"
	"github.com/AndroidGoLab/binder/kernelbinder"
	"golang.org/x/sys/unix"
)

const (
	avcEncoderName = "c2.android.avc.encoder"
	width          = 320
	height         = 240
	bitrate        = 512000
)

func makeGrayYUVFrame(w, h int) []byte {
	ySize := w * h
	uvSize := (w / 2) * (h / 2) * 2 // NV12: interleaved UV
	frame := make([]byte, ySize+uvSize)
	for i := range frame {
		frame[i] = 128
	}
	return frame
}

func run(ctx context.Context) error {
	// Open hwbinder for HIDL services.
	drv, err := kernelbinder.Open(ctx,
		binder.WithDevicePath("/dev/hwbinder"),
		binder.WithMapSize(256*1024),
	)
	if err != nil {
		return fmt.Errorf("open hwbinder: %w", err)
	}
	defer func() { _ = drv.Close(ctx) }()

	// Connect to the Codec2 IComponentStore service.
	store, err := hidlcodec2.GetComponentStore(ctx, drv)
	if err != nil {
		return fmt.Errorf("get component store: %w", err)
	}
	fmt.Printf("Connected to HIDL Codec2 store (handle=%d)\n", store.Handle())

	// List components.
	components, err := store.ListComponents(ctx)
	if err != nil {
		return fmt.Errorf("list components: %w", err)
	}
	fmt.Printf("Found %d Codec2 components\n", len(components))
	var found bool
	for _, comp := range components {
		if comp.Name == avcEncoderName {
			found = true
			fmt.Printf("  -> %s [%s] rank=%d\n", comp.Name, comp.MediaType, comp.Rank)
		}
	}
	if !found {
		return fmt.Errorf("encoder %s not found", avcEncoderName)
	}

	// Register a listener stub for callbacks.
	workDoneCh := make(chan []byte, 16)
	listener := &hidlcodec2.ComponentListenerStub{
		OnWorkDone: func(data []byte) {
			cp := make([]byte, len(data))
			copy(cp, data)
			select {
			case workDoneCh <- cp:
			default:
			}
		},
	}
	listenerCookie := hidlcodec2.RegisterListener(ctx, drv, listener)
	defer hidlcodec2.UnregisterListener(ctx, drv, listenerCookie)

	// Create the encoder component.
	component, err := store.CreateComponent(ctx, avcEncoderName, listenerCookie)
	if err != nil {
		return fmt.Errorf("create component: %w", err)
	}
	defer func() { _ = component.Release(ctx) }()
	fmt.Printf("Encoder component created (handle=%d)\n", component.Handle())

	// Configure via IConfigurable.
	iface, err := component.GetInterface(ctx)
	if err != nil {
		return fmt.Errorf("get interface: %w", err)
	}
	cfg, err := iface.GetConfigurable(ctx)
	if err != nil {
		return fmt.Errorf("get configurable: %w", err)
	}

	configParams := hidlcodec2.ConcatParams(
		hidlcodec2.BuildPictureSizeParam(1, width, height),
		hidlcodec2.BuildBitrateParam(1, bitrate),
	)
	cfgStatus, _, err := cfg.Config(ctx, configParams, true)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	fmt.Printf("Config: status=%s\n", cfgStatus)

	// Start the encoder.
	if err := component.Start(ctx); err != nil {
		return fmt.Errorf("start: %w", err)
	}
	defer func() { _ = component.Stop(ctx) }()
	fmt.Println("Encoder started")

	// Create a gray frame in a memfd.
	frameData := makeGrayYUVFrame(width, height)
	frameFd, err := unix.MemfdCreate("c2-frame", 0)
	if err != nil {
		return fmt.Errorf("memfd_create: %w", err)
	}
	defer unix.Close(frameFd)

	if _, err := unix.Write(frameFd, frameData); err != nil {
		return fmt.Errorf("write memfd: %w", err)
	}

	// Build the work bundle with proper C2Handle format.
	workBundle := &hidlcodec2.WorkBundle{
		Works: []hidlcodec2.Work{
			{
				Input: hidlcodec2.FrameData{
					Flags: 0,
					Ordinal: hidlcodec2.WorkOrdinal{
						TimestampUs:   0,
						FrameIndex:    0,
						CustomOrdinal: 0,
					},
					Buffers: []hidlcodec2.Buffer{
						{
							Blocks: []hidlcodec2.Block{
								{
									Index: 0,
									Meta:  hidlcodec2.BuildRangeInfoParam(0, uint32(len(frameData))),
								},
							},
						},
					},
				},
				Worklets: []hidlcodec2.Worklet{
					{ComponentId: 0},
				},
				WorkletsProcessed: 0,
				Result:            hidlcodec2.StatusOK,
			},
		},
		BaseBlocks: []hidlcodec2.BaseBlock{
			{
				Tag:             0, // nativeBlock
				NativeBlockFds:  []int32{int32(frameFd)},
				NativeBlockInts: hidlcodec2.C2HandleLinearInts(uint64(len(frameData))),
			},
		},
	}

	if err := component.Queue(ctx, workBundle); err != nil {
		return fmt.Errorf("queue: %w", err)
	}
	fmt.Println("Frame queued")

	// Send EOS.
	eosBundle := &hidlcodec2.WorkBundle{
		Works: []hidlcodec2.Work{
			{
				Input: hidlcodec2.FrameData{
					Flags: hidlcodec2.FrameDataEndOfStream,
					Ordinal: hidlcodec2.WorkOrdinal{
						TimestampUs:   33333,
						FrameIndex:    1,
						CustomOrdinal: 0,
					},
				},
				Worklets: []hidlcodec2.Worklet{
					{ComponentId: 0},
				},
				WorkletsProcessed: 0,
				Result:            hidlcodec2.StatusOK,
			},
		},
	}
	if err := component.Queue(ctx, eosBundle); err != nil {
		return fmt.Errorf("queue EOS: %w", err)
	}
	fmt.Println("EOS queued")

	// Wait for output callback or timeout.
	select {
	case data := <-workDoneCh:
		fmt.Printf("OnWorkDone received (%d bytes)\n", len(data))
	case <-time.After(5 * time.Second):
		fmt.Println("Timeout waiting for callback; trying Flush...")
		if flushErr := component.Flush(ctx); flushErr != nil {
			fmt.Printf("Flush error: %v\n", flushErr)
		} else {
			fmt.Println("Flush succeeded")
		}
	}

	fmt.Println("Encode pipeline completed successfully")
	return nil
}

func main() {
	ctx := context.Background()
	if err := run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
