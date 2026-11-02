package igbp

import (
	"context"
	"encoding/binary"
	"testing"
	"time"

	"github.com/AndroidGoLab/binder/gralloc"
	"github.com/AndroidGoLab/binder/parcel"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// TestFrameProducerLeasePreventsReuse verifies producer slots stay owned until released.
//
// Example: a queued image cannot be overwritten while its consumer uses it.
func TestFrameProducerLeasePreventsReuse(t *testing.T) {
	ctx := context.Background()
	p, err := NewFrameProducer(34, []*gralloc.Buffer{{Width: 1, Height: 1, Stride: 1, Format: 1}})
	require.NoError(t, err)
	defer func() { c, cancel := context.WithCancel(ctx); cancel(); _ = p.Close(c) }()
	dequeue := func() (int32, int32) {
		req := parcel.New()
		req.WriteInterfaceToken(Descriptor)
		req.WriteUint32(1)
		req.WriteUint32(1)
		req.WriteInt32(1)
		req.WriteUint64(0)
		req.WriteBool(false)
		req.SetPosition(0)
		reply, err := p.OnTransaction(ctx, DequeueBuffer, req)
		require.NoError(t, err)
		slot, err := reply.ReadInt32()
		require.NoError(t, err)
		_, err = reply.Read(20)
		require.NoError(t, err)
		status, err := reply.ReadInt32()
		require.NoError(t, err)
		return slot, status
	}
	slot, status := dequeue()
	require.Equal(t, int32(0), slot)
	require.Equal(t, int32(BufferNeedsRealloc), status)
	reqBuffer := parcel.New()
	reqBuffer.WriteInterfaceToken(Descriptor)
	reqBuffer.WriteInt32(slot)
	reqBuffer.SetPosition(0)
	_, err = p.OnTransaction(ctx, RequestBuffer, reqBuffer)
	require.NoError(t, err)
	req := parcel.New()
	req.WriteInterfaceToken(Descriptor)
	req.WriteInt32(slot)
	req.WriteInt32(61)
	req.WriteInt32(0)
	req.WriteRawBytes(make([]byte, 61))
	req.SetPosition(0)
	_, err = p.OnTransaction(ctx, QueueBuffer, req)
	require.NoError(t, err)
	frame, err := p.Next(ctx)
	require.NoError(t, err)
	slot, status = dequeue()
	require.Equal(t, int32(-1), slot)
	require.Less(t, status, int32(0))
	require.NoError(t, frame.Release(ctx))
	slot, status = dequeue()
	require.Equal(t, int32(0), slot)
	require.Equal(t, int32(0), status)
}

// TestFrameProducerAndroid10Token verifies native Android 10 callback decoding.
//
// Example: queue setup accepts the legacy interface token and replies in its wire format.
func TestFrameProducerAndroid10Token(t *testing.T) {
	p, err := NewFrameProducer(29, []*gralloc.Buffer{{Width: 1, Height: 1, Stride: 1, Format: 1}})
	require.NoError(t, err)
	req := parcel.New()
	req.WriteInterfaceToken(Descriptor)
	req.WriteInt32(int32(NativeWindowFormat))
	req = req.Android10Request()
	req.SetPosition(0)
	reply, err := p.OnTransaction(context.Background(), Query, req)
	require.NoError(t, err)
	format, err := reply.ReadInt32()
	require.NoError(t, err)
	require.Equal(t, int32(1), format)
}

// TestFrameProducerCloseDetectsOutstandingLease prevents freeing live buffers.
//
// Example: a shutdown must wait until the consumer releases its frame.
func TestFrameProducerCloseDetectsOutstandingLease(t *testing.T) {
	ctx := context.Background()
	p, err := NewFrameProducer(34, []*gralloc.Buffer{{Width: 1, Height: 1, Stride: 1, Format: 1}})
	require.NoError(t, err)
	req := parcel.New()
	req.WriteInterfaceToken(Descriptor)
	req.WriteUint32(1)
	req.WriteUint32(1)
	req.WriteInt32(1)
	req.WriteUint64(0)
	req.WriteBool(false)
	req.SetPosition(0)
	_, err = p.OnTransaction(ctx, DequeueBuffer, req)
	require.NoError(t, err)
	req = parcel.New()
	req.WriteInterfaceToken(Descriptor)
	req.WriteInt32(0)
	req.WriteInt32(61)
	req.WriteInt32(0)
	req.WriteRawBytes(make([]byte, 61))
	req.SetPosition(0)
	_, err = p.OnTransaction(ctx, QueueBuffer, req)
	require.NoError(t, err)
	frame, err := p.Next(ctx)
	require.NoError(t, err)
	closeCtx, cancelClose := context.WithCancel(ctx)
	cancelClose()
	require.Error(t, p.Close(closeCtx))
	require.NoError(t, frame.Release(ctx))
	require.NoError(t, p.Close(ctx))
}

// TestFrameProducerFenceRetainsSlot checks that a canceled fence wait retains ownership.
//
// Example: a frame can be released after its rendering fence signals.
func TestFrameProducerFenceRetainsSlot(t *testing.T) {
	ctx := context.Background()
	p, err := NewFrameProducer(34, []*gralloc.Buffer{{Width: 1, Height: 1, Stride: 1, Format: 1}})
	require.NoError(t, err)
	req := parcel.New()
	req.WriteInterfaceToken(Descriptor)
	req.WriteUint32(1)
	req.WriteUint32(1)
	req.WriteInt32(1)
	req.WriteUint64(0)
	req.WriteBool(false)
	req.SetPosition(0)
	_, err = p.OnTransaction(ctx, DequeueBuffer, req)
	require.NoError(t, err)
	var fds [2]int
	require.NoError(t, unix.Pipe(fds[:]))
	defer unix.Close(fds[1])
	raw := make([]byte, 61)
	binary.LittleEndian.PutUint32(raw[45:], 1)
	req = parcel.New()
	req.WriteInterfaceToken(Descriptor)
	req.WriteInt32(0)
	req.WriteInt32(int32(len(raw)))
	req.WriteInt32(1)
	req.WriteRawBytes(raw)
	req.WriteFileDescriptor(int32(fds[0]))
	req.SetPosition(0)
	_, err = p.OnTransaction(ctx, QueueBuffer, req)
	require.NoError(t, err)
	frame, err := p.Next(ctx)
	require.NoError(t, err)
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	require.ErrorIs(t, frame.Release(cancelled), context.Canceled)
	closeCtx, cancelClose := context.WithCancel(ctx)
	cancelClose()
	require.Error(t, p.Close(closeCtx))
	_, err = unix.Write(fds[1], []byte{1})
	require.NoError(t, err)
	require.NoError(t, frame.Release(ctx))
	require.NoError(t, unix.Close(fds[0]))
	require.NoError(t, p.Close(ctx))
}

// TestFrameProducerNameUsesString8 verifies the native consumer-name wire format.
//
// Example: both Android 10 and Android 14 expect UTF-8 bytes.
func TestFrameProducerNameUsesString8(t *testing.T) {
	p, err := NewFrameProducer(34, []*gralloc.Buffer{{Width: 1, Height: 1, Stride: 1, Format: 1}})
	require.NoError(t, err)
	req := parcel.New()
	req.WriteInterfaceToken(Descriptor)
	req.SetPosition(0)
	reply, err := p.OnTransaction(context.Background(), GetConsumerName, req)
	require.NoError(t, err)
	n, err := reply.ReadInt32()
	require.NoError(t, err)
	raw, err := reply.Read(int(n))
	require.NoError(t, err)
	require.Equal(t, "GoFrameProducer", string(raw))
}

// TestFrameProducerIDsDiffer ensures separate queues are distinguishable.
//
// Example: two virtual displays expose different native queue IDs.
func TestFrameProducerIDsDiffer(t *testing.T) {
	buffers := []*gralloc.Buffer{{Width: 1, Height: 1, Stride: 1, Format: 1}}
	a, err := NewFrameProducer(34, buffers)
	require.NoError(t, err)
	b, err := NewFrameProducer(34, buffers)
	require.NoError(t, err)
	id := func(p *FrameProducer) uint64 {
		req := parcel.New()
		req.WriteInterfaceToken(Descriptor)
		req.SetPosition(0)
		r, err := p.OnTransaction(context.Background(), GetUniqueId, req)
		require.NoError(t, err)
		v, err := r.ReadUint64()
		require.NoError(t, err)
		return v
	}
	require.NotEqual(t, id(a), id(b))
}

// TestFrameProducerTimestampsReply checks the native empty timestamp delta.
//
// Example: a native client can decode a frame history with no events.
func TestFrameProducerTimestampsReply(t *testing.T) {
	p, err := NewFrameProducer(34, []*gralloc.Buffer{{Width: 1, Height: 1, Stride: 1, Format: 1}})
	require.NoError(t, err)
	req := parcel.New()
	req.WriteInterfaceToken(Descriptor)
	req.SetPosition(0)
	reply, err := p.OnTransaction(context.Background(), GetFrameTimestamps, req)
	require.NoError(t, err)
	require.NotNil(t, reply)
	size, err := reply.ReadInt32()
	require.NoError(t, err)
	require.Equal(t, int32(28), size)
	count, err := reply.ReadInt32()
	require.NoError(t, err)
	require.Zero(t, count)
	raw, err := reply.Read(28)
	require.NoError(t, err)
	require.Equal(t, make([]byte, 28), raw)
}

// TestFrameProducerSlowReaderResumes verifies backpressure and slot recovery.
//
// Example: a slow consumer holds both buffers until it releases one frame.
func TestFrameProducerSlowReaderResumes(t *testing.T) {
	ctx := context.Background()
	b := func() *gralloc.Buffer { return &gralloc.Buffer{Width: 1, Height: 1, Stride: 1, Format: 1} }
	p, err := NewFrameProducer(34, []*gralloc.Buffer{b(), b()})
	require.NoError(t, err)
	dequeue := func() int32 {
		req := parcel.New()
		req.WriteInterfaceToken(Descriptor)
		req.WriteUint32(1)
		req.WriteUint32(1)
		req.WriteInt32(1)
		req.WriteUint64(0)
		req.WriteBool(false)
		req.SetPosition(0)
		reply, err := p.OnTransaction(ctx, DequeueBuffer, req)
		require.NoError(t, err)
		slot, err := reply.ReadInt32()
		require.NoError(t, err)
		return slot
	}
	enqueue := func(slot int32) {
		req := parcel.New()
		req.WriteInterfaceToken(Descriptor)
		req.WriteInt32(slot)
		req.WriteInt32(61)
		req.WriteInt32(0)
		req.WriteRawBytes(make([]byte, 61))
		req.SetPosition(0)
		_, err := p.OnTransaction(ctx, QueueBuffer, req)
		require.NoError(t, err)
	}
	a, bslot := dequeue(), dequeue()
	require.Equal(t, int32(0), a)
	require.Equal(t, int32(1), bslot)
	enqueue(a)
	enqueue(bslot)
	require.Equal(t, int32(-1), dequeue())
	frame, err := p.Next(ctx)
	require.NoError(t, err)
	require.NoError(t, frame.Release(ctx))
	require.Equal(t, a, dequeue())
}

// TestFrameProducerCloseRetriesQueuedFence preserves buffers after canceled cleanup.
//
// Example: a later cleanup succeeds after the queued frame's fence signals.
func TestFrameProducerCloseRetriesQueuedFence(t *testing.T) {
	ctx := context.Background()
	p, err := NewFrameProducer(34, []*gralloc.Buffer{{Width: 1, Height: 1, Stride: 1, Format: 1}})
	require.NoError(t, err)
	req := parcel.New()
	req.WriteInterfaceToken(Descriptor)
	req.WriteUint32(1)
	req.WriteUint32(1)
	req.WriteInt32(1)
	req.WriteUint64(0)
	req.WriteBool(false)
	req.SetPosition(0)
	_, err = p.OnTransaction(ctx, DequeueBuffer, req)
	require.NoError(t, err)
	var fds [2]int
	require.NoError(t, unix.Pipe(fds[:]))
	defer unix.Close(fds[0])
	defer unix.Close(fds[1])
	raw := make([]byte, 61)
	binary.LittleEndian.PutUint32(raw[45:], 1)
	req = parcel.New()
	req.WriteInterfaceToken(Descriptor)
	req.WriteInt32(0)
	req.WriteInt32(int32(len(raw)))
	req.WriteInt32(1)
	req.WriteRawBytes(raw)
	req.WriteFileDescriptor(int32(fds[0]))
	req.SetPosition(0)
	_, err = p.OnTransaction(ctx, QueueBuffer, req)
	require.NoError(t, err)
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	require.ErrorIs(t, p.Close(canceled), context.Canceled)
	_, err = unix.Write(fds[1], []byte{1})
	require.NoError(t, err)
	require.NoError(t, p.Close(ctx))
}

// TestFrameProducerCancelFenceKeepsSlotWhenWaitCanceled prevents unsafe reuse.
//
// Example: a stalled producer fence leaves its buffer quarantined.
func TestFrameProducerCancelFenceKeepsSlotWhenWaitCanceled(t *testing.T) {
	ctx := context.Background()
	p, err := NewFrameProducer(34, []*gralloc.Buffer{{Width: 1, Height: 1, Stride: 1, Format: 1}})
	require.NoError(t, err)
	req := parcel.New()
	req.WriteInterfaceToken(Descriptor)
	req.WriteUint32(1)
	req.WriteUint32(1)
	req.WriteInt32(1)
	req.WriteUint64(0)
	req.WriteBool(false)
	req.SetPosition(0)
	_, err = p.OnTransaction(ctx, DequeueBuffer, req)
	require.NoError(t, err)
	var fds [2]int
	require.NoError(t, unix.Pipe(fds[:]))
	defer unix.Close(fds[0])
	defer unix.Close(fds[1])
	req = parcel.New()
	req.WriteInterfaceToken(Descriptor)
	req.WriteInt32(0)
	req.WriteInt32(4)
	req.WriteInt32(1)
	req.WriteInt32(1)
	req.WriteFileDescriptor(int32(fds[0]))
	req.SetPosition(0)
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	_, err = p.OnTransaction(canceled, CancelBuffer, req)
	require.ErrorIs(t, err, context.Canceled)
	closeCtx, cancelClose := context.WithCancel(ctx)
	cancelClose()
	require.Error(t, p.Close(closeCtx))
}

// TestFrameProducerCloseWaitsForLateQueue checks asynchronous display teardown.
//
// Example: a final frame queued during shutdown releases its buffer safely.
func TestFrameProducerCloseWaitsForLateQueue(t *testing.T) {
	ctx := context.Background()
	p, err := NewFrameProducer(34, []*gralloc.Buffer{{Width: 1, Height: 1, Stride: 1, Format: 1}})
	require.NoError(t, err)
	req := parcel.New()
	req.WriteInterfaceToken(Descriptor)
	req.WriteUint32(1)
	req.WriteUint32(1)
	req.WriteInt32(1)
	req.WriteUint64(0)
	req.WriteBool(false)
	req.SetPosition(0)
	_, err = p.OnTransaction(ctx, DequeueBuffer, req)
	require.NoError(t, err)
	closeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- p.Close(closeCtx) }()
	<-p.closed
	req = parcel.New()
	req.WriteInterfaceToken(Descriptor)
	req.WriteInt32(0)
	req.WriteInt32(61)
	req.WriteInt32(0)
	req.WriteRawBytes(make([]byte, 61))
	req.SetPosition(0)
	_, err = p.OnTransaction(ctx, QueueBuffer, req)
	require.NoError(t, err)
	require.NoError(t, <-done)
}

// TestFrameCopyReleasesSignaledFence avoids a second fence wait on release.
//
// Example: even if pixel decoding fails, a signaled fence is closed promptly.
func TestFrameCopyReleasesSignaledFence(t *testing.T) {
	ctx := context.Background()
	p, err := NewFrameProducer(34, []*gralloc.Buffer{{Width: 1, Height: 1, Stride: 1, Format: 1}})
	require.NoError(t, err)
	var fds [2]int
	require.NoError(t, unix.Pipe(fds[:]))
	defer unix.Close(fds[1])
	_, err = unix.Write(fds[1], []byte{1})
	require.NoError(t, err)
	p.slots[0].state = frameSlotQueued
	frame := &Frame{producer: p, slot: 0, fence: int32(fds[0])}
	_, err = frame.CopyRGBA(ctx)
	require.Error(t, err)
	_, err = unix.FcntlInt(uintptr(fds[0]), unix.F_GETFD, 0)
	require.ErrorIs(t, err, unix.EBADF)
	require.NoError(t, frame.Release(ctx))
}

// TestFrameProducerTryNextReturnsOnlyAvailableFrames checks nonblocking acquisition.
//
// Example: a frame-skipping consumer can drain the queue without waiting.
func TestFrameProducerTryNextReturnsOnlyAvailableFrames(t *testing.T) {
	ctx := context.Background()
	p, err := NewFrameProducer(34, []*gralloc.Buffer{{Width: 1, Height: 1, Stride: 1, Format: 1}})
	require.NoError(t, err)
	frame, ok := p.TryNext()
	require.False(t, ok)
	require.Nil(t, frame)
	req := parcel.New()
	req.WriteInterfaceToken(Descriptor)
	req.WriteUint32(1)
	req.WriteUint32(1)
	req.WriteInt32(1)
	req.WriteUint64(0)
	req.WriteBool(false)
	req.SetPosition(0)
	_, err = p.OnTransaction(ctx, DequeueBuffer, req)
	require.NoError(t, err)
	req = parcel.New()
	req.WriteInterfaceToken(Descriptor)
	req.WriteInt32(0)
	req.WriteInt32(61)
	req.WriteInt32(0)
	req.WriteRawBytes(make([]byte, 61))
	req.SetPosition(0)
	_, err = p.OnTransaction(ctx, QueueBuffer, req)
	require.NoError(t, err)
	frame, ok = p.TryNext()
	require.True(t, ok)
	require.NotNil(t, frame)
	_, ok = p.TryNext()
	require.False(t, ok)
	require.NoError(t, frame.Release(ctx))
	require.NoError(t, p.Close(ctx))
	frame, ok = p.TryNext()
	require.False(t, ok)
	require.Nil(t, frame)
}

// TestFrameCopyIntoHonorsCancellation preserves the lease and destination.
//
// Example: a canceled frame read leaves its output and slot untouched.
func TestFrameCopyIntoHonorsCancellation(t *testing.T) {
	p, err := NewFrameProducer(34, []*gralloc.Buffer{{Width: 1, Height: 1, Stride: 1, Format: 1}})
	require.NoError(t, err)
	p.slots[0].state = frameSlotQueued
	frame := &Frame{producer: p, slot: 0, fence: -1}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	dst := []byte{1, 2, 3, 4}
	require.ErrorIs(t, frame.CopyRGBAInto(ctx, dst), context.Canceled)
	require.Equal(t, []byte{1, 2, 3, 4}, dst)
	require.NoError(t, frame.Release(context.Background()))
}
