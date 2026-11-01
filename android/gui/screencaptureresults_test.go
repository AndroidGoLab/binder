package gui

import (
	"context"
	"encoding/binary"
	"golang.org/x/sys/unix"
	"testing"

	"github.com/AndroidGoLab/binder/parcel"
	"github.com/stretchr/testify/require"
)

// TestScreenCaptureResultsRejectsMalformedFence ensures invalid fence layouts fail.
//
// Example: a fence declaring zero bytes and zero descriptors is not a valid fence.
func TestScreenCaptureResultsRejectsMalformedFence(t *testing.T) {
	p := parcel.New()
	p.WriteBool(false)
	p.WriteBool(true)
	p.WriteInt32(0)
	p.WriteInt32(0)
	p.WriteBool(false)
	p.WriteBool(false)
	p.WriteUint32(0)
	p.SetPosition(0)
	var result ScreenCaptureResults
	require.Error(t, result.UnmarshalParcel(p))
}

// TestScreenCaptureResultsOwnsDescriptors checks fence and both buffers survive decoding and are closed.
//
// Example: a capture includes a gain map and a completion fence.
func TestScreenCaptureResultsOwnsDescriptors(t *testing.T) {
	fds := make([]int, 4)
	require.NoError(t, unix.Pipe(fds[:2]))
	require.NoError(t, unix.Pipe(fds[2:]))
	t.Cleanup(func() {
		for _, fd := range fds {
			_ = unix.Close(fd)
		}
	})
	p := parcel.New()
	p.WriteBool(true)
	writeGraphicBuffer(p, int32(fds[0]))
	p.WriteBool(true)
	p.WriteInt32(4)
	p.WriteInt32(1)
	p.WriteInt32(0)
	p.WriteFileDescriptor(int32(fds[1]))
	p.WriteBool(true)
	p.WriteBool(false)
	p.WriteUint32(42)
	p.WriteBool(true)
	writeGraphicBuffer(p, int32(fds[2]))
	p.WriteFloat32(1.5)
	p.SetPosition(0)
	var result ScreenCaptureResults
	require.NoError(t, result.UnmarshalParcel(p))
	require.Equal(t, int32(fds[1]), result.FenceFD)
	require.Equal(t, int32(fds[0]), result.Buffer.Handle.Fds[0])
	require.Equal(t, int32(fds[2]), result.GainMap.Handle.Fds[0])
	require.True(t, result.CapturedSecureLayers)
	require.False(t, result.CapturedHdrLayers)
	require.Equal(t, uint32(42), result.CapturedDataspace)
	require.Equal(t, float32(1.5), result.HdrSdrRatio)
	require.NoError(t, result.Close())
	require.NoError(t, result.Close())
	for _, fd := range fds[:3] {
		_, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
		require.ErrorIs(t, err, unix.EBADF)
	}
	_, err := unix.FcntlInt(uintptr(fds[3]), unix.F_GETFD, 0)
	require.NoError(t, err)
}

// writeGraphicBuffer writes one flattened buffer with a descriptor.
//
// Example: include a buffer or gain map in a test result.
func writeGraphicBuffer(p *parcel.Parcel, fd int32) {
	raw := make([]byte, 52)
	binary.LittleEndian.PutUint32(raw, 0x47423031)
	binary.LittleEndian.PutUint32(raw[40:], 1)
	p.WriteInt32(int32(len(raw)))
	p.WriteInt32(1)
	p.WriteRawBytes(raw)
	p.WriteFileDescriptor(fd)
}

// TestScreenCaptureResultsClosesPartialResult checks descriptors are released on later truncation.
//
// Example: metadata is missing after a received buffer and fence.
func TestScreenCaptureResultsClosesPartialResult(t *testing.T) {
	fds := make([]int, 2)
	require.NoError(t, unix.Pipe(fds))
	t.Cleanup(func() {
		for _, fd := range fds {
			_ = unix.Close(fd)
		}
	})
	p := parcel.New()
	p.WriteBool(true)
	writeGraphicBuffer(p, int32(fds[0]))
	p.WriteBool(true)
	p.WriteInt32(4)
	p.WriteInt32(1)
	p.WriteInt32(0)
	p.WriteFileDescriptor(int32(fds[1]))
	p.SetPosition(0)
	var result ScreenCaptureResults
	require.Error(t, result.UnmarshalParcel(p))
	require.Nil(t, result.Buffer)
	require.Equal(t, int32(-1), result.FenceFD)
	for _, fd := range fds {
		_, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
		require.ErrorIs(t, err, unix.EBADF)
	}
}

// TestScreenCaptureResultsPreservesStatus checks old results without a fence.
//
// Example: SurfaceFlinger reports a failed capture using a nonzero status.
func TestScreenCaptureResultsPreservesStatus(t *testing.T) {
	p := parcel.New()
	p.WriteBool(false)
	p.WriteBool(false)
	p.WriteInt32(-5)
	p.WriteBool(false)
	p.WriteBool(true)
	p.WriteUint32(7)
	p.SetPosition(0)
	var result ScreenCaptureResults
	require.NoError(t, result.UnmarshalParcel(p))
	require.Equal(t, int32(-5), result.FenceStatus)
	require.Equal(t, int32(-1), result.FenceFD)
	require.Nil(t, result.GainMap)
	require.NoError(t, result.Close())
}

// TestScreenCaptureResultsCopiesShareOwnership protects reused descriptors from a second close.
//
// Example: a generated listener passes a result by value and both copies are closed.
func TestScreenCaptureResultsCopiesShareOwnership(t *testing.T) {
	fds := make([]int, 2)
	require.NoError(t, unix.Pipe(fds))
	defer unix.Close(fds[1])
	p := parcel.New()
	p.WriteBool(false)
	p.WriteBool(true)
	p.WriteInt32(4)
	p.WriteInt32(1)
	p.WriteInt32(0)
	p.WriteFileDescriptor(int32(fds[0]))
	p.WriteBool(false)
	p.WriteBool(false)
	p.WriteUint32(0)
	p.SetPosition(0)
	var result ScreenCaptureResults
	require.NoError(t, result.UnmarshalParcel(p))
	copy := result
	require.NoError(t, result.Close())
	require.NoError(t, unix.Dup3(fds[1], fds[0], unix.O_CLOEXEC))
	defer unix.Close(fds[0])
	require.NoError(t, copy.Close())
	_, err := unix.FcntlInt(uintptr(fds[0]), unix.F_GETFD, 0)
	require.NoError(t, err)
}

// TestScreenCaptureResultsDecodeReportsCleanupError retains both errors.
//
// Example: a truncated result holds a descriptor closed unexpectedly by its caller.
func TestScreenCaptureResultsDecodeReportsCleanupError(t *testing.T) {
	fds := make([]int, 2)
	require.NoError(t, unix.Pipe(fds))
	defer unix.Close(fds[1])
	p := parcel.New()
	p.WriteBool(true)
	writeGraphicBuffer(p, int32(fds[0]))
	p.SetPosition(0)
	require.NoError(t, unix.Close(fds[0]))
	var result ScreenCaptureResults
	err := result.UnmarshalParcel(p)
	require.ErrorContains(t, err, "reading hasFence")
	require.ErrorIs(t, err, unix.EBADF)
}

// TestScreenCaptureResultsCopyRGBARejectsClosedCopy avoids using released handles.
//
// Example: one callback copy closes a result before another requests pixels.
func TestScreenCaptureResultsCopyRGBARejectsClosedCopy(t *testing.T) {
	p := parcel.New()
	p.WriteBool(false)
	p.WriteBool(false)
	p.WriteInt32(0)
	p.WriteBool(false)
	p.WriteBool(false)
	p.WriteUint32(0)
	p.SetPosition(0)
	var result ScreenCaptureResults
	require.NoError(t, result.UnmarshalParcel(p))
	copy := result
	require.NoError(t, result.Close())
	_, err := copy.CopyRGBA(context.Background())
	require.ErrorContains(t, err, "closed")
}

// TestScreenCaptureResultsCopyRGBAWaitsForFence protects pixels from premature reads.
//
// Example: a canceled capture stops waiting before its fence signals.
func TestScreenCaptureResultsCopyRGBAWaitsForFence(t *testing.T) {
	bufferFDs := make([]int, 2)
	fenceFDs := make([]int, 2)
	require.NoError(t, unix.Pipe(bufferFDs))
	require.NoError(t, unix.Pipe(fenceFDs))
	defer unix.Close(bufferFDs[1])
	defer unix.Close(fenceFDs[1])
	p := parcel.New()
	p.WriteBool(true)
	writeGraphicBuffer(p, int32(bufferFDs[0]))
	p.WriteBool(true)
	p.WriteInt32(4)
	p.WriteInt32(1)
	p.WriteInt32(0)
	p.WriteFileDescriptor(int32(fenceFDs[0]))
	p.WriteBool(false)
	p.WriteBool(false)
	p.WriteUint32(0)
	p.SetPosition(0)
	var result ScreenCaptureResults
	require.NoError(t, result.UnmarshalParcel(p))
	defer result.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := result.CopyRGBA(ctx)
	require.ErrorIs(t, err, context.Canceled)
	_, err = unix.FcntlInt(uintptr(fenceFDs[0]), unix.F_GETFD, 0)
	require.NoError(t, err)
}
