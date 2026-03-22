package igbp

import (
	"github.com/AndroidGoLab/binder/igbp"
	"github.com/AndroidGoLab/binder/parcel"
)

func (g *ProducerStub) onRequestBuffer(
	data *parcel.Parcel,
) (*parcel.Parcel, error) {
	slot, _ := data.ReadInt32()

	g.mu.Lock()
	buf := g.slots[slot]
	g.mu.Unlock()

	reply := parcel.New()
	if buf == nil {
		reply.WriteInt32(0) // nonNull=0
		reply.WriteInt32(int32(igbp.StatusOK))
		return reply, nil
	}

	reply.WriteInt32(1) // nonNull=1

	bufID := uint64(0xCAFE0000) | uint64(slot)
	WriteGrallocGraphicBuffer(reply, buf.gralloc, bufID)

	reply.WriteInt32(int32(igbp.StatusOK))
	return reply, nil
}

func (g *ProducerStub) onDequeueBuffer(
	data *parcel.Parcel,
) (*parcel.Parcel, error) {
	_, _ = data.ReadUint32()  // w
	_, _ = data.ReadUint32()  // h
	_, _ = data.ReadInt32()   // format
	_, _ = data.ReadUint64()  // usage
	getTimestamps, _ := data.ReadBool()

	g.mu.Lock()
	slot := g.nextSlot
	g.nextSlot = (g.nextSlot + 1) % 4

	needsRealloc := false
	if g.slots[slot] == nil {
		needsRealloc = true
		g.slots[slot] = &slotBuffer{
			gralloc: g.grallocBufs[slot],
		}
	}
	g.mu.Unlock()

	reply := parcel.New()
	reply.WriteInt32(int32(slot))

	// Fence as Flattenable: flattenedSize=4, fdCount=0, numFds=0.
	reply.WriteInt32(4)
	reply.WriteInt32(0)
	reply.WriteUint32(0)

	// bufferAge.
	reply.WriteUint64(0)

	if getTimestamps {
		WriteEmptyFrameEventHistoryDelta(reply)
	}

	if needsRealloc {
		reply.WriteInt32(int32(igbp.BufferNeedsRealloc))
	} else {
		reply.WriteInt32(int32(igbp.StatusOK))
	}
	return reply, nil
}

func (g *ProducerStub) onQueueBuffer(
	data *parcel.Parcel,
) (*parcel.Parcel, error) {
	slot, _ := data.ReadInt32()

	select {
	case g.queuedCh <- int(slot):
	default:
	}

	reply := parcel.New()
	WriteQueueBufferOutput(reply)
	reply.WriteInt32(int32(igbp.StatusOK))
	return reply, nil
}

func (g *ProducerStub) onCancelBuffer(
	data *parcel.Parcel,
) (*parcel.Parcel, error) {
	_, _ = data.ReadInt32()
	reply := parcel.New()
	reply.WriteInt32(int32(igbp.StatusOK))
	return reply, nil
}

func (g *ProducerStub) onQuery(
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
	}

	reply := parcel.New()
	reply.WriteInt32(value)
	reply.WriteInt32(int32(igbp.StatusOK))
	return reply, nil
}

func (g *ProducerStub) onConnect(
	data *parcel.Parcel,
) (*parcel.Parcel, error) {
	hasListener, _ := data.ReadInt32()
	if hasListener == 1 {
		_, _ = data.ReadStrongBinder()
	}
	_, _ = data.ReadInt32() // api
	_, _ = data.ReadInt32() // producerControlled

	reply := parcel.New()
	WriteQueueBufferOutput(reply)
	reply.WriteInt32(int32(igbp.StatusOK))
	return reply, nil
}

func (g *ProducerStub) onDisconnect(
	data *parcel.Parcel,
) (*parcel.Parcel, error) {
	_, _ = data.ReadInt32()
	reply := parcel.New()
	reply.WriteInt32(int32(igbp.StatusOK))
	return reply, nil
}

func (g *ProducerStub) onGetConsumerName() (*parcel.Parcel, error) {
	reply := parcel.New()
	reply.WriteString16("GoCamera")
	return reply, nil
}

func (g *ProducerStub) onGetUniqueId() (*parcel.Parcel, error) {
	reply := parcel.New()
	reply.WriteUint64(0x12345678)
	reply.WriteInt32(int32(igbp.StatusOK))
	return reply, nil
}

func (g *ProducerStub) onGetConsumerUsage() (*parcel.Parcel, error) {
	reply := parcel.New()
	reply.WriteUint64(0)
	reply.WriteInt32(int32(igbp.StatusOK))
	return reply, nil
}

func (g *ProducerStub) onGetLastQueuedBuffer() (*parcel.Parcel, error) {
	reply := parcel.New()
	reply.WriteInt32(int32(igbp.StatusNoInit))
	return reply, nil
}
