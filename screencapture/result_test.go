package screencapture

import (
	"context"
	"encoding/binary"
	"testing"

	"github.com/AndroidGoLab/binder/parcel"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// TestCaptureError preserves a failure returned by SurfaceFlinger.
//
// Example: a permission error without a buffer is not a successful frame.
func TestCaptureError(t *testing.T) {
	p := parcel.New()
	p.WriteBool(false)
	p.WriteBool(false)
	p.WriteInt32(-1)
	p.WriteBool(false)
	p.WriteBool(false)
	p.WriteUint32(0)
	_, err := readResult(p)
	require.ErrorContains(t, err, "-1")
}

// TestReadResultOwnsDescriptors checks that both buffer and fence are retained and released.
//
// Example: closing a decoded native frame releases both incoming descriptors.
func TestReadResultOwnsDescriptors(t *testing.T) {
	bufferFDs := []int{0, 0}
	fenceFDs := []int{0, 0}
	require.NoError(t, unix.Pipe(bufferFDs))
	require.NoError(t, unix.Pipe(fenceFDs))
	defer unix.Close(bufferFDs[1])
	defer unix.Close(fenceFDs[1])
	raw := make([]byte, 52)
	binary.LittleEndian.PutUint32(raw, 0x47423031)
	binary.LittleEndian.PutUint32(raw[4:], 1)
	binary.LittleEndian.PutUint32(raw[8:], 1)
	binary.LittleEndian.PutUint32(raw[12:], 1)
	binary.LittleEndian.PutUint32(raw[40:], 1)
	p := parcel.New()
	p.WriteBool(true)
	p.WriteInt32(int32(len(raw)))
	p.WriteInt32(1)
	p.WriteRawBytes(raw)
	p.WriteFileDescriptor(int32(bufferFDs[0]))
	p.WriteBool(true)
	p.WriteInt32(4)
	p.WriteInt32(1)
	p.WriteInt32(0)
	p.WriteFileDescriptor(int32(fenceFDs[0]))
	p.WriteBool(true)
	p.WriteBool(false)
	p.WriteUint32(42)
	frame, err := readResult(p)
	require.NoError(t, err)
	require.True(t, frame.CapturedSecureLayers)
	require.Equal(t, uint32(42), frame.CapturedDataspace)
	require.NoError(t, frame.Close())
	_, err = unix.FcntlInt(uintptr(bufferFDs[0]), unix.F_GETFD, 0)
	require.ErrorIs(t, err, unix.EBADF)
	_, err = unix.FcntlInt(uintptr(fenceFDs[0]), unix.F_GETFD, 0)
	require.ErrorIs(t, err, unix.EBADF)
}

// TestCopyFrameRequiresBuffer refuses to turn an invalid native result into pixels.
//
// Example: a missing buffer is reported as an error.
func TestCopyFrameRequiresBuffer(t *testing.T) {
	_, err := copyFrame(context.Background(), &nativeFrame{FenceFD: -1})
	require.Error(t, err)
}
