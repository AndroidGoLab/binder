package main

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"sync"
	"time"

	fwkDevice "github.com/xaionaro-go/binder/android/frameworks/cameraservice/device"
	fwkService "github.com/xaionaro-go/binder/android/frameworks/cameraservice/service"
	"github.com/xaionaro-go/binder/binder"
	"github.com/xaionaro-go/binder/binder/versionaware"
	"github.com/xaionaro-go/binder/cmd/internal/igbp"
	"github.com/xaionaro-go/binder/kernelbinder"
	"github.com/xaionaro-go/binder/parcel"
	"github.com/xaionaro-go/binder/servicemanager"

	"github.com/facebookincubator/go-belt/tool/logger"
	"github.com/facebookincubator/go-belt/tool/logger/implementation/logrus"

	"golang.org/x/sys/unix"
)

// --------------------------------------------------------------------
// Camera device callbacks
// --------------------------------------------------------------------

type cameraDeviceCallback struct {
	mu             sync.Mutex
	framesReceived int
}

func (c *cameraDeviceCallback) OnCaptureStarted(
	_ context.Context,
	extras fwkDevice.CaptureResultExtras,
	ts int64,
) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.framesReceived++
	fmt.Printf("  >> OnCaptureStarted: requestId=%d timestamp=%d (total=%d)\n",
		extras.RequestId, ts, c.framesReceived)
	return nil
}

func (c *cameraDeviceCallback) OnDeviceError(
	_ context.Context,
	code fwkDevice.ErrorCode,
	extras fwkDevice.CaptureResultExtras,
) error {
	fmt.Printf("  >> OnDeviceError: code=%d requestId=%d\n", code, extras.RequestId)
	return nil
}

func (c *cameraDeviceCallback) OnDeviceIdle(_ context.Context) error {
	fmt.Println("  >> OnDeviceIdle")
	return nil
}

func (c *cameraDeviceCallback) OnPrepared(_ context.Context, streamId int32) error {
	fmt.Printf("  >> OnPrepared: stream %d\n", streamId)
	return nil
}

func (c *cameraDeviceCallback) OnRepeatingRequestError(
	_ context.Context,
	lastFrame int64,
	reqId int32,
) error {
	fmt.Printf("  >> OnRepeatingRequestError: frame=%d req=%d\n", lastFrame, reqId)
	return nil
}

func (c *cameraDeviceCallback) OnResultReceived(
	_ context.Context,
	meta fwkDevice.CaptureMetadataInfo,
	extras fwkDevice.CaptureResultExtras,
	_ []fwkDevice.PhysicalCaptureResultInfo,
) error {
	fmt.Printf("  >> OnResultReceived: requestId=%d frameNumber=%d tag=%d\n",
		extras.RequestId, extras.FrameNumber, meta.Tag)
	return nil
}

func (c *cameraDeviceCallback) OnClientSharedAccessPriorityChanged(_ context.Context, primary bool) error {
	fmt.Printf("  >> OnClientSharedAccessPriorityChanged: %v\n", primary)
	return nil
}

// --------------------------------------------------------------------
// IGraphicBufferProducer stub (native binder, non-AIDL)
// --------------------------------------------------------------------


// slotBuffer holds per-slot buffer state.
type slotBuffer struct {
	fd     int    // memfd backing the buffer
	width  uint32
	height uint32
	stride uint32
	format int32
	usage  uint64
}

// GraphicBufferProducerStub implements a minimal IGraphicBufferProducer
// that satisfies the camera service's output configuration requirements.
type GraphicBufferProducerStub struct {
	width  uint32
	height uint32
	format int32

	mu       sync.Mutex
	nextSlot int
	slots    [igbp.MaxBufferSlots]*slotBuffer
	queuedCh chan struct{}
}

func NewGraphicBufferProducerStub(
	width uint32,
	height uint32,
) *GraphicBufferProducerStub {
	return &GraphicBufferProducerStub{
		width:    width,
		height:   height,
		format:   int32(igbp.PixelFormatYCbCr420_888),
		queuedCh: make(chan struct{}, 16),
	}
}

func (g *GraphicBufferProducerStub) Descriptor() string {
	return igbp.Descriptor
}

func (g *GraphicBufferProducerStub) OnTransaction(
	_ context.Context,
	code binder.TransactionCode,
	data *parcel.Parcel,
) (*parcel.Parcel, error) {
	// Read and discard the interface token (CHECK_INTERFACE equivalent).
	// The native binder format has: int32(strictMode) + int32(workSource) +
	// int32(vendorHeader) + String16(descriptor).
	if _, err := data.ReadInterfaceToken(); err != nil {
		return nil, fmt.Errorf("IGBP: reading interface token: %w", err)
	}

	switch code {
	case igbp.RequestBuffer:
		return g.onRequestBuffer(data)
	case igbp.DequeueBuffer:
		return g.onDequeueBuffer(data)
	case igbp.QueueBuffer:
		return g.onQueueBuffer(data)
	case igbp.CancelBuffer:
		return g.onCancelBuffer(data)
	case igbp.Query:
		return g.onQuery(data)
	case igbp.Connect:
		return g.onConnect(data)
	case igbp.Disconnect:
		return g.onDisconnect(data)
	case igbp.SetMaxDequeuedBufCount:
		return g.onSimpleInt32Reply("setMaxDequeuedBufferCount", data)
	case igbp.SetAsyncMode:
		return g.onSimpleInt32Reply("setAsyncMode", data)
	case igbp.AllowAllocation:
		return g.onSimpleInt32Reply("allowAllocation", data)
	case igbp.SetGenerationNumber:
		return g.onSimpleInt32Reply("setGenerationNumber", data)
	case igbp.SetDequeueTimeout:
		return g.onSimpleInt32Reply("setDequeueTimeout", data)
	case igbp.SetSharedBufferMode:
		return g.onSimpleInt32Reply("setSharedBufferMode", data)
	case igbp.SetAutoRefresh:
		return g.onSimpleInt32Reply("setAutoRefresh", data)
	case igbp.SetLegacyBufferDrop:
		return g.onSimpleInt32Reply("setLegacyBufferDrop", data)
	case igbp.SetAutoPrerotation:
		return g.onSimpleInt32Reply("setAutoPrerotation", data)
	case igbp.DetachBuffer:
		return g.onSimpleInt32Reply("detachBuffer", data)
	case igbp.GetConsumerName:
		return g.onGetConsumerName()
	case igbp.GetUniqueId:
		return g.onGetUniqueId()
	case igbp.GetConsumerUsage:
		return g.onGetConsumerUsage()
	case igbp.AllocateBuffers:
		fmt.Println("  [IGBP] allocateBuffers")
		return nil, nil // void
	case igbp.GetLastQueuedBuffer:
		return g.onGetLastQueuedBuffer()
	case igbp.GetFrameTimestamps:
		fmt.Println("  [IGBP] getFrameTimestamps")
		return nil, nil // void
	default:
		fmt.Printf("  [IGBP] unhandled code=%d\n", code)
		reply := parcel.New()
		reply.WriteInt32(int32(igbp.StatusNoInit))
		return reply, nil
	}
}

// onSimpleInt32Reply handles transactions that read some arguments and
// return a single int32 status (NO_ERROR).
func (g *GraphicBufferProducerStub) onSimpleInt32Reply(
	name string,
	_ *parcel.Parcel,
) (*parcel.Parcel, error) {
	fmt.Printf("  [IGBP] %s\n", name)
	reply := parcel.New()
	reply.WriteInt32(int32(igbp.StatusOK))
	return reply, nil
}

// writeGraphicBufferToParcel writes a flattened GraphicBuffer to the parcel
// using the Parcel::write(Flattenable) wire format:
//
//	int32(flattenedSize) + int32(fdCount) + raw[flattenedSize] + fd objects
//
// The raw payload follows the GraphicBuffer::flatten() format (GB01).
func writeGraphicBufferToParcel(
	p *parcel.Parcel,
	buf *slotBuffer,
	bufID uint64,
) {
	// The native_handle has 1 fd (the memfd) and 0 extra ints.
	const numFds = int32(1)
	const numInts = int32(0)

	// Flattened size: 13 int32s (header) + numInts int32s.
	flattenedSize := int32(13+numInts) * 4

	p.WriteInt32(flattenedSize)
	p.WriteInt32(int32(numFds)) // fdCount

	// Build the 13-word header as raw bytes.
	raw := make([]byte, flattenedSize)
	binary.LittleEndian.PutUint32(raw[0:], uint32(igbp.GraphicBufferMagicGB01)) // buf[0]: magic
	binary.LittleEndian.PutUint32(raw[4:], buf.width)                      // buf[1]: width
	binary.LittleEndian.PutUint32(raw[8:], buf.height)                     // buf[2]: height
	binary.LittleEndian.PutUint32(raw[12:], buf.stride)                    // buf[3]: stride
	binary.LittleEndian.PutUint32(raw[16:], uint32(buf.format))            // buf[4]: format
	binary.LittleEndian.PutUint32(raw[20:], 1)                             // buf[5]: layerCount
	binary.LittleEndian.PutUint32(raw[24:], uint32(buf.usage))             // buf[6]: usage low
	binary.LittleEndian.PutUint32(raw[28:], uint32(bufID>>32))             // buf[7]: id high
	binary.LittleEndian.PutUint32(raw[32:], uint32(bufID&0xFFFFFFFF))      // buf[8]: id low
	binary.LittleEndian.PutUint32(raw[36:], 0)                             // buf[9]: generationNumber
	binary.LittleEndian.PutUint32(raw[40:], uint32(numFds))                // buf[10]: numFds
	binary.LittleEndian.PutUint32(raw[44:], uint32(numInts))               // buf[11]: numInts
	binary.LittleEndian.PutUint32(raw[48:], uint32(buf.usage>>32))         // buf[12]: usage high

	p.WriteRawBytes(raw)

	// Write the fd as a flat_binder_object (FD type).
	p.WriteFileDescriptor(int32(buf.fd))
}

func (g *GraphicBufferProducerStub) onRequestBuffer(
	data *parcel.Parcel,
) (*parcel.Parcel, error) {
	slot, _ := data.ReadInt32()
	fmt.Printf("  [IGBP] requestBuffer(slot=%d)\n", slot)

	g.mu.Lock()
	buf := g.slots[slot]
	g.mu.Unlock()

	reply := parcel.New()
	if buf == nil {
		fmt.Printf("  [IGBP] requestBuffer: slot %d has no buffer\n", slot)
		reply.WriteInt32(0) // nonNull=0
		reply.WriteInt32(int32(igbp.StatusOK))
		return reply, nil
	}

	// nonNull=1: we have a GraphicBuffer.
	reply.WriteInt32(1)

	// Generate a unique buffer ID from slot.
	bufID := uint64(0xCAFE0000) | uint64(slot)
	writeGraphicBufferToParcel(reply, buf, bufID)

	reply.WriteInt32(int32(igbp.StatusOK))
	fmt.Printf("  [IGBP] requestBuffer: slot=%d w=%d h=%d fmt=%d fd=%d\n",
		slot, buf.width, buf.height, buf.format, buf.fd)
	return reply, nil
}

// bufferSizeForFormat returns the buffer size in bytes for the given
// width, height, and pixel format.
func bufferSizeForFormat(
	w uint32,
	h uint32,
	format int32,
) int64 {
	switch igbp.PixelFormat(format) {
	case igbp.PixelFormatYCbCr420_888:
		// YCbCr 420: Y plane (w*h) + CbCr interleaved (w*h/2).
		return int64(w) * int64(h) * 3 / 2
	default:
		// Conservative RGBA fallback.
		return int64(w) * int64(h) * 4
	}
}

func (g *GraphicBufferProducerStub) onDequeueBuffer(
	data *parcel.Parcel,
) (*parcel.Parcel, error) {
	w, _ := data.ReadUint32()
	h, _ := data.ReadUint32()
	format, _ := data.ReadInt32()
	usage, _ := data.ReadUint64()
	getTimestamps, _ := data.ReadBool()
	fmt.Printf("  [IGBP] dequeueBuffer(w=%d, h=%d, fmt=%d, usage=0x%x, ts=%v)\n",
		w, h, format, usage, getTimestamps)

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

	// Check if we need to allocate a new buffer for this slot.
	needsRealloc := false
	existing := g.slots[slot]
	if existing == nil || existing.width != w || existing.height != h || existing.format != format {
		needsRealloc = true
		// Close old fd if present.
		if existing != nil && existing.fd >= 0 {
			unix.Close(existing.fd)
		}

		bufSize := bufferSizeForFormat(w, h, format)
		fd, err := unix.MemfdCreate("camera-buffer", 0)
		if err != nil {
			g.mu.Unlock()
			return nil, fmt.Errorf("memfd_create: %w", err)
		}
		if err := unix.Ftruncate(fd, bufSize); err != nil {
			unix.Close(fd)
			g.mu.Unlock()
			return nil, fmt.Errorf("ftruncate: %w", err)
		}
		g.slots[slot] = &slotBuffer{
			fd:     fd,
			width:  w,
			height: h,
			stride: w, // stride = width for simple buffers
			format: format,
			usage:  usage,
		}
		fmt.Printf("  [IGBP] allocated buffer slot=%d fd=%d size=%d\n", slot, fd, bufSize)
	}
	g.mu.Unlock()

	reply := parcel.New()
	reply.WriteInt32(int32(slot))

	// Fence as Flattenable: int32(flattenedSize=4) + int32(fdCount=0) + uint32(numFds=0).
	reply.WriteInt32(4) // flattenedSize
	reply.WriteInt32(0) // fdCount
	reply.WriteUint32(0)

	// bufferAge
	reply.WriteUint64(0)

	if getTimestamps {
		writeEmptyFrameEventHistoryDelta(reply)
	}

	if needsRealloc {
		reply.WriteInt32(int32(igbp.BufferNeedsRealloc))
	} else {
		reply.WriteInt32(int32(igbp.StatusOK))
	}
	return reply, nil
}

// writeEmptyFrameEventHistoryDelta writes an empty FrameEventHistoryDelta
// as a Flattenable: compositorTiming (3 * int64) + int32(0 deltas) = 28 bytes.
func writeEmptyFrameEventHistoryDelta(reply *parcel.Parcel) {
	reply.WriteInt32(28) // flattenedSize
	reply.WriteInt32(0)  // fdCount
	reply.WriteInt64(0)  // compositor deadline
	reply.WriteInt64(0)  // compositor interval
	reply.WriteInt64(0)  // compositor presentLatency
	reply.WriteInt32(0)  // delta count
}

func (g *GraphicBufferProducerStub) onQueueBuffer(
	data *parcel.Parcel,
) (*parcel.Parcel, error) {
	slot, _ := data.ReadInt32()
	fmt.Printf("  [IGBP] queueBuffer(slot=%d)\n", slot)

	select {
	case g.queuedCh <- struct{}{}:
	default:
	}

	reply := parcel.New()
	writeQueueBufferOutput(reply)
	reply.WriteInt32(int32(igbp.StatusOK))
	return reply, nil
}

// writeQueueBufferOutput writes a minimal QueueBufferOutput Flattenable.
// Fields (all written inline, not through Parcel::write wrapper):
//
//	width(4) + height(4) + transformHint(4) + numPendingBuffers(4) +
//	nextFrameNumber(8) + bufferReplaced(1) + maxBufferCount(4) +
//	FrameEventHistoryDelta(28 inlined) + result(4) = 61 bytes
func writeQueueBufferOutput(reply *parcel.Parcel) {
	const flatSize = 61
	reply.WriteInt32(flatSize)
	reply.WriteInt32(0) // fdCount

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
	// FrameEventHistoryDelta inline: compositorTiming (3*8=24) + deltaCount (4) = 28
	off += 24 // zeros for compositor timing
	binary.LittleEndian.PutUint32(buf[off:], 0) // deltaCount
	off += 4
	binary.LittleEndian.PutUint32(buf[off:], 0) // result = NO_ERROR
	_ = off

	reply.WriteRawBytes(buf)
}

func (g *GraphicBufferProducerStub) onCancelBuffer(
	data *parcel.Parcel,
) (*parcel.Parcel, error) {
	slot, _ := data.ReadInt32()
	fmt.Printf("  [IGBP] cancelBuffer(slot=%d)\n", slot)
	reply := parcel.New()
	reply.WriteInt32(int32(igbp.StatusOK))
	return reply, nil
}

func (g *GraphicBufferProducerStub) onQuery(
	data *parcel.Parcel,
) (*parcel.Parcel, error) {
	rawWhat, _ := data.ReadInt32()
	what := igbp.NativeWindowQuery(rawWhat)

	var value int32
	switch what {
	case igbp.NativeWindowWidth:
		value = int32(g.width)
	case igbp.NativeWindowHeight:
		value = int32(g.height)
	case igbp.NativeWindowFormat:
		value = g.format
	case igbp.NativeWindowMinUndequeued:
		value = 1
	case igbp.NativeWindowQueuesToComposer:
		value = 0
	case igbp.NativeWindowConcreteType:
		value = int32(igbp.NativeWindowSurface)
	case igbp.NativeWindowDefaultWidth:
		value = int32(g.width)
	case igbp.NativeWindowDefaultHeight:
		value = int32(g.height)
	case igbp.NativeWindowTransformHint:
		value = 0
	case igbp.NativeWindowConsumerRunning:
		value = 0
	case igbp.NativeWindowConsumerUsageBits:
		value = 0
	case igbp.NativeWindowStickyTransform:
		value = 0
	case igbp.NativeWindowDefaultDataspace:
		value = 0
	case igbp.NativeWindowBufferAge:
		value = 0
	case igbp.NativeWindowMaxBufferCount:
		value = 64
	default:
		fmt.Printf("  [IGBP] query: unknown what=%d\n", what)
	}

	fmt.Printf("  [IGBP] query(what=%d) -> %d\n", what, value)
	reply := parcel.New()
	reply.WriteInt32(value)
	reply.WriteInt32(int32(igbp.StatusOK))
	return reply, nil
}

func (g *GraphicBufferProducerStub) onConnect(
	data *parcel.Parcel,
) (*parcel.Parcel, error) {
	hasListener, _ := data.ReadInt32()
	if hasListener == 1 {
		_, _ = data.ReadStrongBinder()
	}
	api, _ := data.ReadInt32()
	producerControlled, _ := data.ReadInt32()
	fmt.Printf("  [IGBP] connect(api=%d, producerControlled=%d)\n", api, producerControlled)

	reply := parcel.New()
	writeQueueBufferOutput(reply)
	reply.WriteInt32(int32(igbp.StatusOK))
	return reply, nil
}

func (g *GraphicBufferProducerStub) onDisconnect(
	data *parcel.Parcel,
) (*parcel.Parcel, error) {
	api, _ := data.ReadInt32()
	fmt.Printf("  [IGBP] disconnect(api=%d)\n", api)
	reply := parcel.New()
	reply.WriteInt32(int32(igbp.StatusOK))
	return reply, nil
}

func (g *GraphicBufferProducerStub) onGetConsumerName() (*parcel.Parcel, error) {
	fmt.Println("  [IGBP] getConsumerName")
	reply := parcel.New()
	reply.WriteString16("GoCamera")
	return reply, nil
}

func (g *GraphicBufferProducerStub) onGetUniqueId() (*parcel.Parcel, error) {
	fmt.Println("  [IGBP] getUniqueId")
	reply := parcel.New()
	reply.WriteUint64(0x12345678)
	reply.WriteInt32(int32(igbp.StatusOK))
	return reply, nil
}

func (g *GraphicBufferProducerStub) onGetConsumerUsage() (*parcel.Parcel, error) {
	fmt.Println("  [IGBP] getConsumerUsage")
	reply := parcel.New()
	// Return 0 consumer usage for now; the camera service validates usage bits.
	reply.WriteUint64(0)
	reply.WriteInt32(int32(igbp.StatusOK))
	return reply, nil
}

func (g *GraphicBufferProducerStub) onGetLastQueuedBuffer() (*parcel.Parcel, error) {
	fmt.Println("  [IGBP] getLastQueuedBuffer")
	reply := parcel.New()
	reply.WriteInt32(int32(igbp.StatusNoInit))
	return reply, nil
}

// --------------------------------------------------------------------
// CreateStream with Surface via raw parcel (writes IGBP binder)
// --------------------------------------------------------------------

func createStreamWithSurface(
	ctx context.Context,
	deviceUser fwkDevice.ICameraDeviceUser,
	transport binder.Transport,
	igbpStub *binder.StubBinder,
	width int32,
	height int32,
) (int32, error) {
	data := parcel.New()
	data.WriteInterfaceToken(fwkDevice.DescriptorICameraDeviceUser)

	// Null indicator for OutputConfiguration parcelable.
	data.WriteInt32(1)

	headerPos := parcel.WriteParcelableHeader(data)

	// windowHandles: empty array (non-nullable in AIDL).
	data.WriteInt32(0)

	data.WriteInt32(0)  // rotation: R0
	data.WriteInt32(-1) // windowGroupId: NONE
	data.WriteString16("")
	data.WriteInt32(width)
	data.WriteInt32(height)
	data.WriteBool(false) // isDeferred

	// surfaces: array of 1 android.view.Surface.
	data.WriteInt32(1)

	// Each element has a null indicator in AIDL serialization.
	data.WriteInt32(1) // non-null

	// view::Surface::writeToParcel(parcel, false):
	data.WriteString16("GoCamera")
	data.WriteInt32(0) // isSingleBuffered
	// IGraphicBufferProducer::exportToParcel: magic + strong binder.
	data.WriteUint32(0x62717565) // USE_BUFFER_QUEUE
	fmt.Printf("  [DEBUG] IGBP binder offset in parcel: %d\n", data.Len())
	binder.WriteBinderToParcel(ctx, data, igbpStub, transport)
	fmt.Printf("  [DEBUG] After IGBP binder, parcel pos: %d\n", data.Len())
	// surfaceControlHandle: null
	data.WriteNullStrongBinder()
	fmt.Printf("  [DEBUG] After null binder, parcel pos: %d\n", data.Len())
	fmt.Printf("  [DEBUG] Objects in parcel: %v\n", data.Objects())

	parcel.WriteParcelableFooter(data, headerPos)
	fmt.Printf("  [DEBUG] Final parcel size: %d, objects: %v\n", data.Len(), data.Objects())

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

// createDefaultRequestRaw calls CreateDefaultRequest using raw parcel I/O.
func createDefaultRequestRaw(
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

// submitRequestProxy sends a SubmitRequestList using the generated
// CaptureRequest.MarshalParcel (correct format without per-element null
// indicators) and a raw Transact with the correct transaction code.
func submitRequestProxy(
	ctx context.Context,
	deviceUser fwkDevice.ICameraDeviceUser,
	captureReq fwkDevice.CaptureRequest,
	isRepeating bool,
) (fwkDevice.SubmitInfo, error) {
	var result fwkDevice.SubmitInfo

	data := parcel.New()
	data.WriteInterfaceToken(fwkDevice.DescriptorICameraDeviceUser)
	data.WriteInt32(1) // requestList: array of 1
	data.WriteInt32(1) // non-null indicator for CaptureRequest element
	if err := captureReq.MarshalParcel(data); err != nil {
		return result, fmt.Errorf("marshal: %w", err)
	}
	data.WriteBool(isRepeating)

	fmt.Printf("  [DEBUG] submitRequestProxy: size=%d, isRepeating=%v\n",
		data.Len(), isRepeating)

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

// --------------------------------------------------------------------
// Main flow
// --------------------------------------------------------------------

func run(ctx context.Context) error {
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

	svc, err := sm.GetService(ctx, "android.frameworks.cameraservice.service.ICameraService/default")
	if err != nil {
		return fmt.Errorf("get service: %w", err)
	}
	fmt.Println("Got frameworks camera service")

	proxy := fwkService.NewCameraServiceProxy(svc)
	cb := &cameraDeviceCallback{}
	stub := fwkDevice.NewCameraDeviceCallbackStub(cb)

	stubBinder := stub.AsBinder().(*binder.StubBinder)
	stubBinder.RegisterWithTransport(ctx, transport)
	time.Sleep(100 * time.Millisecond)
	fmt.Println("Callback stub registered")

	// ConnectDevice
	fmt.Println("\nCalling ConnectDevice...")
	deviceUser, err := proxy.ConnectDevice(ctx, stub, "0")
	if err != nil {
		return fmt.Errorf("ConnectDevice: %w", err)
	}
	fmt.Println("ConnectDevice succeeded!")
	fmt.Printf("  deviceUser binder handle: %d\n", deviceUser.AsBinder().Handle())

	// Step 1: BeginConfigure
	fmt.Println("\n=== Step 1: BeginConfigure ===")
	if err = deviceUser.BeginConfigure(ctx); err != nil {
		return fmt.Errorf("BeginConfigure: %w", err)
	}
	fmt.Println("BeginConfigure OK")

	// Step 2: CreateDefaultRequest
	fmt.Println("\n=== Step 2: CreateDefaultRequest (PREVIEW) ===")
	metadataBytes, err := createDefaultRequestRaw(ctx, deviceUser, fwkDevice.TemplateIdPREVIEW)
	if err != nil {
		return fmt.Errorf("CreateDefaultRequest: %w", err)
	}
	fmt.Printf("CreateDefaultRequest OK: metadata len=%d\n", len(metadataBytes))
	if len(metadataBytes) > 64 {
		fmt.Printf("  first 64 bytes: %s\n", hex.EncodeToString(metadataBytes[:64]))
	}

	// Step 3: Create IGBP and CreateStream
	fmt.Println("\n=== Step 3: CreateStream with IGBP Surface ===")
	igbp := NewGraphicBufferProducerStub(640, 480)
	igbpStubBinder := binder.NewStubBinder(igbp)
	igbpStubBinder.RegisterWithTransport(ctx, transport)

	streamId, err := createStreamWithSurface(ctx, deviceUser, transport, igbpStubBinder, 640, 480)
	if err != nil {
		fmt.Fprintf(os.Stderr, "CreateStream with Surface FAILED: %v\n", err)

		// Fallback: try deferred stream.
		fmt.Println("Trying deferred stream as fallback...")
		_ = deviceUser.BeginConfigure(ctx)
		deferredConfig := fwkDevice.OutputConfiguration{
			WindowHandles:    nil,
			Rotation:         0,
			WindowGroupId:    -1,
			PhysicalCameraId: "",
			Width:            640,
			Height:           480,
			IsDeferred:       true,
		}
		streamId, err = deviceUser.CreateStream(ctx, deferredConfig)
		if err != nil {
			return fmt.Errorf("CreateStream (both approaches failed): %w", err)
		}
	}
	fmt.Printf("CreateStream OK: streamId=%d\n", streamId)

	// Step 4: EndConfigure
	fmt.Println("\n=== Step 4: EndConfigure ===")
	{
		// Build the parcel manually to verify the wire format.
		ecData := parcel.New()
		ecData.WriteInterfaceToken(fwkDevice.DescriptorICameraDeviceUser)
		ecData.WriteInt32(int32(fwkDevice.StreamConfigurationModeNormalMode))
		ecData.WriteInt32(1) // null indicator (this is what generated proxy does)
		sessionParams := fwkDevice.CameraMetadata{Metadata: []byte{}}
		if marshalErr := sessionParams.MarshalParcel(ecData); marshalErr != nil {
			return fmt.Errorf("marshal sessionParams: %w", marshalErr)
		}
		ecData.WriteInt64(0) // startTimeNs
		fmt.Printf("  [DEBUG] EndConfigure parcel size: %d, hex: %s\n",
			ecData.Len(), hex.EncodeToString(ecData.Data()))
	}
	// Pass empty (non-null) metadata; nil byte array triggers STATUS_UNEXPECTED_NULL.
	if err = deviceUser.EndConfigure(ctx, fwkDevice.StreamConfigurationModeNormalMode, fwkDevice.CameraMetadata{Metadata: []byte{}}, 0); err != nil {
		return fmt.Errorf("EndConfigure: %w", err)
	}
	fmt.Println("EndConfigure OK")

	// Step 5: SubmitRequestList
	fmt.Println("\n=== Step 5: SubmitRequestList (repeating) ===")

	// Try using the generated proxy instead of raw parcel building.
	fmt.Println("  Trying generated proxy SubmitRequestList...")
	captureReq := fwkDevice.CaptureRequest{
		PhysicalCameraSettings: []fwkDevice.PhysicalCameraSettings{
			{
				Id: "0",
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

	// Debug: compare proxy vs raw parcel formats from the array start.
	{
		// Proxy parcel.
		proxyData := parcel.New()
		proxyData.WriteInterfaceToken(fwkDevice.DescriptorICameraDeviceUser)
		tokenEnd := proxyData.Len()
		proxyData.WriteInt32(1) // array length
		_ = captureReq.MarshalParcel(proxyData)
		proxyData.WriteBool(true)
		pd := proxyData.Data()
		dumpEnd := tokenEnd + 80
		if dumpEnd > len(pd) {
			dumpEnd = len(pd)
		}
		fmt.Printf("  [DEBUG] Token ends at: %d\n", tokenEnd)
		fmt.Printf("  [DEBUG] Proxy size=%d, from %d: %s\n",
			len(pd), tokenEnd, hex.EncodeToString(pd[tokenEnd:dumpEnd]))

		// Raw parcel (old format with null indicators).
		rawData := parcel.New()
		rawData.WriteInterfaceToken(fwkDevice.DescriptorICameraDeviceUser)
		rawData.WriteInt32(1) // array length
		rawData.WriteInt32(1) // CaptureRequest null indicator (EXTRA)
		rh := parcel.WriteParcelableHeader(rawData)
		rawData.WriteInt32(1) // PCS array
		rawData.WriteInt32(1) // PCS null indicator (EXTRA)
		ph := parcel.WriteParcelableHeader(rawData)
		rawData.WriteString16("0")
		ch := parcel.WriteParcelableHeader(rawData)
		rawData.WriteInt32(fwkDevice.CaptureMetadataInfoTagMetadata)
		mh := parcel.WriteParcelableHeader(rawData)
		rawData.WriteByteArray(metadataBytes)
		parcel.WriteParcelableFooter(rawData, mh)
		parcel.WriteParcelableFooter(rawData, ch)
		parcel.WriteParcelableFooter(rawData, ph)
		rawData.WriteInt32(1)
		rawData.WriteInt32(1) // SAW null indicator (EXTRA)
		sh := parcel.WriteParcelableHeader(rawData)
		rawData.WriteInt32(streamId)
		rawData.WriteInt32(0)
		parcel.WriteParcelableFooter(rawData, sh)
		parcel.WriteParcelableFooter(rawData, rh)
		rawData.WriteBool(true)
		rd := rawData.Data()
		dumpEnd = tokenEnd + 80
		if dumpEnd > len(rd) {
			dumpEnd = len(rd)
		}
		fmt.Printf("  [DEBUG] Raw   size=%d, from %d: %s\n",
			len(rd), tokenEnd, hex.EncodeToString(rd[tokenEnd:dumpEnd]))
	}

	// Use the generated proxy with fixed null indicators.
	submitInfo, err := submitRequestProxy(ctx, deviceUser, captureReq, true)
	if err != nil {
		fmt.Fprintf(os.Stderr, "SubmitRequestList (proxy, repeating) FAILED: %v\n", err)

		submitInfo, err = submitRequestProxy(ctx, deviceUser, captureReq, false)
		if err != nil {
			return fmt.Errorf("SubmitRequestList: %w", err)
		}
	}
	fmt.Printf("SubmitRequestList OK: requestId=%d lastFrame=%d\n",
		submitInfo.RequestId, submitInfo.LastFrameNumber)

	// Wait for callbacks.
	fmt.Println("\nWaiting for callbacks (up to 10 seconds)...")
	deadline := time.After(10 * time.Second)
	for {
		select {
		case <-deadline:
			fmt.Println("Timeout reached")
			goto done
		case <-time.After(200 * time.Millisecond):
			cb.mu.Lock()
			n := cb.framesReceived
			cb.mu.Unlock()
			if n >= 3 {
				fmt.Printf("Got %d frames, success!\n", n)
				goto done
			}
		}
	}

done:
	fmt.Println("\n=== Disconnect ===")
	if err = deviceUser.Disconnect(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Disconnect: %v\n", err)
	} else {
		fmt.Println("Disconnect OK")
	}

	cb.mu.Lock()
	n := cb.framesReceived
	cb.mu.Unlock()
	fmt.Printf("\nDone. Total frames received: %d\n", n)
	if n == 0 {
		return fmt.Errorf("no frames received")
	}
	return nil
}

func main() {
	ctx := context.Background()
	ctx = logger.CtxWithLogger(ctx, logrus.Default())

	if err := run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: %v\n", err)
		os.Exit(1)
	}
}
