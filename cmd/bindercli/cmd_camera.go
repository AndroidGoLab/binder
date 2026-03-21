//go:build linux

package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"sync"
	"time"

	common "github.com/xaionaro-go/binder/android/hardware/common"
	"github.com/xaionaro-go/binder/android/hardware/graphics/allocator"
	gfxCommon "github.com/xaionaro-go/binder/android/hardware/graphics/common"

	fwkDevice "github.com/xaionaro-go/binder/android/frameworks/cameraservice/device"
	fwkService "github.com/xaionaro-go/binder/android/frameworks/cameraservice/service"
	"github.com/xaionaro-go/binder/binder"
	"github.com/xaionaro-go/binder/cmd/internal/igbp"
	"github.com/xaionaro-go/binder/parcel"
	"github.com/xaionaro-go/binder/servicemanager"

	"github.com/spf13/cobra"
	"golang.org/x/sys/unix"
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

// --------------------------------------------------------------------
// Camera device callbacks
// --------------------------------------------------------------------

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

// --------------------------------------------------------------------
// Gralloc buffer allocation
// --------------------------------------------------------------------

// grallocBuffer holds a gralloc-allocated buffer with its NativeHandle.
type grallocBuffer struct {
	handle common.NativeHandle
	stride int32
	width  uint32
	height uint32
	format int32
	usage  uint64

	// mmapData holds a persistent mmap of the dmabuf, set by Mmap().
	// Keeping it mapped avoids mmap/munmap syscalls per frame read.
	mmapData []byte
}

// Mmap creates a persistent read-only mmap of this buffer's dmabuf FD.
// The mmapData field can then be read directly. Call Munmap to release.
func (b *grallocBuffer) Mmap() error {
	if len(b.handle.Fds) == 0 {
		return fmt.Errorf("no FDs in gralloc buffer")
	}
	fd := int(b.handle.Fds[0])
	bufSize := int(b.width) * int(b.height) * 3 / 2 // YCbCr_420_888
	data, err := unix.Mmap(fd, 0, bufSize, unix.PROT_READ, unix.MAP_SHARED)
	if err != nil {
		return fmt.Errorf("mmap fd=%d size=%d: %w", fd, bufSize, err)
	}
	b.mmapData = data
	return nil
}

// Munmap releases the persistent mmap created by Mmap.
func (b *grallocBuffer) Munmap() {
	if b.mmapData != nil {
		_ = unix.Munmap(b.mmapData)
		b.mmapData = nil
	}
}

// allocateGrallocBuffer allocates a gralloc buffer using the IAllocator service.
func allocateGrallocBuffer(
	ctx context.Context,
	sm *servicemanager.ServiceManager,
	width int32,
	height int32,
	format gfxCommon.PixelFormat,
	usage gfxCommon.BufferUsage,
) (*grallocBuffer, error) {
	svc, err := sm.GetService(ctx, "android.hardware.graphics.allocator.IAllocator/default")
	if err != nil {
		return nil, fmt.Errorf("get allocator service: %w", err)
	}

	proxy := allocator.NewAllocatorProxy(svc)

	name := make([]byte, allocator.BufferDescriptorInfoNameSize)
	copy(name, "camera-buffer")

	desc := allocator.BufferDescriptorInfo{
		Name:              name,
		Width:             width,
		Height:            height,
		LayerCount:        1,
		Format:            format,
		Usage:             usage,
		ReservedSize:      0,
		AdditionalOptions: []gfxCommon.ExtendableType{},
	}

	result, err := proxy.Allocate2(ctx, desc, 1)
	if err != nil {
		return nil, fmt.Errorf("Allocate2: %w", err)
	}

	if len(result.Buffers) == 0 {
		return nil, fmt.Errorf("Allocate2 returned 0 buffers")
	}

	buf := &grallocBuffer{
		handle: result.Buffers[0],
		stride: result.Stride,
		width:  uint32(width),
		height: uint32(height),
		format: int32(format),
		usage:  uint64(usage),
	}

	fmt.Fprintf(os.Stderr, "  Gralloc buffer: stride=%d fds=%v ints_count=%d\n",
		buf.stride, buf.handle.Fds, len(buf.handle.Ints))
	return buf, nil
}

// --------------------------------------------------------------------
// IGraphicBufferProducer stub (native binder, non-AIDL)
// --------------------------------------------------------------------

// slotBuffer holds per-slot buffer state backed by a gralloc buffer.
type slotBuffer struct {
	gralloc *grallocBuffer
}

// graphicBufferProducerStub implements a minimal IGraphicBufferProducer
// that satisfies the camera service's output configuration requirements.
type graphicBufferProducerStub struct {
	width  uint32
	height uint32
	format int32

	// Pre-allocated gralloc buffers (one per slot we may use).
	grallocBufs [4]*grallocBuffer

	mu       sync.Mutex
	nextSlot int
	slots    [igbp.MaxBufferSlots]*slotBuffer
	queuedCh chan queuedFrame
}

// queuedFrame contains the slot index of a queued buffer.
type queuedFrame struct {
	slot int
}

func newGraphicBufferProducerStub(
	width uint32,
	height uint32,
	grallocBufs [4]*grallocBuffer,
) *graphicBufferProducerStub {
	return &graphicBufferProducerStub{
		width:       width,
		height:      height,
		format:      int32(igbp.PixelFormatYCbCr420_888),
		grallocBufs: grallocBufs,
		queuedCh:    make(chan queuedFrame, 16),
	}
}

func (g *graphicBufferProducerStub) Descriptor() string {
	return igbp.Descriptor
}

func (g *graphicBufferProducerStub) OnTransaction(
	_ context.Context,
	code binder.TransactionCode,
	data *parcel.Parcel,
) (*parcel.Parcel, error) {
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
		fmt.Fprintln(os.Stderr, "  [IGBP] allocateBuffers")
		return nil, nil // void
	case igbp.GetLastQueuedBuffer:
		return g.onGetLastQueuedBuffer()
	case igbp.GetFrameTimestamps:
		fmt.Fprintln(os.Stderr, "  [IGBP] getFrameTimestamps")
		return nil, nil // void
	default:
		fmt.Fprintf(os.Stderr, "  [IGBP] unhandled code=%d\n", code)
		reply := parcel.New()
		reply.WriteInt32(int32(igbp.StatusNoInit))
		return reply, nil
	}
}

func (g *graphicBufferProducerStub) onSimpleInt32Reply(
	name string,
	_ *parcel.Parcel,
) (*parcel.Parcel, error) {
	fmt.Fprintf(os.Stderr, "  [IGBP] %s\n", name)
	reply := parcel.New()
	reply.WriteInt32(int32(igbp.StatusOK))
	return reply, nil
}

// writeGrallocGraphicBufferToParcel writes a flattened GraphicBuffer backed
// by a gralloc NativeHandle. The wire format is:
//
//	int32(flattenedSize) + int32(fdCount) + raw[flattenedSize] + fd objects
//
// The raw payload follows the GraphicBuffer::flatten() format (GB01).
func writeGrallocGraphicBufferToParcel(
	p *parcel.Parcel,
	buf *grallocBuffer,
	bufID uint64,
) {
	numFds := int32(len(buf.handle.Fds))
	numInts := int32(len(buf.handle.Ints))

	// Flattened size: 13 int32s (header) + numInts int32s.
	flattenedSize := (13 + numInts) * 4

	p.WriteInt32(flattenedSize)
	p.WriteInt32(numFds)

	// Build the header + ints as raw bytes.
	raw := make([]byte, flattenedSize)
	binary.LittleEndian.PutUint32(raw[0:], uint32(igbp.GraphicBufferMagicGB01)) // buf[0]: magic
	binary.LittleEndian.PutUint32(raw[4:], buf.width)                      // buf[1]: width
	binary.LittleEndian.PutUint32(raw[8:], buf.height)                     // buf[2]: height
	binary.LittleEndian.PutUint32(raw[12:], uint32(buf.stride))            // buf[3]: stride
	binary.LittleEndian.PutUint32(raw[16:], uint32(buf.format))            // buf[4]: format
	binary.LittleEndian.PutUint32(raw[20:], 1)                             // buf[5]: layerCount
	binary.LittleEndian.PutUint32(raw[24:], uint32(buf.usage))             // buf[6]: usage low
	binary.LittleEndian.PutUint32(raw[28:], uint32(bufID>>32))             // buf[7]: id high
	binary.LittleEndian.PutUint32(raw[32:], uint32(bufID&0xFFFFFFFF))      // buf[8]: id low
	binary.LittleEndian.PutUint32(raw[36:], 0)                             // buf[9]: generationNumber
	binary.LittleEndian.PutUint32(raw[40:], uint32(numFds))                // buf[10]: numFds
	binary.LittleEndian.PutUint32(raw[44:], uint32(numInts))               // buf[11]: numInts
	binary.LittleEndian.PutUint32(raw[48:], uint32(buf.usage>>32))         // buf[12]: usage high

	// Append native_handle ints after the 13-word header.
	for i, v := range buf.handle.Ints {
		binary.LittleEndian.PutUint32(raw[52+i*4:], uint32(v))
	}

	p.WriteRawBytes(raw)

	// Write each FD as a flat_binder_object.
	for _, fd := range buf.handle.Fds {
		p.WriteFileDescriptor(fd)
	}
}

func (g *graphicBufferProducerStub) onRequestBuffer(
	data *parcel.Parcel,
) (*parcel.Parcel, error) {
	slot, _ := data.ReadInt32()
	fmt.Fprintf(os.Stderr, "  [IGBP] requestBuffer(slot=%d)\n", slot)

	g.mu.Lock()
	buf := g.slots[slot]
	g.mu.Unlock()

	reply := parcel.New()
	if buf == nil {
		fmt.Fprintf(os.Stderr, "  [IGBP] requestBuffer: slot %d has no buffer\n", slot)
		reply.WriteInt32(0) // nonNull=0
		reply.WriteInt32(int32(igbp.StatusOK))
		return reply, nil
	}

	// nonNull=1: we have a GraphicBuffer.
	reply.WriteInt32(1)

	bufID := uint64(0xCAFE0000) | uint64(slot)
	writeGrallocGraphicBufferToParcel(reply, buf.gralloc, bufID)

	reply.WriteInt32(int32(igbp.StatusOK))
	fmt.Fprintf(os.Stderr, "  [IGBP] requestBuffer: slot=%d w=%d h=%d stride=%d fmt=%d fds=%v\n",
		slot, buf.gralloc.width, buf.gralloc.height, buf.gralloc.stride,
		buf.gralloc.format, buf.gralloc.handle.Fds)
	return reply, nil
}

func (g *graphicBufferProducerStub) onDequeueBuffer(
	data *parcel.Parcel,
) (*parcel.Parcel, error) {
	w, _ := data.ReadUint32()
	h, _ := data.ReadUint32()
	format, _ := data.ReadInt32()
	usage, _ := data.ReadUint64()
	getTimestamps, _ := data.ReadBool()
	fmt.Fprintf(os.Stderr, "  [IGBP] dequeueBuffer(w=%d, h=%d, fmt=%d, usage=0x%x, ts=%v)\n",
		w, h, format, usage, getTimestamps)

	g.mu.Lock()
	slot := g.nextSlot
	g.nextSlot = (g.nextSlot + 1) % 4

	needsRealloc := false
	if g.slots[slot] == nil {
		needsRealloc = true
		// Assign the pre-allocated gralloc buffer.
		g.slots[slot] = &slotBuffer{
			gralloc: g.grallocBufs[slot],
		}
		fmt.Fprintf(os.Stderr, "  [IGBP] assigned gralloc buffer to slot=%d\n", slot)
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

func (g *graphicBufferProducerStub) onQueueBuffer(
	data *parcel.Parcel,
) (*parcel.Parcel, error) {
	slot, _ := data.ReadInt32()
	fmt.Fprintf(os.Stderr, "  [IGBP] queueBuffer(slot=%d)\n", slot)

	select {
	case g.queuedCh <- queuedFrame{slot: int(slot)}:
	default:
	}

	reply := parcel.New()
	writeQueueBufferOutput(reply)
	reply.WriteInt32(int32(igbp.StatusOK))
	return reply, nil
}

// queueBufferOutputPayload is a pre-computed QueueBufferOutput flattenable payload.
// Allocated once to avoid per-frame allocation in writeQueueBufferOutput.
var queueBufferOutputPayload = func() []byte {
	const flatSize = 61
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
	return buf
}()

// writeQueueBufferOutput writes a minimal QueueBufferOutput Flattenable.
func writeQueueBufferOutput(reply *parcel.Parcel) {
	reply.WriteInt32(int32(len(queueBufferOutputPayload)))
	reply.WriteInt32(0) // fdCount
	reply.WriteRawBytes(queueBufferOutputPayload)
}

func (g *graphicBufferProducerStub) onCancelBuffer(
	data *parcel.Parcel,
) (*parcel.Parcel, error) {
	slot, _ := data.ReadInt32()
	fmt.Fprintf(os.Stderr, "  [IGBP] cancelBuffer(slot=%d)\n", slot)
	reply := parcel.New()
	reply.WriteInt32(int32(igbp.StatusOK))
	return reply, nil
}

func (g *graphicBufferProducerStub) onQuery(
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
		fmt.Fprintf(os.Stderr, "  [IGBP] query: unknown what=%d\n", what)
	}

	fmt.Fprintf(os.Stderr, "  [IGBP] query(what=%d) -> %d\n", what, value)
	reply := parcel.New()
	reply.WriteInt32(value)
	reply.WriteInt32(int32(igbp.StatusOK))
	return reply, nil
}

func (g *graphicBufferProducerStub) onConnect(
	data *parcel.Parcel,
) (*parcel.Parcel, error) {
	hasListener, _ := data.ReadInt32()
	if hasListener == 1 {
		_, _ = data.ReadStrongBinder()
	}
	api, _ := data.ReadInt32()
	producerControlled, _ := data.ReadInt32()
	fmt.Fprintf(os.Stderr, "  [IGBP] connect(api=%d, producerControlled=%d)\n", api, producerControlled)

	reply := parcel.New()
	writeQueueBufferOutput(reply)
	reply.WriteInt32(int32(igbp.StatusOK))
	return reply, nil
}

func (g *graphicBufferProducerStub) onDisconnect(
	data *parcel.Parcel,
) (*parcel.Parcel, error) {
	api, _ := data.ReadInt32()
	fmt.Fprintf(os.Stderr, "  [IGBP] disconnect(api=%d)\n", api)
	reply := parcel.New()
	reply.WriteInt32(int32(igbp.StatusOK))
	return reply, nil
}

func (g *graphicBufferProducerStub) onGetConsumerName() (*parcel.Parcel, error) {
	fmt.Fprintln(os.Stderr, "  [IGBP] getConsumerName")
	reply := parcel.New()
	reply.WriteString16("GoCamera")
	return reply, nil
}

func (g *graphicBufferProducerStub) onGetUniqueId() (*parcel.Parcel, error) {
	fmt.Fprintln(os.Stderr, "  [IGBP] getUniqueId")
	reply := parcel.New()
	reply.WriteUint64(0x12345678)
	reply.WriteInt32(int32(igbp.StatusOK))
	return reply, nil
}

func (g *graphicBufferProducerStub) onGetConsumerUsage() (*parcel.Parcel, error) {
	fmt.Fprintln(os.Stderr, "  [IGBP] getConsumerUsage")
	reply := parcel.New()
	reply.WriteUint64(0)
	reply.WriteInt32(int32(igbp.StatusOK))
	return reply, nil
}

func (g *graphicBufferProducerStub) onGetLastQueuedBuffer() (*parcel.Parcel, error) {
	fmt.Fprintln(os.Stderr, "  [IGBP] getLastQueuedBuffer")
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

	data.WriteInt32(1) // non-null OutputConfiguration

	headerPos := parcel.WriteParcelableHeader(data)

	// windowHandles: empty array.
	data.WriteInt32(0)

	data.WriteInt32(0)  // rotation: R0
	data.WriteInt32(-1) // windowGroupId: NONE
	data.WriteString16("")
	data.WriteInt32(width)
	data.WriteInt32(height)
	data.WriteBool(false) // isDeferred

	// surfaces: array of 1.
	data.WriteInt32(1)
	data.WriteInt32(1) // non-null

	// view::Surface::writeToParcel
	data.WriteString16("GoCamera")
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

// submitRequestRaw sends a SubmitRequestList using raw parcel I/O.
func submitRequestRaw(
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
// Main recording flow
// --------------------------------------------------------------------

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
	var grallocBufs [4]*grallocBuffer
	for i := range grallocBufs {
		buf, allocErr := allocateGrallocBuffer(
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

	// ConnectDevice
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

	// Step 1: BeginConfigure
	fmt.Fprintln(os.Stderr, "\n=== Step 1: BeginConfigure ===")
	if err = deviceUser.BeginConfigure(ctx); err != nil {
		return fmt.Errorf("BeginConfigure: %w", err)
	}
	fmt.Fprintln(os.Stderr, "BeginConfigure OK")

	// Step 2: CreateDefaultRequest
	fmt.Fprintln(os.Stderr, "\n=== Step 2: CreateDefaultRequest (PREVIEW) ===")
	metadataBytes, err := createDefaultRequestRaw(ctx, deviceUser, fwkDevice.TemplateIdPREVIEW)
	if err != nil {
		return fmt.Errorf("CreateDefaultRequest: %w", err)
	}
	fmt.Fprintf(os.Stderr, "CreateDefaultRequest OK: metadata len=%d\n", len(metadataBytes))

	// Step 3: Create IGBP and CreateStream
	fmt.Fprintln(os.Stderr, "\n=== Step 3: CreateStream with IGBP Surface ===")
	uWidth := uint32(width)
	uHeight := uint32(height)
	igbp := newGraphicBufferProducerStub(uWidth, uHeight, grallocBufs)
	igbpStubBinder := binder.NewStubBinder(igbp)
	igbpStubBinder.RegisterWithTransport(ctx, transport)

	streamId, err := createStreamWithSurface(ctx, deviceUser, transport, igbpStubBinder, width, height)
	if err != nil {
		return fmt.Errorf("CreateStream: %w", err)
	}
	fmt.Fprintf(os.Stderr, "CreateStream OK: streamId=%d\n", streamId)

	// Step 4: EndConfigure
	fmt.Fprintln(os.Stderr, "\n=== Step 4: EndConfigure ===")
	if err = deviceUser.EndConfigure(ctx, fwkDevice.StreamConfigurationModeNormalMode, fwkDevice.CameraMetadata{Metadata: []byte{}}, 0); err != nil {
		return fmt.Errorf("EndConfigure: %w", err)
	}
	fmt.Fprintln(os.Stderr, "EndConfigure OK")

	// Step 5: SubmitRequestList
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

	submitInfo, err := submitRequestRaw(ctx, deviceUser, captureReq, true)
	if err != nil {
		fmt.Fprintf(os.Stderr, "SubmitRequestList (repeating) FAILED: %v\n", err)
		submitInfo, err = submitRequestRaw(ctx, deviceUser, captureReq, false)
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
		case frame := <-igbp.queuedCh:
			igbp.mu.Lock()
			buf := igbp.slots[frame.slot]
			igbp.mu.Unlock()

			if buf == nil {
				fmt.Fprintf(os.Stderr, "  Frame from slot %d: no buffer\n", frame.slot)
				continue
			}

			// Write directly from the persistent mmap to output,
			// avoiding an intermediate copy through a frame buffer.
			frameData := buf.gralloc.mmapData
			if frameData == nil {
				fmt.Fprintf(os.Stderr, "  Frame from slot %d: buffer not mmap'd\n", frame.slot)
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
