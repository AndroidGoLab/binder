package igbp

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"

	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/gralloc"
	"github.com/AndroidGoLab/binder/parcel"
	"golang.org/x/sys/unix"
)

// frameSlotState identifies the owner of a producer buffer slot.
//
// Example: a leased frame cannot be dequeued by SurfaceFlinger.
type frameSlotState int

const (
	// frameSlotFree is available to the display producer.
	//
	// Example: a slot becomes free after its consumer releases it.
	frameSlotFree frameSlotState = iota
	// frameSlotDequeued is owned by SurfaceFlinger.
	//
	// Example: rendering can proceed before queueBuffer.
	frameSlotDequeued
	// frameSlotQueued is owned by the frame consumer.
	//
	// Example: it cannot be returned from dequeueBuffer.
	frameSlotQueued
)

// frameSlot holds a buffer's ownership and cached-buffer state.
//
// Example: requested prevents unnecessary native buffer reallocation.
type frameSlot struct {
	buffer    *gralloc.Buffer
	state     frameSlotState
	requested bool
}

// FrameProducer implements the native producer side of a display frame queue.
// Buffers remain owned by the caller and must outlive this producer and leases.
//
// Example: register it as a Binder stub and pass that stub to a virtual display.
type FrameProducer struct {
	api      int
	mu       sync.Mutex
	slots    []frameSlot
	frames   chan *Frame
	closed   chan struct{}
	isClosed bool
	sequence uint64
	id       uint64
}

// Frame is an exclusive lease on a queued graphics buffer and its acquire fence.
//
// Example: copy pixels and release the frame before it can be reused.
type Frame struct {
	producer *FrameProducer
	slot     int
	fence    int32
	mu       sync.Mutex
	released bool
}

// NewFrameProducer creates an Android 10 or 14 native queue for supplied buffers.
//
// Example: pass buffers allocated for the virtual display's output dimensions.
func NewFrameProducer(api int, buffers []*gralloc.Buffer) (*FrameProducer, error) {
	if api != 29 && api != 34 {
		return nil, fmt.Errorf("unsupported IGraphicBufferProducer API %d", api)
	}
	if len(buffers) == 0 || len(buffers) > MaxBufferSlots {
		return nil, errors.New("invalid buffer count")
	}
	slots := make([]frameSlot, len(buffers))
	for i, b := range buffers {
		if b == nil || b.Width == 0 || b.Height == 0 || b.Stride < int32(b.Width) {
			return nil, fmt.Errorf("invalid buffer %d", i)
		}
		if i > 0 && (b.Width != buffers[0].Width || b.Height != buffers[0].Height || b.Format != buffers[0].Format) {
			return nil, errors.New("buffer dimensions or formats differ")
		}
		slots[i].buffer = b
	}
	return &FrameProducer{api: api, id: uint64(binder.NewLocalBinderIdentity()), slots: slots, frames: make(chan *Frame, len(buffers)), closed: make(chan struct{})}, nil
}

// Descriptor returns the native producer interface name.
//
// Example: Binder validates calls against the producer interface.
func (p *FrameProducer) Descriptor() string { return Descriptor }

// Next waits for a queued frame or context cancellation.
//
// Example: a caller reads a frame and then releases its lease.
func (p *FrameProducer) Next(ctx context.Context) (*Frame, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-p.closed:
		return nil, errors.New("frame producer closed")
	case frame := <-p.frames:
		return frame, nil
	}
}

// CopyRGBA waits for rendering and copies a supported buffer to packed pixels.
//
// Example: copy a frame before releasing it to SurfaceFlinger.
func (f *Frame) CopyRGBA(ctx context.Context) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.released {
		return nil, errors.New("frame released")
	}
	if err := gralloc.WaitFence(ctx, f.fence); err != nil {
		return nil, fmt.Errorf("wait frame fence: %w", err)
	}
	return gralloc.CopyRGBA(f.producer.slots[f.slot].buffer)
}

// Release waits for the producer fence and returns the slot to the queue.
// On failure the frame remains leased, so a caller may retry safely.
//
// Example: release a copied frame without allowing rendering to overwrite it early.
func (f *Frame) Release(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.released {
		return errors.New("frame already released")
	}
	if err := gralloc.WaitFence(ctx, f.fence); err != nil {
		return fmt.Errorf("wait frame fence: %w", err)
	}
	if f.fence >= 0 {
		if err := unix.Close(int(f.fence)); err != nil {
			return fmt.Errorf("close frame fence: %w", err)
		}
		f.fence = -1
	}
	f.producer.mu.Lock()
	defer f.producer.mu.Unlock()
	f.producer.slots[f.slot].state = frameSlotFree
	f.released = true
	return nil
}

// Close stops delivery and releases queued frames. Borrowed frames must be released
// by their callers before their buffers are freed.
//
// Example: destroy the display before closing its producer and allocations.
func (p *FrameProducer) Close(ctx context.Context) error {
	p.mu.Lock()
	if !p.isClosed {
		p.isClosed = true
		close(p.closed)
	}
	p.mu.Unlock()
	var errs []error
	for {
		select {
		case f := <-p.frames:
			if err := f.Release(ctx); err != nil {
				p.frames <- f
				return errors.Join(append(errs, err)...)
			}
		default:
			p.mu.Lock()
			for i := range p.slots {
				if p.slots[i].state != frameSlotFree {
					errs = append(errs, fmt.Errorf("buffer slot %d remains owned", i))
				}
			}
			p.mu.Unlock()
			return errors.Join(errs...)
		}
	}
}

// OnTransaction handles native producer transactions and maintains slot leases.
//
// Example: SurfaceFlinger dequeues, renders, and queues the same buffer repeatedly.
func (p *FrameProducer) OnTransaction(ctx context.Context, code binder.TransactionCode, data *parcel.Parcel) (*parcel.Parcel, error) {
	token, err := p.readToken(data)
	if err != nil {
		return nil, err
	}
	if token != Descriptor {
		return nil, fmt.Errorf("unexpected producer interface %q", token)
	}
	switch code {
	case DequeueBuffer:
		return p.dequeue(data)
	case RequestBuffer:
		return p.request(data)
	case QueueBuffer:
		return p.queue(data)
	case CancelBuffer:
		return p.cancel(ctx, data)
	case Connect:
		return p.connect(data)
	case Query:
		return p.query(data)
	case GetConsumerName:
		r := parcel.New()
		writeString8(r, "GoFrameProducer")
		return r, nil
	case SetMaxDequeuedBufCount, SetAsyncMode, AllowAllocation, SetGenerationNumber, SetDequeueTimeout, SetSharedBufferMode, SetAutoRefresh, SetLegacyBufferDrop, SetAutoPrerotation, Disconnect:
		return simpleOKReply()
	case AllocateBuffers:
		return nil, nil
	case GetFrameTimestamps:
		r := parcel.New()
		WriteEmptyFrameEventHistoryDelta(r)
		return r, nil
	case GetConsumerUsage:
		r := parcel.New()
		r.WriteUint64(p.slots[0].buffer.Usage)
		r.WriteInt32(0)
		return r, nil
	case GetUniqueId:
		r := parcel.New()
		r.WriteUint64(p.id)
		r.WriteInt32(0)
		return r, nil
	default:
		return nil, fmt.Errorf("unsupported producer transaction %d", code)
	}
}

// dequeue leases a free slot to the producer.
//
// Example: unavailable slots return WOULD_BLOCK.
func (p *FrameProducer) dequeue(data *parcel.Parcel) (*parcel.Parcel, error) {
	if _, err := data.Read(20); err != nil {
		return nil, err
	}
	timestamps, err := data.ReadBool()
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	slot := -1
	status := int32(-11)
	if !p.isClosed {
		for i := range p.slots {
			if p.slots[i].state == frameSlotFree {
				slot = i
				p.slots[i].state = frameSlotDequeued
				status = 0
				if !p.slots[i].requested {
					status = BufferNeedsRealloc
				}
				break
			}
		}
	}
	r := parcel.New()
	r.WriteInt32(int32(slot))
	r.WriteInt32(4)
	r.WriteInt32(0)
	r.WriteInt32(0)
	r.WriteUint64(0)
	if timestamps {
		WriteEmptyFrameEventHistoryDelta(r)
	}
	r.WriteInt32(status)
	return r, nil
}

// request serializes the allocated buffer associated with a dequeued slot.
//
// Example: Surface caches the returned native handle for later reuse.
func (p *FrameProducer) request(data *parcel.Parcel) (*parcel.Parcel, error) {
	slot, err := data.ReadInt32()
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if slot < 0 || int(slot) >= len(p.slots) || p.slots[slot].state != frameSlotDequeued {
		return nil, fmt.Errorf("invalid requested slot %d", slot)
	}
	p.slots[slot].requested = true
	r := parcel.New()
	r.WriteInt32(1)
	WriteGrallocGraphicBuffer(r, p.slots[slot].buffer, uint64(slot)+1)
	r.WriteInt32(0)
	return r, nil
}

// readFrameFence extracts a versioned QueueBufferInput fence.
//
// Example: an absent acquire fence is represented by -1.
func (p *FrameProducer) readFrameFence(data *parcel.Parcel) (int32, int32, error) {
	slot, err := data.ReadInt32()
	if err != nil {
		return 0, -1, err
	}
	size, err := data.ReadInt32()
	if err != nil {
		return 0, -1, err
	}
	count, err := data.ReadInt32()
	if err != nil {
		return 0, -1, err
	}
	minimum := int32(57)
	if p.api == 34 {
		minimum = 61
	}
	if size < minimum || count < 0 || count > 1 {
		return 0, -1, fmt.Errorf("invalid queue input size=%d fds=%d", size, count)
	}
	raw, err := data.Read(int(size))
	if err != nil {
		return 0, -1, err
	}
	if binary.LittleEndian.Uint32(raw[45:49]) != uint32(count) {
		return 0, -1, errors.New("invalid queue fence count")
	}
	if count == 0 {
		return slot, -1, nil
	}
	fd, err := data.ReadOwnedFileDescriptor()
	return slot, fd, err
}

// queue publishes a leased frame and returns native queue metadata.
//
// Example: queueing makes a frame visible to Next.
func (p *FrameProducer) queue(data *parcel.Parcel) (*parcel.Parcel, error) {
	slot, fd, err := p.readFrameFence(data)
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.isClosed || slot < 0 || int(slot) >= len(p.slots) || p.slots[slot].state != frameSlotDequeued {
		if fd >= 0 {
			_ = unix.Close(int(fd))
		}
		return nil, fmt.Errorf("invalid queued slot %d", slot)
	}
	p.slots[slot].state = frameSlotQueued
	p.sequence++
	p.frames <- &Frame{producer: p, slot: int(slot), fence: fd}
	return p.queueOutput(), nil
}

// cancel waits for the cancellation fence before reusing its slot.
//
// Example: a canceled GPU write cannot race a subsequent dequeue.
func (p *FrameProducer) cancel(ctx context.Context, data *parcel.Parcel) (*parcel.Parcel, error) {
	slot, err := data.ReadInt32()
	if err != nil {
		return nil, err
	}
	size, err := data.ReadInt32()
	if err != nil {
		return nil, err
	}
	count, err := data.ReadInt32()
	if err != nil {
		return nil, err
	}
	n, err := data.ReadInt32()
	if err != nil {
		return nil, err
	}
	if size != 4 || count < 0 || count > 1 || n != count {
		return nil, errors.New("invalid cancel fence")
	}
	fd := int32(-1)
	if count == 1 {
		fd, err = data.ReadOwnedFileDescriptor()
		if err != nil {
			return nil, err
		}
		defer unix.Close(int(fd))
	}
	if err = gralloc.WaitFence(ctx, fd); err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if slot < 0 || int(slot) >= len(p.slots) || p.slots[slot].state != frameSlotDequeued {
		return nil, fmt.Errorf("invalid canceled slot %d", slot)
	}
	p.slots[slot].state = frameSlotFree
	return simpleOKReply()
}

// connect returns the supported queue geometry.
//
// Example: an output surface queries the consumer's dimensions on connection.
func (p *FrameProducer) connect(data *parcel.Parcel) (*parcel.Parcel, error) {
	has, err := data.ReadInt32()
	if err != nil {
		return nil, err
	}
	if has == 1 {
		if p.api == 29 {
			_, _, err = data.ReadNullableStrongBinderLegacy()
		} else {
			_, _, err = data.ReadNullableStrongBinder()
		}
		if err != nil {
			return nil, err
		}
	} else if has != 0 {
		return nil, errors.New("invalid producer listener")
	}
	if _, err = data.ReadInt32(); err != nil {
		return nil, err
	}
	if _, err = data.ReadInt32(); err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.queueOutput(), nil
}

// query returns supported native-window properties.
//
// Example: SurfaceFlinger discovers the RGBA output format.
func (p *FrameProducer) query(data *parcel.Parcel) (*parcel.Parcel, error) {
	q, err := data.ReadInt32()
	if err != nil {
		return nil, err
	}
	b := p.slots[0].buffer
	var value int32
	switch NativeWindowQuery(q) {
	case NativeWindowWidth, NativeWindowDefaultWidth:
		value = int32(b.Width)
	case NativeWindowHeight, NativeWindowDefaultHeight:
		value = int32(b.Height)
	case NativeWindowFormat:
		value = b.Format
	case NativeWindowMinUndequeued:
		value = 1
	case NativeWindowConcreteType:
		value = int32(NativeWindowSurface)
	case NativeWindowMaxBufferCount:
		value = int32(len(p.slots))
	case NativeWindowQueuesToComposer, NativeWindowTransformHint, NativeWindowConsumerRunning, NativeWindowConsumerUsageBits, NativeWindowStickyTransform, NativeWindowDefaultDataspace, NativeWindowBufferAge:
		value = 0
	default:
		return nil, fmt.Errorf("unsupported native window query %d", q)
	}
	r := parcel.New()
	r.WriteInt32(value)
	r.WriteInt32(0)
	return r, nil
}

// queueOutput serializes the version-specific QueueBufferOutput layout.
//
// Example: API 29 omits the API 34 max-buffer-count and result words.
func (p *FrameProducer) queueOutput() *parcel.Parcel {
	size := 53
	if p.api == 34 {
		size = 61
	}
	payload := make([]byte, size)
	binary.LittleEndian.PutUint32(payload, uint32(p.slots[0].buffer.Width))
	binary.LittleEndian.PutUint32(payload[4:], uint32(p.slots[0].buffer.Height))
	binary.LittleEndian.PutUint64(payload[16:], p.sequence+1)
	if p.api == 34 {
		binary.LittleEndian.PutUint32(payload[25:], uint32(len(p.slots)))
	}
	r := parcel.New()
	r.WriteInt32(int32(size))
	r.WriteInt32(0)
	r.WriteRawBytes(payload)
	r.WriteInt32(0)
	return r
}

// writeString8 serializes a native Android String8.
//
// Example: API 29 producer names use String8 rather than String16.
func writeString8(p *parcel.Parcel, s string) {
	p.WriteInt32(int32(len(s)))
	p.WriteRawBytes(append([]byte(s), 0))
	for (len(s)+1)%4 != 0 {
		p.WriteRawBytes([]byte{0})
		s += " "
	}
}

// readToken decodes the system Binder interface token for the selected API.
//
// Example: Android 10 omits the later vendor header word.
func (p *FrameProducer) readToken(data *parcel.Parcel) (string, error) {
	if p.api != 29 {
		return data.ReadInterfaceToken()
	}
	if _, err := data.ReadInt32(); err != nil {
		return "", err
	}
	if _, err := data.ReadInt32(); err != nil {
		return "", err
	}
	return data.ReadString16()
}
