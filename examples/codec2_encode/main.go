// Codec2 H.264 encoding via binder.
//
// This example connects to the Codec2 software IComponentStore, creates an
// H.264 encoder component, feeds a single gray YUV frame, and reports the
// encoded output size.
//
// Build:
//
//	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -buildmode=pie -o build/codec2_encode ./examples/codec2_encode/
//	adb push build/codec2_encode /data/local/tmp/ && adb shell /data/local/tmp/codec2_encode
package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"time"

	common "github.com/AndroidGoLab/binder/android/hardware/common"
	c2 "github.com/AndroidGoLab/binder/android/hardware/media/c2"
	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/binder/versionaware"
	"github.com/AndroidGoLab/binder/kernelbinder"
	"github.com/AndroidGoLab/binder/servicemanager"
	"golang.org/x/sys/unix"
)

const (
	serviceName    = "android.hardware.media.c2.IComponentStore/software"
	avcEncoderName = "c2.android.avc.encoder"
	width          = 320
	height         = 240
	bitrate        = 512000
)

// componentListener receives callbacks from the encoder.
type componentListener struct {
	workDoneCh chan c2.WorkBundle
}

var _ c2.IComponentListenerServer = (*componentListener)(nil)

func (l *componentListener) OnError(
	_ context.Context,
	status c2.Status,
	errorCode int32,
) error {
	fmt.Fprintf(os.Stderr, "OnError: status=%d, code=%d\n", status.Status, errorCode)
	return nil
}

func (l *componentListener) OnFramesRendered(
	_ context.Context,
	_ []c2.IComponentListenerRenderedFrame,
) error {
	return nil
}

func (l *componentListener) OnInputBuffersReleased(
	_ context.Context,
	_ []c2.IComponentListenerInputBuffer,
) error {
	return nil
}

func (l *componentListener) OnTripped(
	_ context.Context,
	_ []c2.SettingResult,
) error {
	return nil
}

func (l *componentListener) OnWorkDone(
	_ context.Context,
	wb c2.WorkBundle,
) error {
	select {
	case l.workDoneCh <- wb:
	default:
	}
	return nil
}

// buildC2Param constructs a single C2 param blob.
func buildC2Param(
	index uint32,
	payload []byte,
) []byte {
	totalSize := 8 + uint32(len(payload))
	buf := make([]byte, totalSize)
	binary.LittleEndian.PutUint32(buf[0:], totalSize)
	binary.LittleEndian.PutUint32(buf[4:], index)
	copy(buf[8:], payload)
	return buf
}

func buildPictureSizeParam(
	stream uint32,
	w uint32,
	h uint32,
) []byte {
	index := uint32(0x4B400000) | (stream << 17)
	payload := make([]byte, 8)
	binary.LittleEndian.PutUint32(payload[0:], w)
	binary.LittleEndian.PutUint32(payload[4:], h)
	return buildC2Param(index, payload)
}

func buildBitrateParam(
	stream uint32,
	rate uint32,
) []byte {
	index := uint32(0x4B200000) | (stream << 17)
	payload := make([]byte, 4)
	binary.LittleEndian.PutUint32(payload[0:], rate)
	return buildC2Param(index, payload)
}

func makeGrayYUVFrame(
	w int,
	h int,
) []byte {
	ySize := w * h
	uvSize := (w / 2) * (h / 2) * 2
	frame := make([]byte, ySize+uvSize)
	for i := range frame {
		frame[i] = 128
	}
	return frame
}

func run(ctx context.Context) error {
	drv, err := kernelbinder.Open(ctx, binder.WithMapSize(4*1024*1024))
	if err != nil {
		return fmt.Errorf("open binder: %w", err)
	}
	defer func() { _ = drv.Close(ctx) }()

	transport, err := versionaware.NewTransport(ctx, drv, 0)
	if err != nil {
		return fmt.Errorf("transport: %w", err)
	}
	sm := servicemanager.New(transport)

	// Connect to the Codec2 software component store.
	svc, err := sm.GetService(ctx, servicemanager.ServiceName(serviceName))
	if err != nil {
		return fmt.Errorf("get Codec2 service: %w", err)
	}
	if svc == nil {
		return fmt.Errorf("Codec2 IComponentStore/software not found")
	}
	store := c2.NewComponentStoreProxy(svc)

	// List components to verify the encoder exists.
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
		return fmt.Errorf("encoder %s not found in component list", avcEncoderName)
	}

	// Create the listener stub.
	listenerImpl := &componentListener{
		workDoneCh: make(chan c2.WorkBundle, 16),
	}
	listener := c2.NewComponentListenerStub(listenerImpl)

	// Get the pool client manager from the store.
	poolMgr, err := store.GetPoolClientManager(ctx)
	if err != nil {
		return fmt.Errorf("get pool client manager: %w", err)
	}

	// Create the encoder component.
	component, err := store.CreateComponent(ctx, avcEncoderName, listener, poolMgr)
	if err != nil {
		return fmt.Errorf("create component: %w", err)
	}
	defer func() { _ = component.Release(ctx) }()
	fmt.Println("Encoder component created")

	// Configure via IConfigurable.
	iface, err := component.GetInterface(ctx)
	if err != nil {
		return fmt.Errorf("get interface: %w", err)
	}
	configurable := c2.NewConfigurableProxy(iface.AsBinder())

	configParams := append(
		buildPictureSizeParam(1, width, height),
		buildBitrateParam(1, bitrate)...,
	)
	configResult, err := configurable.Config(
		ctx,
		c2.Params{Params: configParams},
		true,
	)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	fmt.Printf("Config: status=%d, failures=%d\n",
		configResult.Status.Status, len(configResult.Failures))

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

	// Build the work bundle.
	workBundle := c2.WorkBundle{
		Works: []c2.Work{
			{
				Input: c2.FrameData{
					Flags: 0,
					Ordinal: c2.WorkOrdinal{
						TimestampUs:   0,
						FrameIndex:    0,
						CustomOrdinal: 0,
					},
					Buffers: []c2.Buffer{
						{
							Info: c2.Params{},
							Blocks: []c2.Block{
								{
									Index: 0,
									Meta:  c2.Params{},
									Fence: common.NativeHandle{},
								},
							},
						},
					},
					ConfigUpdate: c2.Params{},
				},
				Worklets: []c2.Worklet{
					{
						ComponentId: 0,
						Output: c2.FrameData{
							Ordinal: c2.WorkOrdinal{},
						},
					},
				},
				WorkletsProcessed: 0,
				Result:            c2.Status{Status: c2.StatusOK},
			},
		},
		BaseBlocks: []c2.BaseBlock{
			{
				Tag: c2.BaseBlockTagNativeBlock,
				NativeBlock: common.NativeHandle{
					Fds:  []int32{int32(frameFd)},
					Ints: []int32{int32(len(frameData))},
				},
			},
		},
	}

	if err := component.Queue(ctx, workBundle); err != nil {
		return fmt.Errorf("queue: %w", err)
	}
	fmt.Println("Frame queued")

	// Send EOS.
	eosBundle := c2.WorkBundle{
		Works: []c2.Work{
			{
				Input: c2.FrameData{
					Flags: c2.FrameDataEndOfStream,
					Ordinal: c2.WorkOrdinal{
						TimestampUs:   33333,
						FrameIndex:    1,
						CustomOrdinal: 0,
					},
					ConfigUpdate: c2.Params{},
				},
				Worklets: []c2.Worklet{
					{
						ComponentId: 0,
						Output: c2.FrameData{
							Ordinal: c2.WorkOrdinal{},
						},
					},
				},
				WorkletsProcessed: 0,
				Result:            c2.Status{Status: c2.StatusOK},
			},
		},
	}
	if err := component.Queue(ctx, eosBundle); err != nil {
		return fmt.Errorf("queue EOS: %w", err)
	}
	fmt.Println("EOS queued")

	// Wait for output callback or flush.
	select {
	case wb := <-listenerImpl.workDoneCh:
		fmt.Printf("OnWorkDone received: %d works, %d base blocks\n",
			len(wb.Works), len(wb.BaseBlocks))
		for i, w := range wb.Works {
			fmt.Printf("  Work[%d]: result=%d\n", i, w.Result.Status)
			if len(w.Worklets) > 0 {
				wl := w.Worklets[0]
				fmt.Printf("  Worklet output: flags=%d, buffers=%d\n",
					wl.Output.Flags, len(wl.Output.Buffers))
			}
		}
	case <-time.After(5 * time.Second):
		fmt.Println("Timeout waiting for callback; trying Flush...")
		flushBundle, flushErr := component.Flush(ctx)
		if flushErr != nil {
			fmt.Printf("Flush error: %v\n", flushErr)
		} else {
			fmt.Printf("Flush: %d works, %d base blocks\n",
				len(flushBundle.Works), len(flushBundle.BaseBlocks))
		}
	}

	return nil
}

func main() {
	ctx := context.Background()
	if err := run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
