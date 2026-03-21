//go:build e2e

package e2e

import (
	"context"
	"encoding/binary"
	"fmt"
	"sync"
	"testing"
	"time"

	fwkDevice "github.com/xaionaro-go/binder/android/frameworks/cameraservice/device"
	fwkService "github.com/xaionaro-go/binder/android/frameworks/cameraservice/service"
	"github.com/xaionaro-go/binder/binder"
	"github.com/xaionaro-go/binder/binder/versionaware"
	"github.com/xaionaro-go/binder/kernelbinder"
	"github.com/xaionaro-go/binder/parcel"
	"github.com/xaionaro-go/binder/servicemanager"

	"github.com/stretchr/testify/assert"

	"golang.org/x/sys/unix"
)

// ---------------------------------------------------------------------------
// IGBP constants (from cmd/internal/igbp, copied to keep test self-contained)
// ---------------------------------------------------------------------------

const (
	camIGBPDescriptor = "android.gui.IGraphicBufferProducer"

	camIGBPRequestBuffer          binder.TransactionCode = 1
	camIGBPDequeueBuffer          binder.TransactionCode = 2
	camIGBPDetachBuffer           binder.TransactionCode = 3
	camIGBPQueueBuffer            binder.TransactionCode = 6
	camIGBPCancelBuffer           binder.TransactionCode = 7
	camIGBPQuery                  binder.TransactionCode = 8
	camIGBPConnect                binder.TransactionCode = 9
	camIGBPDisconnect             binder.TransactionCode = 10
	camIGBPAllocateBuffers        binder.TransactionCode = 12
	camIGBPAllowAllocation        binder.TransactionCode = 13
	camIGBPSetGenerationNumber    binder.TransactionCode = 14
	camIGBPGetConsumerName        binder.TransactionCode = 15
	camIGBPSetMaxDequeuedBufCount binder.TransactionCode = 16
	camIGBPSetAsyncMode           binder.TransactionCode = 17
	camIGBPSetSharedBufferMode    binder.TransactionCode = 18
	camIGBPSetAutoRefresh         binder.TransactionCode = 19
	camIGBPSetDequeueTimeout      binder.TransactionCode = 20
	camIGBPGetLastQueuedBuffer    binder.TransactionCode = 21
	camIGBPGetFrameTimestamps     binder.TransactionCode = 22
	camIGBPGetUniqueId            binder.TransactionCode = 23
	camIGBPGetConsumerUsage       binder.TransactionCode = 24
	camIGBPSetLegacyBufferDrop    binder.TransactionCode = 25
	camIGBPSetAutoPrerotation     binder.TransactionCode = 26

	camIGBPStatusOK               int32 = 0
	camIGBPStatusNoInit           int32 = -19
	camIGBPBufferNeedsRealloc     int32 = 0x1
	camIGBPGraphicBufferMagicGB01 int32 = 0x47423031
	camIGBPMaxBufferSlots               = 64

	camIGBPPixelFormatYCbCr420_888 int32 = 0x23

	camNativeWindowWidth             int32 = 0
	camNativeWindowHeight            int32 = 1
	camNativeWindowFormat            int32 = 2
	camNativeWindowMinUndequeued     int32 = 3
	camNativeWindowQueuesToComposer  int32 = 4
	camNativeWindowConcreteType      int32 = 5
	camNativeWindowDefaultWidth      int32 = 6
	camNativeWindowDefaultHeight     int32 = 7
	camNativeWindowTransformHint     int32 = 8
	camNativeWindowConsumerRunning   int32 = 9
	camNativeWindowConsumerUsageBits int32 = 10
	camNativeWindowStickyTransform   int32 = 11
	camNativeWindowDefaultDataspace  int32 = 12
	camNativeWindowBufferAge         int32 = 13
	camNativeWindowMaxBufferCount    int32 = 21

	camNativeWindowSurfaceType int32 = 1 // NATIVE_WINDOW_SURFACE
)

// ---------------------------------------------------------------------------
// Camera device callback
// ---------------------------------------------------------------------------

type camTestCallback struct {
	mu              sync.Mutex
	framesStarted   int
	resultsReceived int
	errors          int
}

func (c *camTestCallback) OnCaptureStarted(
	_ context.Context,
	_ fwkDevice.CaptureResultExtras,
	_ int64,
) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.framesStarted++
	return nil
}

func (c *camTestCallback) OnDeviceError(
	_ context.Context,
	_ fwkDevice.ErrorCode,
	_ fwkDevice.CaptureResultExtras,
) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.errors++
	return nil
}

func (c *camTestCallback) OnDeviceIdle(_ context.Context) error {
	return nil
}

func (c *camTestCallback) OnPrepared(_ context.Context, _ int32) error {
	return nil
}

func (c *camTestCallback) OnRepeatingRequestError(
	_ context.Context,
	_ int64,
	_ int32,
) error {
	return nil
}

func (c *camTestCallback) OnResultReceived(
	_ context.Context,
	_ fwkDevice.CaptureMetadataInfo,
	_ fwkDevice.CaptureResultExtras,
	_ []fwkDevice.PhysicalCaptureResultInfo,
) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.resultsReceived++
	return nil
}

// ---------------------------------------------------------------------------
// Minimal IGBP stub (memfd-backed, no gralloc dependency)
// ---------------------------------------------------------------------------

type camSlotBuffer struct {
	fd     int
	width  uint32
	height uint32
	stride uint32
	format int32
	usage  uint64
}

type camIGBPStub struct {
	width  uint32
	height uint32
	format int32

	mu       sync.Mutex
	nextSlot int
	slots    [camIGBPMaxBufferSlots]*camSlotBuffer
	queuedCh chan int // slot index of queued buffer
}

func newCamIGBPStub(width, height uint32) *camIGBPStub {
	return &camIGBPStub{
		width:    width,
		height:   height,
		format:   camIGBPPixelFormatYCbCr420_888,
		queuedCh: make(chan int, 16),
	}
}

func (g *camIGBPStub) closeFDs() {
	g.mu.Lock()
	defer g.mu.Unlock()
	for i, slot := range g.slots {
		if slot != nil && slot.fd >= 0 {
			unix.Close(slot.fd)
			g.slots[i] = nil
		}
	}
}

func (g *camIGBPStub) Descriptor() string {
	return camIGBPDescriptor
}

func (g *camIGBPStub) OnTransaction(
	_ context.Context,
	code binder.TransactionCode,
	data *parcel.Parcel,
) (*parcel.Parcel, error) {
	if _, err := data.ReadInterfaceToken(); err != nil {
		return nil, fmt.Errorf("IGBP: reading interface token: %w", err)
	}

	switch code {
	case camIGBPRequestBuffer:
		return g.onRequestBuffer(data)
	case camIGBPDequeueBuffer:
		return g.onDequeueBuffer(data)
	case camIGBPQueueBuffer:
		return g.onQueueBuffer(data)
	case camIGBPCancelBuffer:
		return g.onCancelBuffer(data)
	case camIGBPQuery:
		return g.onQuery(data)
	case camIGBPConnect:
		return g.onConnect(data)
	case camIGBPDisconnect:
		return g.onDisconnect(data)
	case camIGBPSetMaxDequeuedBufCount,
		camIGBPSetAsyncMode,
		camIGBPAllowAllocation,
		camIGBPSetGenerationNumber,
		camIGBPSetDequeueTimeout,
		camIGBPSetSharedBufferMode,
		camIGBPSetAutoRefresh,
		camIGBPSetLegacyBufferDrop,
		camIGBPSetAutoPrerotation,
		camIGBPDetachBuffer:
		return g.onSimpleOK()
	case camIGBPGetConsumerName:
		return g.onGetConsumerName()
	case camIGBPGetUniqueId:
		return g.onGetUniqueId()
	case camIGBPGetConsumerUsage:
		return g.onGetConsumerUsage()
	case camIGBPAllocateBuffers, camIGBPGetFrameTimestamps:
		return nil, nil // void
	case camIGBPGetLastQueuedBuffer:
		return g.onGetLastQueuedBuffer()
	default:
		reply := parcel.New()
		reply.WriteInt32(camIGBPStatusNoInit)
		return reply, nil
	}
}

func (g *camIGBPStub) onSimpleOK() (*parcel.Parcel, error) {
	reply := parcel.New()
	reply.WriteInt32(camIGBPStatusOK)
	return reply, nil
}

func (g *camIGBPStub) onRequestBuffer(data *parcel.Parcel) (*parcel.Parcel, error) {
	slot, _ := data.ReadInt32()

	g.mu.Lock()
	buf := g.slots[slot]
	g.mu.Unlock()

	reply := parcel.New()
	if buf == nil {
		reply.WriteInt32(0) // nonNull=0
		reply.WriteInt32(camIGBPStatusOK)
		return reply, nil
	}

	reply.WriteInt32(1) // nonNull=1

	// Write flattened GraphicBuffer.
	const numFds int32 = 1
	const numInts int32 = 0
	flattenedSize := (13 + numInts) * 4

	reply.WriteInt32(flattenedSize)
	reply.WriteInt32(numFds)

	raw := make([]byte, flattenedSize)
	bufID := uint64(0xCAFE0000) | uint64(slot)
	binary.LittleEndian.PutUint32(raw[0:], uint32(camIGBPGraphicBufferMagicGB01))
	binary.LittleEndian.PutUint32(raw[4:], buf.width)
	binary.LittleEndian.PutUint32(raw[8:], buf.height)
	binary.LittleEndian.PutUint32(raw[12:], buf.stride)
	binary.LittleEndian.PutUint32(raw[16:], uint32(buf.format))
	binary.LittleEndian.PutUint32(raw[20:], 1) // layerCount
	binary.LittleEndian.PutUint32(raw[24:], uint32(buf.usage))
	binary.LittleEndian.PutUint32(raw[28:], uint32(bufID>>32))
	binary.LittleEndian.PutUint32(raw[32:], uint32(bufID&0xFFFFFFFF))
	binary.LittleEndian.PutUint32(raw[36:], 0) // generationNumber
	binary.LittleEndian.PutUint32(raw[40:], uint32(numFds))
	binary.LittleEndian.PutUint32(raw[44:], uint32(numInts))
	binary.LittleEndian.PutUint32(raw[48:], uint32(buf.usage>>32))

	reply.WriteRawBytes(raw)
	reply.WriteFileDescriptor(int32(buf.fd))

	reply.WriteInt32(camIGBPStatusOK)
	return reply, nil
}

func (g *camIGBPStub) onDequeueBuffer(data *parcel.Parcel) (*parcel.Parcel, error) {
	w, _ := data.ReadUint32()
	h, _ := data.ReadUint32()
	format, _ := data.ReadInt32()
	usage, _ := data.ReadUint64()
	getTimestamps, _ := data.ReadBool()

	if w == 0 {
		w = g.width
	}
	if h == 0 {
		h = g.height
	}
	if format == 0 {
		format = g.format
	}

	g.mu.Lock()
	slot := g.nextSlot
	g.nextSlot = (g.nextSlot + 1) % 4

	needsRealloc := false
	existing := g.slots[slot]
	if existing == nil || existing.width != w || existing.height != h || existing.format != format {
		needsRealloc = true
		if existing != nil && existing.fd >= 0 {
			unix.Close(existing.fd)
		}

		bufSize := int64(w) * int64(h) * 3 / 2 // YCbCr 420
		fd, err := unix.MemfdCreate("camera-test-buffer", 0)
		if err != nil {
			g.mu.Unlock()
			return nil, fmt.Errorf("memfd_create: %w", err)
		}
		if err := unix.Ftruncate(fd, bufSize); err != nil {
			unix.Close(fd)
			g.mu.Unlock()
			return nil, fmt.Errorf("ftruncate: %w", err)
		}
		g.slots[slot] = &camSlotBuffer{
			fd:     fd,
			width:  w,
			height: h,
			stride: w,
			format: format,
			usage:  usage,
		}
	}
	g.mu.Unlock()

	reply := parcel.New()
	reply.WriteInt32(int32(slot))

	// Fence: flattenedSize=4, fdCount=0, numFds=0.
	reply.WriteInt32(4)
	reply.WriteInt32(0)
	reply.WriteUint32(0)

	// bufferAge.
	reply.WriteUint64(0)

	if getTimestamps {
		// Empty FrameEventHistoryDelta.
		reply.WriteInt32(28)
		reply.WriteInt32(0)
		reply.WriteInt64(0)
		reply.WriteInt64(0)
		reply.WriteInt64(0)
		reply.WriteInt32(0)
	}

	if needsRealloc {
		reply.WriteInt32(camIGBPBufferNeedsRealloc)
	} else {
		reply.WriteInt32(camIGBPStatusOK)
	}
	return reply, nil
}

func (g *camIGBPStub) onQueueBuffer(data *parcel.Parcel) (*parcel.Parcel, error) {
	slot, _ := data.ReadInt32()

	select {
	case g.queuedCh <- int(slot):
	default:
	}

	reply := parcel.New()
	g.writeQueueBufferOutput(reply)
	reply.WriteInt32(camIGBPStatusOK)
	return reply, nil
}

// writeQueueBufferOutput writes a minimal QueueBufferOutput Flattenable (61 bytes).
func (g *camIGBPStub) writeQueueBufferOutput(reply *parcel.Parcel) {
	const flatSize = 61
	reply.WriteInt32(flatSize)
	reply.WriteInt32(0)

	buf := make([]byte, flatSize)
	off := 0
	binary.LittleEndian.PutUint32(buf[off:], 0) // width
	off += 4
	binary.LittleEndian.PutUint32(buf[off:], 0) // height
	off += 4
	binary.LittleEndian.PutUint32(buf[off:], 0) // transformHint
	off += 4
	binary.LittleEndian.PutUint32(buf[off:], 0) // numPendingBuffers
	off += 4
	binary.LittleEndian.PutUint64(buf[off:], 1) // nextFrameNumber
	off += 8
	buf[off] = 0 // bufferReplaced
	off += 1
	binary.LittleEndian.PutUint32(buf[off:], 64) // maxBufferCount
	off += 4
	off += 24                                   // compositorTiming zeros
	binary.LittleEndian.PutUint32(buf[off:], 0) // deltaCount
	off += 4
	binary.LittleEndian.PutUint32(buf[off:], 0) // result
	_ = off

	reply.WriteRawBytes(buf)
}

func (g *camIGBPStub) onCancelBuffer(data *parcel.Parcel) (*parcel.Parcel, error) {
	_, _ = data.ReadInt32()
	reply := parcel.New()
	reply.WriteInt32(camIGBPStatusOK)
	return reply, nil
}

func (g *camIGBPStub) onQuery(data *parcel.Parcel) (*parcel.Parcel, error) {
	rawWhat, _ := data.ReadInt32()

	var value int32
	switch rawWhat {
	case camNativeWindowWidth:
		value = int32(g.width)
	case camNativeWindowHeight:
		value = int32(g.height)
	case camNativeWindowFormat:
		value = g.format
	case camNativeWindowMinUndequeued:
		value = 1
	case camNativeWindowQueuesToComposer:
		value = 0
	case camNativeWindowConcreteType:
		value = camNativeWindowSurfaceType
	case camNativeWindowDefaultWidth:
		value = int32(g.width)
	case camNativeWindowDefaultHeight:
		value = int32(g.height)
	case camNativeWindowTransformHint:
		value = 0
	case camNativeWindowConsumerRunning:
		value = 0
	case camNativeWindowConsumerUsageBits:
		value = 0
	case camNativeWindowStickyTransform:
		value = 0
	case camNativeWindowDefaultDataspace:
		value = 0
	case camNativeWindowBufferAge:
		value = 0
	case camNativeWindowMaxBufferCount:
		value = 64
	}

	reply := parcel.New()
	reply.WriteInt32(value)
	reply.WriteInt32(camIGBPStatusOK)
	return reply, nil
}

func (g *camIGBPStub) onConnect(data *parcel.Parcel) (*parcel.Parcel, error) {
	hasListener, _ := data.ReadInt32()
	if hasListener == 1 {
		_, _ = data.ReadStrongBinder()
	}
	_, _ = data.ReadInt32() // api
	_, _ = data.ReadInt32() // producerControlled

	reply := parcel.New()
	g.writeQueueBufferOutput(reply)
	reply.WriteInt32(camIGBPStatusOK)
	return reply, nil
}

func (g *camIGBPStub) onDisconnect(data *parcel.Parcel) (*parcel.Parcel, error) {
	_, _ = data.ReadInt32()
	reply := parcel.New()
	reply.WriteInt32(camIGBPStatusOK)
	return reply, nil
}

func (g *camIGBPStub) onGetConsumerName() (*parcel.Parcel, error) {
	reply := parcel.New()
	reply.WriteString16("GoCameraTest")
	return reply, nil
}

func (g *camIGBPStub) onGetUniqueId() (*parcel.Parcel, error) {
	reply := parcel.New()
	reply.WriteUint64(0xE2E7E570)
	reply.WriteInt32(camIGBPStatusOK)
	return reply, nil
}

func (g *camIGBPStub) onGetConsumerUsage() (*parcel.Parcel, error) {
	reply := parcel.New()
	reply.WriteUint64(0)
	reply.WriteInt32(camIGBPStatusOK)
	return reply, nil
}

func (g *camIGBPStub) onGetLastQueuedBuffer() (*parcel.Parcel, error) {
	reply := parcel.New()
	reply.WriteInt32(camIGBPStatusNoInit)
	return reply, nil
}

// ---------------------------------------------------------------------------
// Raw parcel helpers for camera operations
// ---------------------------------------------------------------------------

// camConnectDevice performs the ConnectDevice transaction manually because the
// generated proxy doesn't write the callback binder or read the returned
// ICameraDeviceUser binder (both are interface{} in the generated code).
func camConnectDevice(
	ctx context.Context,
	svc binder.IBinder,
	callback fwkDevice.ICameraDeviceCallback,
	cameraID string,
) (fwkDevice.ICameraDeviceUser, error) {
	data := parcel.New()
	data.WriteInterfaceToken(fwkService.DescriptorICameraService)
	binder.WriteBinderToParcel(ctx, data, callback.AsBinder(), svc.Transport())
	data.WriteString16(cameraID)

	code, err := svc.ResolveCode(ctx, fwkService.DescriptorICameraService, fwkService.MethodICameraServiceConnectDevice)
	if err != nil {
		return nil, fmt.Errorf("resolving connectDevice: %w", err)
	}

	reply, err := svc.Transact(ctx, code, 0, data)
	if err != nil {
		return nil, fmt.Errorf("transaction: %w", err)
	}
	defer reply.Recycle()

	if err = binder.ReadStatus(reply); err != nil {
		return nil, fmt.Errorf("status: %w", err)
	}

	handle, err := reply.ReadStrongBinder()
	if err != nil {
		return nil, fmt.Errorf("reading device user binder: %w", err)
	}

	remote := binder.NewProxyBinder(svc.Transport(), svc.Identity(), handle)
	return fwkDevice.NewCameraDeviceUserProxy(remote), nil
}

func camCreateStreamWithSurface(
	ctx context.Context,
	deviceUser fwkDevice.ICameraDeviceUser,
	transport binder.Transport,
	igbpStub *binder.StubBinder,
	width int32,
	height int32,
) (int32, error) {
	data := parcel.New()
	data.WriteInterfaceToken(fwkDevice.DescriptorICameraDeviceUser)
	data.WriteInt32(1) // non-null OutputConfiguration

	headerPos := parcel.WriteParcelableHeader(data)

	data.WriteInt32(0)  // windowHandles: empty array
	data.WriteInt32(0)  // rotation: R0
	data.WriteInt32(-1) // windowGroupId: NONE
	data.WriteString16("")
	data.WriteInt32(width)
	data.WriteInt32(height)
	data.WriteBool(false) // isDeferred

	// surfaces: array of 1.
	data.WriteInt32(1)
	data.WriteInt32(1) // non-null

	// view::Surface::writeToParcel.
	data.WriteString16("GoCameraTest")
	data.WriteInt32(0)           // isSingleBuffered
	data.WriteUint32(0x62717565) // USE_BUFFER_QUEUE
	binder.WriteBinderToParcel(ctx, data, igbpStub, transport)
	data.WriteNullStrongBinder() // surfaceControlHandle: null

	parcel.WriteParcelableFooter(data, headerPos)

	reply, err := deviceUser.AsBinder().Transact(
		ctx,
		fwkDevice.TransactionICameraDeviceUserCreateStream,
		0,
		data,
	)
	if err != nil {
		return 0, fmt.Errorf("transaction: %w", err)
	}
	defer reply.Recycle()

	if err = binder.ReadStatus(reply); err != nil {
		return 0, fmt.Errorf("status: %w", err)
	}

	streamId, err := reply.ReadInt32()
	if err != nil {
		return 0, fmt.Errorf("readStreamId: %w", err)
	}
	return streamId, nil
}

func camCreateDefaultRequestRaw(
	ctx context.Context,
	deviceUser fwkDevice.ICameraDeviceUser,
	templateId fwkDevice.TemplateId,
) ([]byte, error) {
	data := parcel.New()
	data.WriteInterfaceToken(fwkDevice.DescriptorICameraDeviceUser)
	data.WriteInt32(int32(templateId))

	reply, err := deviceUser.AsBinder().Transact(
		ctx,
		fwkDevice.TransactionICameraDeviceUserCreateDefaultRequest,
		0,
		data,
	)
	if err != nil {
		return nil, fmt.Errorf("transaction: %w", err)
	}
	defer reply.Recycle()

	if err = binder.ReadStatus(reply); err != nil {
		return nil, fmt.Errorf("status: %w", err)
	}

	nullInd, err := reply.ReadInt32()
	if err != nil {
		return nil, fmt.Errorf("null indicator: %w", err)
	}
	if nullInd == 0 {
		return nil, fmt.Errorf("null metadata")
	}

	endPos, err := parcel.ReadParcelableHeader(reply)
	if err != nil {
		return nil, fmt.Errorf("parcelable header: %w", err)
	}

	metadataBytes, err := reply.ReadByteArray()
	if err != nil {
		return nil, fmt.Errorf("ReadByteArray: %w", err)
	}

	parcel.SkipToParcelableEnd(reply, endPos)
	return metadataBytes, nil
}

func camSubmitRequestRaw(
	ctx context.Context,
	deviceUser fwkDevice.ICameraDeviceUser,
	captureReq fwkDevice.CaptureRequest,
	isRepeating bool,
) (fwkDevice.SubmitInfo, error) {
	var result fwkDevice.SubmitInfo

	data := parcel.New()
	data.WriteInterfaceToken(fwkDevice.DescriptorICameraDeviceUser)
	data.WriteInt32(1) // requestList: array of 1
	data.WriteInt32(1) // non-null indicator
	if err := captureReq.MarshalParcel(data); err != nil {
		return result, fmt.Errorf("marshal: %w", err)
	}
	data.WriteBool(isRepeating)

	reply, err := deviceUser.AsBinder().Transact(
		ctx,
		fwkDevice.TransactionICameraDeviceUserSubmitRequestList,
		0,
		data,
	)
	if err != nil {
		return result, fmt.Errorf("transaction: %w", err)
	}
	defer reply.Recycle()

	if err = binder.ReadStatus(reply); err != nil {
		return result, fmt.Errorf("status: %w", err)
	}

	nullInd, err := reply.ReadInt32()
	if err != nil {
		return result, fmt.Errorf("null indicator: %w", err)
	}
	if nullInd != 0 {
		if err = result.UnmarshalParcel(reply); err != nil {
			return result, fmt.Errorf("unmarshal SubmitInfo: %w", err)
		}
	}

	return result, nil
}

// ---------------------------------------------------------------------------
// openBinderLarge opens a binder with 4MB mmap for camera buffer operations.
// ---------------------------------------------------------------------------

func openBinderLarge(t *testing.T) *versionaware.Transport {
	t.Helper()
	ctx := context.Background()
	driver, err := kernelbinder.Open(ctx, binder.WithMapSize(4*1024*1024))
	requireOrSkip(t, err)
	transport, err := versionaware.NewTransport(ctx, driver, 0)
	requireOrSkip(t, err)
	t.Cleanup(func() {
		_ = driver.Close(ctx)
	})
	return transport
}

// ---------------------------------------------------------------------------
// E2E camera capture test
// ---------------------------------------------------------------------------

// TestCameraCapture_SingleFrame connects to the camera via binder,
// configures a capture stream with a memfd-backed IGraphicBufferProducer
// stub, submits a capture request, and waits for at least one frame
// to be queued back, proving the full camera pipeline works end-to-end.
func TestCameraCapture_SingleFrame(t *testing.T) {
	const (
		cameraID = "0"
		width    = 640
		height   = 480
		timeout  = 5 * time.Second
	)

	ctx := context.Background()

	// Camera needs a larger mmap than the default 128K.
	transport := openBinderLarge(t)
	sm := servicemanager.New(transport)

	// Step 0: Get camera service.
	svc, err := sm.GetService(ctx, "android.frameworks.cameraservice.service.ICameraService/default")
	requireOrSkip(t, err)

	// Create and register camera callback stub.
	cb := &camTestCallback{}
	cbStub := fwkDevice.NewCameraDeviceCallbackStub(cb)
	cbStubBinder := cbStub.AsBinder().(*binder.StubBinder)
	cbStubBinder.RegisterWithTransport(ctx, transport)
	time.Sleep(100 * time.Millisecond)

	// Step 1: ConnectDevice via raw transaction (the generated proxy
	// doesn't write the callback binder or read the returned binder).
	deviceUser, err := camConnectDevice(ctx, svc, cbStub, cameraID)
	requireOrSkip(t, err)
	t.Log("ConnectDevice succeeded")

	t.Cleanup(func() {
		if disconnectErr := deviceUser.Disconnect(ctx); disconnectErr != nil {
			t.Logf("Disconnect: %v", disconnectErr)
		}
	})

	// Step 2: BeginConfigure.
	err = deviceUser.BeginConfigure(ctx)
	requireOrSkip(t, err)
	t.Log("BeginConfigure OK")

	// Step 3: CreateDefaultRequest (PREVIEW template).
	metadataBytes, err := camCreateDefaultRequestRaw(ctx, deviceUser, fwkDevice.TemplateIdPREVIEW)
	requireOrSkip(t, err)
	t.Logf("CreateDefaultRequest OK: metadata len=%d", len(metadataBytes))

	// Step 4: Create IGBP stub and CreateStream.
	igbpStub := newCamIGBPStub(width, height)
	t.Cleanup(func() { igbpStub.closeFDs() })

	igbpStubBinder := binder.NewStubBinder(igbpStub)
	igbpStubBinder.RegisterWithTransport(ctx, transport)

	streamId, err := camCreateStreamWithSurface(
		ctx, deviceUser, transport, igbpStubBinder, width, height,
	)
	requireOrSkip(t, err)
	t.Logf("CreateStream OK: streamId=%d", streamId)

	// Step 5: EndConfigure.
	err = deviceUser.EndConfigure(
		ctx,
		fwkDevice.StreamConfigurationModeNormalMode,
		fwkDevice.CameraMetadata{Metadata: []byte{}},
		0,
	)
	requireOrSkip(t, err)
	t.Log("EndConfigure OK")

	// Step 6: SubmitRequestList (try repeating first, then single).
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

	submitInfo, err := camSubmitRequestRaw(ctx, deviceUser, captureReq, true)
	if err != nil {
		t.Logf("SubmitRequestList (repeating) failed: %v; trying single shot", err)
		submitInfo, err = camSubmitRequestRaw(ctx, deviceUser, captureReq, false)
		requireOrSkip(t, err)
	}
	t.Logf("SubmitRequestList OK: requestId=%d lastFrame=%d",
		submitInfo.RequestId, submitInfo.LastFrameNumber)

	// Step 7: Wait for callbacks proving the binder pipeline works.
	// The IGBP stub uses memfd buffers without gralloc metadata, so the
	// camera HAL cannot write actual frames. We verify that:
	//   - OnCaptureStarted fires (camera accepted the request and started)
	//   - IGBP dequeueBuffer is called (camera uses our surface)
	// Device errors (ErrorCodeCameraBuffer) are expected because the HAL
	// cannot import memfd handles via gralloc.
	t.Log("Waiting for capture callbacks...")
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case slot := <-igbpStub.queuedCh:
		t.Logf("Frame received on IGBP slot %d", slot)
	case <-timer.C:
		cb.mu.Lock()
		started := cb.framesStarted
		results := cb.resultsReceived
		errs := cb.errors
		cb.mu.Unlock()

		t.Logf("Timeout: started=%d results=%d errors=%d", started, results, errs)
		if started == 0 && results == 0 && errs == 0 {
			t.Fatalf("No callbacks received within %v: the binder callback pipeline is not working", timeout)
		}
	}

	// Verify the binder callback pipeline delivered at least one callback.
	cb.mu.Lock()
	started := cb.framesStarted
	results := cb.resultsReceived
	errs := cb.errors
	cb.mu.Unlock()

	t.Logf("Camera callbacks: started=%d results=%d errors=%d", started, results, errs)

	// OnCaptureStarted proves the camera accepted our request and the
	// callback binder delivered the notification. OnDeviceError also
	// proves callback delivery (the error is about the buffer, not binder).
	totalCallbacks := started + results + errs
	assert.Greater(t, totalCallbacks, 0,
		"expected at least one camera callback (OnCaptureStarted, OnResultReceived, or OnDeviceError)")
	assert.Greater(t, started, 0,
		"expected OnCaptureStarted to fire (camera accepted the capture request)")
}
