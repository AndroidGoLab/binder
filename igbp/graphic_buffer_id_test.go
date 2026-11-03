package igbp

import (
	"context"
	"encoding/binary"
	"os"
	"sync"
	"testing"

	"github.com/AndroidGoLab/binder/gralloc"
	"github.com/AndroidGoLab/binder/parcel"
	"github.com/stretchr/testify/require"
)

// TestGraphicBufferIDGeneratorConcurrent verifies uniqueness and the synthetic namespace.
//
// Example: concurrent producer creation cannot assign the same wrapper ID.
func TestGraphicBufferIDGeneratorConcurrent(t *testing.T) {
	var generator graphicBufferIDGenerator
	const count = 256
	ids := make(chan uint64, count)
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, err := generator.next()
			if err != nil {
				t.Error(err)
				return
			}
			ids <- id
		}()
	}
	wg.Wait()
	close(ids)
	seen := make(map[uint64]bool)
	for id := range ids {
		require.Equal(t, uint64(1)<<63|uint64(os.Getpid())<<32, id&0xffffffff00000000)
		require.NotZero(t, uint32(id))
		require.False(t, seen[id])
		seen[id] = true
	}
	require.Len(t, seen, count)
}

// TestGraphicBufferIDGeneratorExhaustion verifies that the sequence never wraps.
//
// Example: an exhausted producer cannot reuse a previously issued ID.
func TestGraphicBufferIDGeneratorExhaustion(t *testing.T) {
	var generator graphicBufferIDGenerator
	generator.counter.Store(^uint32(0) - 1)
	id, err := generator.next()
	require.NoError(t, err)
	require.Equal(t, ^uint32(0), uint32(id))
	for i := 0; i < 2; i++ {
		id, err = generator.next()
		require.Error(t, err)
		require.Zero(t, id)
	}
}

// TestGraphicBufferIDs distinguishes producer instances and preserves slot identities.
//
// Example: reopening a display does not reuse an earlier native buffer ID.
func TestGraphicBufferIDs(t *testing.T) {
	ids := make(map[uint64]bool)
	for _, api := range []int{29, 34} {
		for instance := 0; instance < 2; instance++ {
			p, err := NewFrameProducer(api, []*gralloc.Buffer{{Width: 1, Height: 1, Stride: 1}, {Width: 1, Height: 1, Stride: 1}})
			require.NoError(t, err)
			for slot := range p.slots {
				p.slots[slot].state = frameSlotDequeued
				read := func() uint64 {
					req := parcel.New()
					req.WriteInterfaceToken(Descriptor)
					req.WriteInt32(int32(slot))
					if api == 29 {
						req = req.Android10Request()
					}
					req.SetPosition(0)
					reply, err := p.OnTransaction(context.Background(), RequestBuffer, req)
					require.NoError(t, err)
					_, err = reply.Read(12)
					require.NoError(t, err)
					raw, err := reply.Read(36)
					require.NoError(t, err)
					return uint64(binary.LittleEndian.Uint32(raw[28:]))<<32 | uint64(binary.LittleEndian.Uint32(raw[32:]))
				}
				id := read()
				require.Equal(t, id, read())
				require.False(t, ids[id])
				ids[id] = true
			}
		}
	}
	for instance := 0; instance < 2; instance++ {
		p := NewProducerStub(1, 1, [4]*gralloc.Buffer{{Width: 1, Height: 1, Stride: 1}, {Width: 1, Height: 1, Stride: 1}})
		for slot := 0; slot < 2; slot++ {
			req := parcel.New()
			req.WriteUint32(1)
			req.WriteUint32(1)
			req.WriteInt32(1)
			req.WriteUint64(0)
			req.WriteBool(false)
			req.SetPosition(0)
			_, err := p.onDequeueBuffer(req)
			require.NoError(t, err)
			read := func() uint64 {
				req := parcel.New()
				req.WriteInt32(int32(slot))
				req.SetPosition(0)
				reply, err := p.onRequestBuffer(req)
				require.NoError(t, err)
				_, err = reply.Read(12)
				require.NoError(t, err)
				raw, err := reply.Read(36)
				require.NoError(t, err)
				return uint64(binary.LittleEndian.Uint32(raw[28:]))<<32 | uint64(binary.LittleEndian.Uint32(raw[32:]))
			}
			id := read()
			require.Equal(t, id, read())
			require.False(t, ids[id])
			ids[id] = true
		}
	}
}
