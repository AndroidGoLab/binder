// Package igbp provides a minimal IGraphicBufferProducer stub backed by
// gralloc-allocated buffers, suitable for receiving camera frames.
package igbp

import (
	"context"
	"fmt"
	"sync"

	"github.com/xaionaro-go/binder/binder"
	"github.com/xaionaro-go/binder/camera/gralloc"
	"github.com/xaionaro-go/binder/igbp"
	"github.com/xaionaro-go/binder/parcel"
)

// ProducerStub implements a minimal IGraphicBufferProducer that provides
// gralloc-allocated buffers to the camera HAL. Queued frames are
// delivered via QueuedFrames().
type ProducerStub struct {
	width  uint32
	height uint32
	format int32

	grallocBufs [4]*gralloc.Buffer

	mu       sync.Mutex
	nextSlot int
	slots    [igbp.MaxBufferSlots]*slotBuffer
	queuedCh chan int // slot index of queued buffer
}

// slotBuffer holds per-slot buffer state backed by a gralloc buffer.
type slotBuffer struct {
	gralloc *gralloc.Buffer
}

// NewProducerStub creates a new IGraphicBufferProducer stub backed by
// the given pre-allocated gralloc buffers.
func NewProducerStub(
	width uint32,
	height uint32,
	grallocBufs [4]*gralloc.Buffer,
) *ProducerStub {
	return &ProducerStub{
		width:       width,
		height:      height,
		format:      int32(igbp.PixelFormatYCbCr420_888),
		grallocBufs: grallocBufs,
		queuedCh:    make(chan int, 16),
	}
}

// QueuedFrames returns a channel that receives the slot index each time
// the camera queues a buffer back.
func (g *ProducerStub) QueuedFrames() <-chan int {
	return g.queuedCh
}

// SlotBuffer returns the gralloc buffer for the given slot, or nil if
// the slot has not been assigned yet.
func (g *ProducerStub) SlotBuffer(slot int) *gralloc.Buffer {
	g.mu.Lock()
	defer g.mu.Unlock()
	sb := g.slots[slot]
	if sb == nil {
		return nil
	}
	return sb.gralloc
}

// Descriptor implements binder.NativeTransactionHandler.
func (g *ProducerStub) Descriptor() string {
	return igbp.Descriptor
}

// OnTransaction implements binder.NativeTransactionHandler.
func (g *ProducerStub) OnTransaction(
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
	case igbp.SetMaxDequeuedBufCount,
		igbp.SetAsyncMode,
		igbp.AllowAllocation,
		igbp.SetGenerationNumber,
		igbp.SetDequeueTimeout,
		igbp.SetSharedBufferMode,
		igbp.SetAutoRefresh,
		igbp.SetLegacyBufferDrop,
		igbp.SetAutoPrerotation,
		igbp.DetachBuffer:
		return simpleOKReply()
	case igbp.GetConsumerName:
		return g.onGetConsumerName()
	case igbp.GetUniqueId:
		return g.onGetUniqueId()
	case igbp.GetConsumerUsage:
		return g.onGetConsumerUsage()
	case igbp.AllocateBuffers, igbp.GetFrameTimestamps:
		return nil, nil // void
	case igbp.GetLastQueuedBuffer:
		return g.onGetLastQueuedBuffer()
	default:
		reply := parcel.New()
		reply.WriteInt32(int32(igbp.StatusNoInit))
		return reply, nil
	}
}

func simpleOKReply() (*parcel.Parcel, error) {
	reply := parcel.New()
	reply.WriteInt32(int32(igbp.StatusOK))
	return reply, nil
}
