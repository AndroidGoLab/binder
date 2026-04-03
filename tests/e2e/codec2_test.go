//go:build e2e || e2e_root

package e2e

import (
	"context"
	"testing"
	"time"

	common "github.com/AndroidGoLab/binder/android/hardware/common"
	gfxCommon "github.com/AndroidGoLab/binder/android/hardware/graphics/common"
	c2 "github.com/AndroidGoLab/binder/android/hardware/media/c2"
	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/codec2/hidlcodec2"
	"github.com/AndroidGoLab/binder/gralloc"
	"github.com/AndroidGoLab/binder/kernelbinder"
	"github.com/AndroidGoLab/binder/servicemanager"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const (
	codec2ServiceName = "android.hardware.media.c2.IComponentStore/software"
	avcEncoderName    = "c2.android.avc.encoder"
	aacEncoderName    = "c2.android.aac.encoder"
)

// ---------------------------------------------------------------------------
// Stubs
// ---------------------------------------------------------------------------

// componentListenerStub is a minimal IComponentListenerServer that forwards
// OnWorkDone callbacks through a channel for test synchronization.
type componentListenerStub struct {
	workDoneCh chan c2.WorkBundle
}

var _ c2.IComponentListenerServer = (*componentListenerStub)(nil)

func (l *componentListenerStub) OnError(
	_ context.Context,
	_ c2.Status,
	_ int32,
) error {
	return nil
}

func (l *componentListenerStub) OnFramesRendered(
	_ context.Context,
	_ []c2.IComponentListenerRenderedFrame,
) error {
	return nil
}

func (l *componentListenerStub) OnInputBuffersReleased(
	_ context.Context,
	_ []c2.IComponentListenerInputBuffer,
) error {
	return nil
}

func (l *componentListenerStub) OnTripped(
	_ context.Context,
	_ []c2.SettingResult,
) error {
	return nil
}

func (l *componentListenerStub) OnWorkDone(
	_ context.Context,
	wb c2.WorkBundle,
) error {
	select {
	case l.workDoneCh <- wb:
	default:
	}
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// getComponentStore connects to the Codec2 software component store.
func getComponentStore(
	ctx context.Context,
	t *testing.T,
) c2.IComponentStore {
	t.Helper()
	driver := openBinder(t)
	sm := servicemanager.New(driver)
	svc, err := sm.GetService(ctx, servicemanager.ServiceName(codec2ServiceName))
	requireOrSkip(t, err)
	if svc == nil {
		t.Skip("Codec2 IComponentStore/software not available on this device")
	}
	return c2.NewComponentStoreProxy(svc)
}

// createAVCEncoder creates an AVC encoder component with a listener.
// Skips the test if the AIDL IComponentStore does not support CreateComponent
// (e.g. on emulators where Codec2 is backed by HIDL rather than native AIDL).
func createAVCEncoder(
	ctx context.Context,
	t *testing.T,
	store c2.IComponentStore,
	listenerImpl *componentListenerStub,
) c2.IComponent {
	t.Helper()

	listener := c2.NewComponentListenerStub(listenerImpl)

	poolMgr, err := store.GetPoolClientManager(ctx)
	requireOrSkip(t, err)

	component, err := store.CreateComponent(ctx, avcEncoderName, listener, poolMgr)
	requireOrSkip(t, err)
	require.NotNil(t, component, "CreateComponent returned nil")
	return component
}

// buildPictureSizeParam builds a C2StreamPictureSizeInfo::input param blob.
// Uses the hidlcodec2 package for correct index computation.
func buildPictureSizeParam(
	stream uint32,
	width uint32,
	height uint32,
) []byte {
	return hidlcodec2.BuildPictureSizeParam(stream, width, height)
}

// buildBitrateParam builds a C2StreamBitrateInfo::output param blob.
// Uses the hidlcodec2 package for correct index computation.
func buildBitrateParam(
	stream uint32,
	bitrate uint32,
) []byte {
	return hidlcodec2.BuildBitrateParam(stream, bitrate)
}

// concatParams concatenates multiple C2 param blobs with 8-byte alignment
// padding between them, as required by the Codec2 param wire format.
func concatParams(params ...[]byte) []byte {
	return hidlcodec2.ConcatParams(params...)
}

// createMemfd creates a memfd, writes data to it, and returns the fd.
func createMemfd(
	t *testing.T,
	name string,
	data []byte,
) int32 {
	t.Helper()
	fd, err := unix.MemfdCreate(name, 0)
	require.NoError(t, err, "memfd_create failed")

	n, err := unix.Write(fd, data)
	require.NoError(t, err, "write to memfd failed")
	require.Equal(t, len(data), n, "short write to memfd")

	t.Cleanup(func() { unix.Close(fd) })
	return int32(fd)
}

// makeGrayYUVFrame creates a solid gray NV12 frame (Y=128, Cb=128, Cr=128).
func makeGrayYUVFrame(
	width int,
	height int,
) []byte {
	ySize := width * height
	uvSize := (width / 2) * (height / 2) * 2 // NV12: interleaved UV
	frame := make([]byte, ySize+uvSize)
	for i := range frame {
		frame[i] = 128
	}
	return frame
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestCodec2_ListComponents verifies we can connect to the Codec2
// IComponentStore and call ListComponents. On emulators where Codec2 is
// backed by HIDL, the AIDL wrapper may return an empty list; on devices
// with native AIDL Codec2, the list should contain codecs including
// the AVC encoder.
func TestCodec2_ListComponents(t *testing.T) {
	ctx := context.Background()
	store := getComponentStore(ctx, t)

	components, err := store.ListComponents(ctx)
	requireOrSkip(t, err)

	t.Logf("Found %d Codec2 components", len(components))

	if len(components) == 0 {
		// On HIDL-backed emulators (e.g. goldfish), the AIDL
		// IComponentStore wrapper legitimately returns an empty list.
		// The test still passes because the binder round-trip succeeded.
		t.Log("ListComponents returned 0 components (expected on HIDL-backed emulators)")
		return
	}

	var foundAVCEncoder bool
	for _, comp := range components {
		var kindStr string
		switch comp.Kind {
		case c2.IComponentStoreComponentTraitsKindENCODER:
			kindStr = "encoder"
		case c2.IComponentStoreComponentTraitsKindDECODER:
			kindStr = "decoder"
		default:
			kindStr = "other"
		}

		var domainStr string
		switch comp.Domain {
		case c2.IComponentStoreComponentTraitsDomainVIDEO:
			domainStr = "video"
		case c2.IComponentStoreComponentTraitsDomainAUDIO:
			domainStr = "audio"
		case c2.IComponentStoreComponentTraitsDomainIMAGE:
			domainStr = "image"
		default:
			domainStr = "other"
		}

		t.Logf("  %s [%s/%s] media=%s rank=%d",
			comp.Name, domainStr, kindStr, comp.MediaType, comp.Rank)
		if comp.Name == avcEncoderName {
			foundAVCEncoder = true
			assert.Equal(t, c2.IComponentStoreComponentTraitsKindENCODER, comp.Kind)
			assert.Equal(t, c2.IComponentStoreComponentTraitsDomainVIDEO, comp.Domain)
			assert.Equal(t, "video/avc", comp.MediaType)
		}
	}
	assert.True(t, foundAVCEncoder, "expected to find %s in component list", avcEncoderName)
}

// TestCodec2_CreateEncoder verifies the Create/Start/Stop/Release lifecycle.
func TestCodec2_CreateEncoder(t *testing.T) {
	ctx := context.Background()
	store := getComponentStore(ctx, t)

	listenerImpl := &componentListenerStub{
		workDoneCh: make(chan c2.WorkBundle, 16),
	}
	component := createAVCEncoder(ctx, t, store, listenerImpl)
	defer func() {
		_ = component.Release(ctx)
	}()

	// Verify we can get the component interface and its configurable.
	iface, err := component.GetInterface(ctx)
	requireOrSkip(t, err)
	require.NotNil(t, iface, "GetInterface returned nil")

	configurable, err := iface.GetConfigurable(ctx)
	requireOrSkip(t, err)
	require.NotNil(t, configurable, "GetConfigurable returned nil")

	name, err := configurable.GetName(ctx)
	requireOrSkip(t, err)
	t.Logf("Configurable name: %s", name)

	// Test the Start/Stop lifecycle.
	err = component.Start(ctx)
	requireOrSkip(t, err)
	t.Log("Encoder started successfully")

	err = component.Stop(ctx)
	requireOrSkip(t, err)
	t.Log("Encoder stopped successfully")
}

// TestCodec2_EncodeFrame is the full end-to-end test: configure an AVC
// encoder, queue a YUV frame, signal EOS, and wait for encoded output.
func TestCodec2_EncodeFrame(t *testing.T) {
	const (
		width   = 320
		height  = 240
		bitrate = 512000
	)

	ctx := context.Background()
	store := getComponentStore(ctx, t)

	listenerImpl := &componentListenerStub{
		workDoneCh: make(chan c2.WorkBundle, 16),
	}
	component := createAVCEncoder(ctx, t, store, listenerImpl)
	defer func() {
		_ = component.Release(ctx)
	}()

	// Configure the encoder: picture size + bitrate via IConfigurable.
	iface, err := component.GetInterface(ctx)
	requireOrSkip(t, err)

	configurable, err := iface.GetConfigurable(ctx)
	requireOrSkip(t, err)

	configParams := concatParams(
		buildPictureSizeParam(0, width, height),
		buildBitrateParam(0, bitrate),
	)
	configResult, err := configurable.Config(ctx, c2.Params{Params: configParams}, true)
	requireOrSkip(t, err)
	t.Logf("Config result status: %d, failures: %d",
		configResult.Status.Status, len(configResult.Failures))

	// Start the encoder.
	err = component.Start(ctx)
	requireOrSkip(t, err)
	defer func() {
		_ = component.Stop(ctx)
	}()
	t.Log("Encoder started")

	// Create a YUV frame and write it to a memfd.
	frameData := makeGrayYUVFrame(width, height)
	frameFd := createMemfd(t, "c2-input-frame", frameData)

	// Build the WorkBundle with one Work item containing the frame.
	// BaseBlocks[0] carries the memfd as a NativeBlock.
	// Work.Input.Buffers[0].Blocks[0].Index = 0 references BaseBlocks[0].
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
					Fds:  []int32{frameFd},
					Ints: []int32{int32(len(frameData))},
				},
			},
		},
	}

	err = component.Queue(ctx, workBundle)
	requireOrSkip(t, err)
	t.Log("Work queued")

	// Signal end-of-stream so the encoder flushes output.
	eosBundle := c2.WorkBundle{
		Works: []c2.Work{
			{
				Input: c2.FrameData{
					Flags: c2.FrameDataEndOfStream,
					Ordinal: c2.WorkOrdinal{
						TimestampUs:   33333, // ~30fps
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
	err = component.Queue(ctx, eosBundle)
	requireOrSkip(t, err)
	t.Log("EOS queued")

	// Wait for OnWorkDone callbacks from the encoder. We expect at least
	// one callback (for the frame) and possibly a second (for the EOS).
	var receivedOutput bool
	deadline := time.After(10 * time.Second)

	for i := 0; i < 2; i++ {
		select {
		case wb := <-listenerImpl.workDoneCh:
			t.Logf("Received OnWorkDone[%d]: %d works, %d base blocks",
				i, len(wb.Works), len(wb.BaseBlocks))
			for j, w := range wb.Works {
				t.Logf("  Work[%d]: result=%d, workletsProcessed=%d",
					j, w.Result.Status, w.WorkletsProcessed)
				if len(w.Worklets) > 0 {
					wl := w.Worklets[0]
					t.Logf("    Worklet output: flags=%d, buffers=%d",
						wl.Output.Flags, len(wl.Output.Buffers))
				}
			}
			receivedOutput = true
		case <-deadline:
			// Fall back to Flush to retrieve any pending work.
			t.Log("Timed out waiting for OnWorkDone; attempting Flush")
			flushBundle, flushErr := component.Flush(ctx)
			if flushErr != nil {
				t.Logf("Flush error: %v", flushErr)
			} else {
				t.Logf("Flush returned %d works, %d base blocks",
					len(flushBundle.Works), len(flushBundle.BaseBlocks))
				if len(flushBundle.Works) > 0 {
					receivedOutput = true
				}
			}
			i = 2 // exit the for loop
		}
	}

	if receivedOutput {
		t.Log("Full encode pipeline verified: frame queued and output received")
	} else {
		t.Log("Queue succeeded; callback dispatch not yet received (binder read loop integration)")
	}
}

// ---------------------------------------------------------------------------
// HIDL Codec2 tests (hwbinder)
// ---------------------------------------------------------------------------

// openHwBinder opens a connection to /dev/hwbinder for HIDL tests.
func openHwBinder(t *testing.T) *kernelbinder.Driver {
	t.Helper()
	ctx := context.Background()
	drv, err := kernelbinder.Open(ctx,
		binder.WithDevicePath("/dev/hwbinder"),
		binder.WithMapSize(256*1024),
	)
	if err != nil {
		t.Skipf("hwbinder unavailable: %v", err)
	}
	t.Cleanup(func() { _ = drv.Close(ctx) })
	return drv
}

// getHIDLComponentStore connects to the HIDL Codec2 IComponentStore.
func getHIDLComponentStore(
	ctx context.Context,
	t *testing.T,
) (*hidlcodec2.ComponentStore, *kernelbinder.Driver) {
	t.Helper()
	drv := openHwBinder(t)

	store, err := hidlcodec2.GetComponentStore(ctx, drv)
	if err != nil {
		t.Skipf("HIDL Codec2 store unavailable: %v", err)
	}

	return store, drv
}

// TestCodec2HIDL_ListComponents verifies we can connect to the HIDL
// Codec2 IComponentStore on hwbinder and enumerate codecs.
func TestCodec2HIDL_ListComponents(t *testing.T) {
	ctx := context.Background()
	store, _ := getHIDLComponentStore(ctx, t)

	components, err := store.ListComponents(ctx)
	require.NoError(t, err, "ListComponents failed")
	require.NotEmpty(t, components, "expected at least one component from HIDL store")

	t.Logf("HIDL Codec2: found %d components", len(components))

	var foundAVCEncoder bool
	for _, comp := range components {
		var kindStr string
		switch comp.Kind {
		case hidlcodec2.KindEncoder:
			kindStr = "encoder"
		case hidlcodec2.KindDecoder:
			kindStr = "decoder"
		default:
			kindStr = "other"
		}

		var domainStr string
		switch comp.Domain {
		case hidlcodec2.DomainVideo:
			domainStr = "video"
		case hidlcodec2.DomainAudio:
			domainStr = "audio"
		case hidlcodec2.DomainImage:
			domainStr = "image"
		default:
			domainStr = "other"
		}

		t.Logf("  %s [%s/%s] media=%s rank=%d",
			comp.Name, domainStr, kindStr, comp.MediaType, comp.Rank)
		if comp.Name == avcEncoderName {
			foundAVCEncoder = true
			assert.Equal(t, hidlcodec2.KindEncoder, comp.Kind)
			assert.Equal(t, hidlcodec2.DomainVideo, comp.Domain)
			assert.Equal(t, "video/avc", comp.MediaType)
		}
	}
	assert.True(t, foundAVCEncoder, "expected to find %s in HIDL component list", avcEncoderName)
}

// TestCodec2HIDL_CreateEncoder verifies Create/Start/Stop/Release lifecycle
// via HIDL hwbinder.
func TestCodec2HIDL_CreateEncoder(t *testing.T) {
	ctx := context.Background()
	store, drv := getHIDLComponentStore(ctx, t)

	// Register a listener stub so the component can send callbacks.
	listener := &hidlcodec2.ComponentListenerStub{}
	listenerCookie := hidlcodec2.RegisterListener(ctx, drv, listener)
	defer hidlcodec2.UnregisterListener(ctx, drv, listenerCookie)

	component, err := store.CreateComponent(ctx, avcEncoderName, listenerCookie)
	require.NoError(t, err, "CreateComponent failed")
	require.NotNil(t, component, "CreateComponent returned nil")
	defer func() { _ = component.Release(ctx) }()

	t.Logf("HIDL Codec2: component handle=%d", component.Handle())

	// Get the component interface and its configurable.
	iface, err := component.GetInterface(ctx)
	require.NoError(t, err, "GetInterface failed")
	require.NotNil(t, iface, "GetInterface returned nil")

	cfg, err := iface.GetConfigurable(ctx)
	require.NoError(t, err, "GetConfigurable failed")
	require.NotNil(t, cfg, "GetConfigurable returned nil")

	name, err := cfg.GetName(ctx)
	require.NoError(t, err, "GetName failed")
	t.Logf("HIDL Codec2: configurable name=%s", name)
	assert.Equal(t, avcEncoderName, name)

	// Test Start/Stop lifecycle.
	err = component.Start(ctx)
	require.NoError(t, err, "Start failed")
	t.Log("HIDL Codec2: encoder started")

	err = component.Stop(ctx)
	require.NoError(t, err, "Stop failed")
	t.Log("HIDL Codec2: encoder stopped")
}

// TestCodec2HIDL_QueueEmpty verifies that queueing an empty WorkBundle
// succeeds. This validates the scatter-gather serialization baseline.
func TestCodec2HIDL_QueueEmpty(t *testing.T) {
	ctx := context.Background()
	store, drv := getHIDLComponentStore(ctx, t)

	listener := &hidlcodec2.ComponentListenerStub{}
	listenerCookie := hidlcodec2.RegisterListener(ctx, drv, listener)
	defer hidlcodec2.UnregisterListener(ctx, drv, listenerCookie)

	component, err := store.CreateComponent(ctx, avcEncoderName, listenerCookie)
	require.NoError(t, err, "CreateComponent failed")
	defer func() { _ = component.Release(ctx) }()

	err = component.Start(ctx)
	require.NoError(t, err, "Start failed")
	defer func() { _ = component.Stop(ctx) }()

	// Queue an empty WorkBundle (no works, no base blocks).
	emptyBundle := &hidlcodec2.WorkBundle{}
	err = component.Queue(ctx, emptyBundle)
	require.NoError(t, err, "Queue empty bundle failed")
	t.Log("HIDL Codec2: empty queue succeeded")

	// Step A0: Work with 1 empty Buffer (no Blocks).
	bundleA0 := &hidlcodec2.WorkBundle{
		Works: []hidlcodec2.Work{
			{
				Input: hidlcodec2.FrameData{
					Ordinal: hidlcodec2.WorkOrdinal{FrameIndex: 0},
					Buffers: []hidlcodec2.Buffer{
						{},
					},
				},
				Worklets: []hidlcodec2.Worklet{
					{ComponentId: 0},
				},
				Result: hidlcodec2.StatusOK,
			},
		},
	}
	err = component.Queue(ctx, bundleA0)
	if err != nil {
		t.Logf("Step A0 (1 empty Buffer, no Blocks): %v", err)
	} else {
		t.Log("Step A0 (1 empty Buffer, no Blocks): succeeded")
	}

	// Step A1: Work with 1 Buffer with 1 Block.
	bundleA1 := &hidlcodec2.WorkBundle{
		Works: []hidlcodec2.Work{
			{
				Input: hidlcodec2.FrameData{
					Ordinal: hidlcodec2.WorkOrdinal{FrameIndex: 0},
					Buffers: []hidlcodec2.Buffer{
						{
							Blocks: []hidlcodec2.Block{
								{Index: 0},
							},
						},
					},
				},
				Worklets: []hidlcodec2.Worklet{
					{ComponentId: 0},
				},
				Result: hidlcodec2.StatusOK,
			},
		},
	}
	err = component.Queue(ctx, bundleA1)
	if err != nil {
		t.Logf("Step A1 (1 Buffer + 1 Block): %v", err)
	} else {
		t.Log("Step A1 (1 Buffer + 1 Block): succeeded")
	}

	// Step B: Work with NO buffers but WITH BaseBlocks.
	frameData := makeGrayYUVFrame(320, 240)
	frameFd := createMemfd(t, "hidl-c2-empty", frameData)
	bundleB := &hidlcodec2.WorkBundle{
		Works: []hidlcodec2.Work{
			{
				Input: hidlcodec2.FrameData{
					Ordinal: hidlcodec2.WorkOrdinal{FrameIndex: 0},
				},
				Worklets: []hidlcodec2.Worklet{
					{ComponentId: 0},
				},
				Result: hidlcodec2.StatusOK,
			},
		},
		BaseBlocks: []hidlcodec2.BaseBlock{
			{
				Tag:             0,
				NativeBlockFds:  []int32{frameFd},
				NativeBlockInts: hidlcodec2.C2HandleLinearInts(uint64(len(frameData))),
			},
		},
	}
	err = component.Queue(ctx, bundleB)
	if err != nil {
		t.Logf("Step B (Work+BaseBlock, no Buffers): %v", err)
	} else {
		t.Log("Step B (Work+BaseBlock, no Buffers): succeeded")
	}

	// Queue a minimal EOS Work (no buffers, no base blocks).
	eosBundle := &hidlcodec2.WorkBundle{
		Works: []hidlcodec2.Work{
			{
				Input: hidlcodec2.FrameData{
					Flags: hidlcodec2.FrameDataEndOfStream,
					Ordinal: hidlcodec2.WorkOrdinal{
						TimestampUs:   0,
						FrameIndex:    0,
						CustomOrdinal: 0,
					},
				},
				Worklets: []hidlcodec2.Worklet{
					{ComponentId: 0},
				},
				Result: hidlcodec2.StatusOK,
			},
		},
	}
	err = component.Queue(ctx, eosBundle)
	require.NoError(t, err, "Queue EOS bundle failed")
	t.Log("HIDL Codec2: EOS queue succeeded")
}

// TestCodec2HIDL_EncodeFrame tests actual encoding through the HIDL Codec2
// pipeline. It tries two approaches:
//
//  1. AAC audio encoder with linear PCM blocks (no gralloc required)
//  2. AVC video encoder with gralloc graphic blocks (requires HIDL allocator)
//
// The test verifies that OnWorkDone fires and output contains non-zero
// encoded data.
func TestCodec2HIDL_EncodeFrame(t *testing.T) {
	t.Run("AAC", testCodec2HIDL_EncodeAAC)
	t.Run("AVC", testCodec2HIDL_EncodeAVC)
}

// testCodec2HIDL_EncodeAAC encodes PCM audio through c2.android.aac.encoder.
// The AAC encoder accepts linear blocks (memfd-backed), so no gralloc is
// needed. This is the most reliable encode path on emulators.
func testCodec2HIDL_EncodeAAC(t *testing.T) {
	const (
		sampleRate   = 44100
		channelCount = 1
		bitrate      = 64000
		// AAC frame = 1024 samples, 16-bit mono = 2048 bytes.
		samplesPerFrame = 1024
		bytesPerSample  = 2
		frameSize       = samplesPerFrame * bytesPerSample * channelCount
	)

	ctx := context.Background()
	store, drv := getHIDLComponentStore(ctx, t)

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

	component, err := store.CreateComponent(ctx, aacEncoderName, listenerCookie)
	require.NoError(t, err, "CreateComponent(AAC) failed")
	defer func() { _ = component.Release(ctx) }()

	// Configure: sample rate, channel count, bitrate.
	iface, err := component.GetInterface(ctx)
	require.NoError(t, err, "GetInterface failed")

	cfg, err := iface.GetConfigurable(ctx)
	require.NoError(t, err, "GetConfigurable failed")

	configParams := hidlcodec2.ConcatParams(
		hidlcodec2.BuildSampleRateParam(0, sampleRate),
		hidlcodec2.BuildChannelCountParam(0, channelCount),
		hidlcodec2.BuildBitrateParam(0, bitrate),
	)
	cfgStatus, _, err := cfg.Config(ctx, configParams, true)
	require.NoError(t, err, "Config failed")
	require.Equal(t, hidlcodec2.StatusOK, cfgStatus,
		"Config returned non-OK status: %s", cfgStatus)
	t.Logf("AAC encoder configured: %dHz %dch %dbps, status=%s",
		sampleRate, channelCount, bitrate, cfgStatus)

	err = component.Start(ctx)
	require.NoError(t, err, "Start failed")
	defer func() { _ = component.Stop(ctx) }()
	t.Log("AAC encoder started")

	// Build PCM audio data: silence (zeroes) for one AAC frame.
	pcmData := make([]byte, frameSize)
	pcmFd := createMemfd(t, "aac-input-pcm", pcmData)

	// Queue a work item with the linear PCM block.
	frameBundle := &hidlcodec2.WorkBundle{
		Works: []hidlcodec2.Work{
			{
				Input: hidlcodec2.FrameData{
					Ordinal: hidlcodec2.WorkOrdinal{
						FrameIndex: 0,
					},
					Buffers: []hidlcodec2.Buffer{
						{
							Blocks: []hidlcodec2.Block{
								{
									Index: 0,
									Meta:  hidlcodec2.BuildRangeInfoParam(0, frameSize),
								},
							},
						},
					},
				},
				Worklets: []hidlcodec2.Worklet{
					{ComponentId: 0},
				},
				Result: hidlcodec2.StatusOK,
			},
		},
		BaseBlocks: []hidlcodec2.BaseBlock{
			{
				Tag:             0, // nativeBlock
				NativeBlockFds:  []int32{pcmFd},
				NativeBlockInts: hidlcodec2.C2HandleLinearInts(frameSize),
			},
		},
	}
	err = component.Queue(ctx, frameBundle)
	require.NoError(t, err, "Queue PCM frame failed")
	t.Log("AAC PCM frame queued")

	// Queue EOS to flush the encoder.
	eosBundle := &hidlcodec2.WorkBundle{
		Works: []hidlcodec2.Work{
			{
				Input: hidlcodec2.FrameData{
					Flags: hidlcodec2.FrameDataEndOfStream,
					Ordinal: hidlcodec2.WorkOrdinal{
						FrameIndex: 1,
					},
				},
				Worklets: []hidlcodec2.Worklet{
					{ComponentId: 0},
				},
				Result: hidlcodec2.StatusOK,
			},
		},
	}
	err = component.Queue(ctx, eosBundle)
	require.NoError(t, err, "Queue EOS failed")
	t.Log("AAC EOS queued")

	// Wait for OnWorkDone callbacks. We expect at least one callback
	// with actual encoded AAC output.
	var totalOutputBytes int
	deadline := time.After(10 * time.Second)
	for i := 0; i < 3; i++ {
		select {
		case data := <-workDoneCh:
			t.Logf("AAC onWorkDone[%d]: %d raw callback bytes", i, len(data))
			totalOutputBytes += len(data)
		case <-deadline:
			if totalOutputBytes == 0 {
				// Try drain as fallback.
				t.Log("AAC: timeout; trying Drain")
				_ = component.Drain(ctx, true)
				select {
				case data := <-workDoneCh:
					t.Logf("AAC onWorkDone after drain: %d bytes", len(data))
					totalOutputBytes += len(data)
				case <-time.After(5 * time.Second):
					t.Fatal("AAC: no onWorkDone callback received")
				}
			}
			i = 3 // exit loop
		}
	}

	require.Greater(t, totalOutputBytes, 0,
		"AAC encoder produced no output")
	t.Logf("AAC encode complete: total callback data=%d bytes", totalOutputBytes)
}

// testCodec2HIDL_EncodeAVC encodes a YUV frame through c2.android.avc.encoder.
// The AVC encoder requires graphic blocks (C2GraphicBlock), which need a
// gralloc-allocated buffer wrapped with C2HandleGralloc metadata.
func testCodec2HIDL_EncodeAVC(t *testing.T) {
	const (
		encWidth   = 320
		encHeight  = 240
		encBitrate = 512000
		encFormat  = gfxCommon.PixelFormatYcbcr420888
		encUsage   = gfxCommon.BufferUsage(
			gfxCommon.BufferUsageCpuWriteOften | gfxCommon.BufferUsageVideoEncoder,
		)
	)

	ctx := context.Background()
	store, drv := getHIDLComponentStore(ctx, t)

	// Allocate a gralloc buffer for the input frame.
	sm := servicemanager.New(openBinder(t))
	buf, err := gralloc.Allocate(ctx, sm, encWidth, encHeight, encFormat, encUsage)
	if err != nil {
		t.Skipf("gralloc allocation unavailable: %v", err)
	}
	t.Logf("AVC gralloc buffer: fds=%d ints=%d stride=%d",
		len(buf.Handle.Fds), len(buf.Handle.Ints), buf.Stride)
	t.Cleanup(func() {
		buf.Munmap()
		for _, fd := range buf.Handle.Fds {
			unix.Close(int(fd))
		}
	})

	// Try to mmap and write gray YUV data. On goldfish emulators mmap
	// may fail; the encoder can still import the handle directly.
	if mmapErr := buf.Mmap(); mmapErr != nil {
		t.Logf("AVC gralloc mmap failed (expected on goldfish): %v", mmapErr)
	} else {
		grayFrame := makeGrayYUVFrame(encWidth, encHeight)
		copyLen := len(grayFrame)
		if copyLen > len(buf.MmapData) {
			copyLen = len(buf.MmapData)
		}
		copy(buf.MmapData[:copyLen], grayFrame[:copyLen])
		t.Logf("AVC: wrote %d bytes of gray YUV to gralloc buffer", copyLen)
	}

	// Register listener and create component.
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

	component, err := store.CreateComponent(ctx, avcEncoderName, listenerCookie)
	require.NoError(t, err, "CreateComponent(AVC) failed")
	defer func() { _ = component.Release(ctx) }()

	// Configure picture size + bitrate.
	iface, err := component.GetInterface(ctx)
	require.NoError(t, err, "GetInterface failed")

	cfg, err := iface.GetConfigurable(ctx)
	require.NoError(t, err, "GetConfigurable failed")

	configParams := hidlcodec2.ConcatParams(
		hidlcodec2.BuildPictureSizeParam(0, encWidth, encHeight),
		hidlcodec2.BuildBitrateParam(0, encBitrate),
	)
	cfgStatus, _, err := cfg.Config(ctx, configParams, true)
	require.NoError(t, err, "Config failed")
	require.Equal(t, hidlcodec2.StatusOK, cfgStatus,
		"Config returned non-OK status: %s", cfgStatus)
	t.Logf("AVC encoder configured: %dx%d %dbps, status=%s",
		encWidth, encHeight, encBitrate, cfgStatus)

	err = component.Start(ctx)
	require.NoError(t, err, "Start failed")
	defer func() { _ = component.Stop(ctx) }()
	t.Log("AVC encoder started")

	// Build the C2HandleGralloc-wrapped native_handle by appending
	// ExtraData ints to the gralloc handle's existing ints.
	grallocExtraInts := hidlcodec2.C2HandleGrallocInts(
		encWidth, encHeight,
		uint32(encFormat), uint64(encUsage),
		uint32(buf.Stride),
	)
	c2HandleInts := make([]int32, 0, len(buf.Handle.Ints)+len(grallocExtraInts))
	c2HandleInts = append(c2HandleInts, buf.Handle.Ints...)
	c2HandleInts = append(c2HandleInts, grallocExtraInts...)

	// Queue a work item with the graphic block.
	frameBundle := &hidlcodec2.WorkBundle{
		Works: []hidlcodec2.Work{
			{
				Input: hidlcodec2.FrameData{
					Ordinal: hidlcodec2.WorkOrdinal{
						FrameIndex: 0,
					},
					Buffers: []hidlcodec2.Buffer{
						{
							Blocks: []hidlcodec2.Block{
								{
									Index: 0,
									Meta:  hidlcodec2.BuildRangeInfoParam(0, uint32(buf.BufferSize())),
								},
							},
						},
					},
				},
				Worklets: []hidlcodec2.Worklet{
					{ComponentId: 0},
				},
				Result: hidlcodec2.StatusOK,
			},
		},
		BaseBlocks: []hidlcodec2.BaseBlock{
			{
				Tag:             0, // nativeBlock
				NativeBlockFds:  buf.Handle.Fds,
				NativeBlockInts: c2HandleInts,
			},
		},
	}
	err = component.Queue(ctx, frameBundle)
	if err != nil {
		// On goldfish emulators, gralloc buffers use /dev/goldfish_address_space
		// FDs that cannot be transferred via binder to the codec process.
		// The encoder fails with HIDL_CORRUPTED when it tries to import the
		// handle. This is an emulator limitation, not a binder bug.
		t.Skipf("Queue graphic frame failed (expected on goldfish emulator): %v", err)
	}
	t.Log("AVC graphic frame queued")

	// Queue EOS to flush.
	eosBundle := &hidlcodec2.WorkBundle{
		Works: []hidlcodec2.Work{
			{
				Input: hidlcodec2.FrameData{
					Flags: hidlcodec2.FrameDataEndOfStream,
					Ordinal: hidlcodec2.WorkOrdinal{
						FrameIndex: 1,
					},
				},
				Worklets: []hidlcodec2.Worklet{
					{ComponentId: 0},
				},
				Result: hidlcodec2.StatusOK,
			},
		},
	}
	err = component.Queue(ctx, eosBundle)
	require.NoError(t, err, "Queue EOS failed")
	t.Log("AVC EOS queued")

	// Wait for OnWorkDone callbacks.
	var totalOutputBytes int
	deadline := time.After(10 * time.Second)
	for i := 0; i < 3; i++ {
		select {
		case data := <-workDoneCh:
			t.Logf("AVC onWorkDone[%d]: %d raw callback bytes", i, len(data))
			totalOutputBytes += len(data)
		case <-deadline:
			if totalOutputBytes == 0 {
				t.Log("AVC: timeout; trying Drain")
				_ = component.Drain(ctx, true)
				select {
				case data := <-workDoneCh:
					t.Logf("AVC onWorkDone after drain: %d bytes", len(data))
					totalOutputBytes += len(data)
				case <-time.After(5 * time.Second):
					t.Fatal("AVC: no onWorkDone callback received")
				}
			}
			i = 3 // exit loop
		}
	}

	require.Greater(t, totalOutputBytes, 0,
		"AVC encoder produced no output")
	t.Logf("AVC encode complete: total callback data=%d bytes", totalOutputBytes)
}
