package igbp

import (
	"context"
	"testing"

	"github.com/AndroidGoLab/binder/gralloc"
	"github.com/AndroidGoLab/binder/parcel"
	"github.com/stretchr/testify/require"
)

// TestNativeGraphicBufferSerialization checks native metadata and descriptor order.
//
// Example: serialize an allocated Android hardware buffer for a producer request.
func TestNativeGraphicBufferSerialization(t *testing.T) {
	buffers, err := gralloc.AllocateRGBA(context.Background(), nil, 29, 8, 8, 1)
	require.NoError(t, err)
	defer // Release the allocation after checking its serialized representation.
	//
	// Example: close descriptors acquired for this test.
	func() { require.NoError(t, gralloc.CloseBuffers(buffers)) }()
	b := buffers[0]
	raw, ok := b.FlattenedGraphicBuffer()
	require.True(t, ok)
	require.NotEmpty(t, raw)
	require.NotEmpty(t, b.Handle.Fds)
	p := parcel.New()
	WriteGrallocGraphicBuffer(p, b, 0xdeadbeef)
	p.SetPosition(0)
	size, err := p.ReadInt32()
	require.NoError(t, err)
	require.Equal(t, int32(len(raw)), size)
	count, err := p.ReadInt32()
	require.NoError(t, err)
	require.Equal(t, int32(len(b.Handle.Fds)), count)
	got, err := p.Read(len(raw))
	require.NoError(t, err)
	require.Equal(t, raw, got)
	for _, want := range b.Handle.Fds {
		fd, err := p.ReadFileDescriptor()
		require.NoError(t, err)
		require.Equal(t, want, fd)
	}
}
