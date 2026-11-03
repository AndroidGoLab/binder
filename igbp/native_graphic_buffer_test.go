package igbp

import (
	"context"
	"errors"
	"testing"

	"github.com/AndroidGoLab/binder/gralloc"
	"github.com/AndroidGoLab/binder/parcel"
	"github.com/stretchr/testify/require"
)

// TestWriteFlattenedGraphicBufferPreservesMetadata checks the flattened payload.
//
// Example: retain every metadata byte in the parcel.
func TestWriteFlattenedGraphicBufferPreservesMetadata(t *testing.T) {
	raw := make([]byte, 60)
	for i := range raw {
		raw[i] = byte(i)
	}
	p := parcel.New()
	writeFlattenedGraphicBuffer(p, raw, nil)
	p.SetPosition(0)
	size, err := p.ReadInt32()
	require.NoError(t, err)
	require.Equal(t, int32(len(raw)), size)
	count, err := p.ReadInt32()
	require.NoError(t, err)
	require.Zero(t, count)
	got, err := p.Read(len(raw))
	require.NoError(t, err)
	require.Equal(t, raw, got)
}

// failingCPUAccess fails once before completing a pending access.
//
// Example: simulate an unlock failure followed by a successful retry.
type failingCPUAccess struct{ calls int }

// FinishCPUAccess fails on the first call and succeeds on subsequent calls.
//
// Example: exercise a frame lease retained after an unlock failure.
func (f *failingCPUAccess) FinishCPUAccess() error {
	f.calls++
	if f.calls == 1 {
		return errors.New("unlock failed")
	}
	return nil
}

// TestReleaseRetainsSlotWhenFinishCPUAccessFails checks slot ownership on retry.
//
// Example: a failed unlock prevents dequeue until access finishes.
func TestReleaseRetainsSlotWhenFinishCPUAccessFails(t *testing.T) {
	p, err := NewFrameProducer(34, []*gralloc.Buffer{{Width: 1, Height: 1, Stride: 1, Format: 1}})
	require.NoError(t, err)
	p.slots[0].state = frameSlotQueued
	p.slots[0].cpuAccess = &failingCPUAccess{}
	frame := &Frame{producer: p, slot: 0, fence: -1}
	require.ErrorContains(t, frame.Release(context.Background()), "unlock failed")
	request := // Dequeue a slot and return its reply.
		//
		// Example: verify whether a frame's slot is available for reuse.
		func() *parcel.Parcel {
			data := parcel.New()
			data.WriteRawBytes(make([]byte, 20))
			data.WriteBool(false)
			data.SetPosition(0)
			reply, err := p.dequeue(data)
			require.NoError(t, err)
			return reply
		}
	reply := request()
	slot, err := reply.ReadInt32()
	require.NoError(t, err)
	require.Equal(t, int32(-1), slot)
	require.NoError(t, frame.Release(context.Background()))
	reply = request()
	slot, err = reply.ReadInt32()
	require.NoError(t, err)
	require.Equal(t, int32(0), slot)
}
