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
	// frameSlotCanceled retains a producer fence until it signals.
	//
	// Example: a canceled GPU write cannot race the next dequeue.
	frameSlotCanceled
	// frameSlotWaiting reserves a canceled fence for shutdown cleanup.
	//
	// Example: concurrent dequeues cannot take a fence being waited on.
	frameSlotWaiting
)

// cpuAccessFinisher completes outstanding buffer access before slot reuse.
//
// Example: finish a failed unlock before returning a frame to the producer.
type cpuAccessFinisher interface {
	// FinishCPUAccess ends any outstanding access or reports why it cannot.
	//
	// Example: retry an unlock that failed during frame copying.
	FinishCPUAccess() error
}

// frameSlot holds a buffer's ownership and cached-buffer state.
//
// Example: requested prevents unnecessary native buffer reallocation.
type frameSlot struct {
	buffer    *gralloc.Buffer
	id        uint64
	reader    *gralloc.RGBAReader
	cpuAccess cpuAccessFinisher
	state     frameSlotState
	requested bool
	fence     int32
}

// FrameProducer implements the native producer side of a display frame queue.
// Buffers remain owned by the caller and must outlive this producer and leases.
//
// Example: register it as a Binder stub and pass that stub to a virtual display.
type FrameProducer struct {
	api      int
	closing  chan struct{}
	mu       sync.Mutex
	slots    []frameSlot
	frames   chan *Frame
	closed   chan struct{}
	changed  chan struct{}
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
		id, err := graphicBufferIDs.next()
		if err != nil {
			return nil, fmt.Errorf("allocating buffer ID: %w", err)
		}
		slots[i].buffer = b
		slots[i].cpuAccess = b
		slots[i].id = id
	}
	return &FrameProducer{api: api, id: uint64(binder.NewLocalBinderIdentity()), slots: slots, frames: make(chan *Frame, len(buffers)), closed: make(chan struct{}), changed: make(chan struct{}, 1), closing: make(chan struct{}, 1)}, nil
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

// TryNext acquires a queued frame without waiting. The returned frame retains
// its slot and fence until the caller releases it, including when it is skipped.
//
// Example: drain queued frames before encoding the latest one.
func (p *FrameProducer) TryNext() (*Frame, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.isClosed {
		return nil, false
	}
	select {
	case frame := <-p.frames:
		return frame, true
	default:
		return nil, false
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
	if err := f.waitFence(ctx); err != nil {
		return nil, fmt.Errorf("wait frame fence: %w", err)
	}
	return gralloc.CopyRGBA(f.producer.slots[f.slot].buffer)
}

// CopyRGBAInto waits for rendering and copies pixels into a caller-owned slice.
// The destination must contain exactly width*height*4 bytes; it may be partly
// modified if an error is returned. The frame remains leased until Release.
//
// Example: reuse one RGBA slice for successive display frames.
func (f *Frame) CopyRGBAInto(ctx context.Context, pixels []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.released {
		return errors.New("frame released")
	}
	if err := f.waitFence(ctx); err != nil {
		return fmt.Errorf("wait frame fence: %w", err)
	}
	slot := &f.producer.slots[f.slot]
	if slot.reader == nil {
		reader, err := gralloc.NewRGBAReader(slot.buffer)
		if err != nil {
			return err
		}
		slot.reader = reader
	}
	return slot.reader.CopyInto(pixels)
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
	if err := f.waitFence(ctx); err != nil {
		return fmt.Errorf("wait frame fence: %w", err)
	}
	if err := f.producer.slots[f.slot].cpuAccess.FinishCPUAccess(); err != nil {
		return fmt.Errorf("finish frame CPU access: %w", err)
	}
	f.producer.mu.Lock()
	defer f.producer.mu.Unlock()
	f.producer.slots[f.slot].state = frameSlotFree
	f.released = true
	f.producer.signalChange()
	return nil
}

// waitFence waits once and releases the frame's owned acquire fence.
// The caller holds the frame mutex.
//
// Example: a copied frame can be released without polling the fence again.
func (f *Frame) waitFence(ctx context.Context) error {
	if err := gralloc.WaitFence(ctx, f.fence); err != nil {
		return err
	}
	if f.fence < 0 {
		return nil
	}
	fd := f.fence
	f.fence = -1
	if err := unix.Close(int(fd)); err != nil {
		return fmt.Errorf("close fence: %w", err)
	}
	return nil
}

// Close stops delivery and releases queued frames. Borrowed frames must be released
// by their callers before their buffers are freed.
//
// Example: destroy the display before closing its producer and allocations.
func (p *FrameProducer) Close(ctx context.Context) error {
	select {
	case p.closing <- struct{}{}:
	default:
		select {
		case p.closing <- struct{}{}:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	defer // Allow another shutdown attempt after this one returns.
	//
	// Example: retry cleanup after a timed-out lease.
	func() { <-p.closing }()
	p.mu.Lock()
	if !p.isClosed {
		p.isClosed = true
		close(p.closed)
	}
	p.mu.Unlock()
	for {
		select {
		case f := <-p.frames:
			if err := f.Release(ctx); err != nil {
				p.frames <- f
				return err
			}
		default:
			p.mu.Lock()
			owned := false
			pending := -1
			var pendingFD int32
			for i := range p.slots {
				if p.slots[i].state != frameSlotFree {
					owned = true
				}
				if pending < 0 && p.slots[i].state == frameSlotCanceled {
					pending = i
					pendingFD = p.slots[i].fence
					p.slots[i].state = frameSlotWaiting
				}
			}
			p.mu.Unlock()
			if pending >= 0 {
				err := gralloc.WaitFence(ctx, pendingFD)
				p.mu.Lock()
				if err != nil {
					p.slots[pending].state = frameSlotCanceled
					p.signalChange()
					p.mu.Unlock()
					return fmt.Errorf("wait canceled buffer fence: %w", err)
				}
				closeErr := unix.Close(int(pendingFD))
				p.slots[pending].fence = -1
				p.slots[pending].state = frameSlotFree
				p.signalChange()
				p.mu.Unlock()
				if closeErr != nil {
					return fmt.Errorf("close canceled buffer fence: %w", closeErr)
				}
				continue
			}
			if !owned {
				var errs []error
				for i := range p.slots {
					if p.slots[i].reader != nil {
						if err := p.slots[i].reader.Close(); err != nil {
							errs = append(errs, err)
						} else {
							p.slots[i].reader = nil
						}
					}
				}
				return errors.Join(errs...)
			}
			select {
			case <-ctx.Done():
				return fmt.Errorf("waiting for buffer ownership: %w", ctx.Err())
			case <-p.changed:
			}
		}
	}
}

// signalChange wakes a shutdown waiting for slot ownership to change.
//
// Example: a late producer callback wakes Close after the display is destroyed.
func (p *FrameProducer) signalChange() {
	select {
	case p.changed <- struct{}{}:
	default:
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
		return p.cancel(data)
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
			if p.slots[i].state == frameSlotCanceled {
				fds := []unix.PollFd{{Fd: p.slots[i].fence, Events: unix.POLLIN}}
				for {
					_, err := unix.Poll(fds, 0)
					if errors.Is(err, unix.EINTR) {
						continue
					}
					if err != nil {
						return nil, fmt.Errorf("poll canceled buffer fence: %w", err)
					}
					break
				}
				if fds[0].Revents&(unix.POLLERR|unix.POLLHUP|unix.POLLNVAL) != 0 {
					return nil, fmt.Errorf("canceled buffer fence poll failed: %#x", fds[0].Revents)
				}
				if fds[0].Revents&unix.POLLIN != 0 {
					fd := p.slots[i].fence
					p.slots[i].fence = -1
					p.slots[i].state = frameSlotFree
					if err := unix.Close(int(fd)); err != nil {
						return nil, fmt.Errorf("close canceled buffer fence: %w", err)
					}
				}
			}
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
	WriteGrallocGraphicBuffer(r, p.slots[slot].buffer, p.slots[slot].id)
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
	if slot < 0 || int(slot) >= len(p.slots) || p.slots[slot].state != frameSlotDequeued {
		if fd >= 0 {
			_ = unix.Close(int(fd))
		}
		return nil, fmt.Errorf("invalid queued slot %d", slot)
	}
	p.slots[slot].state = frameSlotQueued
	p.sequence++
	p.frames <- &Frame{producer: p, slot: int(slot), fence: fd}
	p.signalChange()
	return p.queueOutput(), nil
}

// cancel retains the cancellation fence without blocking the Binder callback.
//
// Example: a canceled GPU write cannot race a subsequent dequeue.
func (p *FrameProducer) cancel(data *parcel.Parcel) (*parcel.Parcel, error) {
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
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if slot < 0 || int(slot) >= len(p.slots) || p.slots[slot].state != frameSlotDequeued {
		if fd >= 0 {
			return nil, errors.Join(fmt.Errorf("invalid canceled slot %d", slot), unix.Close(int(fd)))
		}
		return nil, fmt.Errorf("invalid canceled slot %d", slot)
	}
	if fd >= 0 {
		p.slots[slot].state = frameSlotCanceled
		p.slots[slot].fence = fd
	} else {
		p.slots[slot].state = frameSlotFree
	}
	p.signalChange()
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
