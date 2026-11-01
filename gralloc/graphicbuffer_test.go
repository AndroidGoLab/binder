package gralloc

import (
	"encoding/binary"
	"errors"
	"testing"

	"github.com/AndroidGoLab/binder/parcel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// graphicParcel creates a flattened GraphicBuffer header for decoding tests.
//
// Example: create a handle with one descriptor and no integer payload.
func graphicParcel(
	fds int32,
	ints int32,
	data []byte,
) *parcel.Parcel {
	raw := make([]byte, 52+len(data))
	binary.LittleEndian.PutUint32(raw, graphicBufferMagic)
	binary.LittleEndian.PutUint32(raw[40:], uint32(fds))
	binary.LittleEndian.PutUint32(raw[44:], uint32(ints))
	copy(raw[52:], data)
	p := parcel.New()
	p.WriteInt32(int32(len(raw)))
	p.WriteInt32(fds)
	p.WriteRawBytes(raw)
	return p
}

// TestReadGraphicBufferReportsCloseFailure retains errors from partial cleanup.
//
// Example: a repeated received FD causes a second close to report EBADF.
func TestReadGraphicBufferReportsCloseFailure(t *testing.T) {
	fds := make([]int, 2)
	require.NoError(t, unix.Pipe(fds))
	defer unix.Close(fds[1])
	p := graphicParcel(3, 0, nil)
	p.WriteFileDescriptor(int32(fds[0]))
	p.WriteFileDescriptor(int32(fds[0]))
	p = parcel.FromReceivedBytes(p.Data(), p.Objects())
	_, err := ReadGraphicBuffer(p)
	require.ErrorIs(t, err, unix.EBADF)
}

// TestReadGraphicBufferRejectsTruncatedInts checks handle length validation.
//
// Example: reject a header that declares more words than are present.
func TestReadGraphicBufferRejectsTruncatedInts(t *testing.T) {
	p := graphicParcel(0, 2, make([]byte, 4))
	_, err := ReadGraphicBuffer(p)
	require.Error(t, err)
}

// TestReadGraphicBufferClosesPartialFDs checks cleanup after a truncated handle.
//
// Example: a missing second FD closes the first FD already read.
func TestReadGraphicBufferClosesPartialFDs(t *testing.T) {
	fds := make([]int, 2)
	require.NoError(t, unix.Pipe(fds))
	defer unix.Close(fds[1])
	p := graphicParcel(2, 0, nil)
	p.WriteFileDescriptor(int32(fds[0]))
	p = parcel.FromReceivedBytes(p.Data(), p.Objects())
	_, err := ReadGraphicBuffer(p)
	require.Error(t, err)
	_, err = unix.FcntlInt(uintptr(fds[0]), unix.F_GETFD, 0)
	require.ErrorIs(t, err, unix.EBADF)
}

// TestReadGraphicBufferPreservesBorrowedPartialFD checks failure ownership.
//
// Example: a truncated borrowed parcel must not close the source FD.
func TestReadGraphicBufferPreservesBorrowedPartialFD(t *testing.T) {
	fds := make([]int, 2)
	require.NoError(t, unix.Pipe(fds))
	defer unix.Close(fds[0])
	defer unix.Close(fds[1])
	p := graphicParcel(2, 0, nil)
	p.WriteFileDescriptor(int32(fds[0]))
	_, err := ReadGraphicBuffer(p)
	require.Error(t, err)
	_, err = unix.FcntlInt(uintptr(fds[0]), unix.F_GETFD, 0)
	require.NoError(t, err)
}

// TestReadOwnedGraphicBufferDuplicatesBorrowedFD checks independent ownership.
//
// Example: closing a decoded buffer leaves its source descriptor open.
func TestReadOwnedGraphicBufferDuplicatesBorrowedFD(t *testing.T) {
	fds := make([]int, 2)
	require.NoError(t, unix.Pipe(fds))
	defer unix.Close(fds[0])
	defer unix.Close(fds[1])
	p := graphicParcel(1, 0, nil)
	p.WriteFileDescriptor(int32(fds[0]))
	buf, err := ReadOwnedGraphicBuffer(p)
	require.NoError(t, err)
	require.NotEqual(t, int32(fds[0]), buf.Handle.Fds[0])
	flags, err := unix.FcntlInt(uintptr(buf.Handle.Fds[0]), unix.F_GETFD, 0)
	require.NoError(t, err)
	require.NotZero(t, flags&unix.FD_CLOEXEC)
	require.NoError(t, unix.Close(int(buf.Handle.Fds[0])))
	_, err = unix.FcntlInt(uintptr(fds[0]), unix.F_GETFD, 0)
	require.NoError(t, err)
}

// TestReadGraphicBufferReturnsBorrowedFD preserves successful legacy reads.
//
// Example: a locally constructed parcel returns its original descriptor.
func TestReadGraphicBufferReturnsBorrowedFD(t *testing.T) {
	fds := make([]int, 2)
	require.NoError(t, unix.Pipe(fds))
	defer unix.Close(fds[0])
	defer unix.Close(fds[1])
	p := graphicParcel(1, 0, nil)
	p.WriteFileDescriptor(int32(fds[0]))
	buf, err := ReadGraphicBuffer(p)
	require.NoError(t, err)
	require.Equal(t, int32(fds[0]), buf.Handle.Fds[0])
	_, err = unix.FcntlInt(uintptr(fds[0]), unix.F_GETFD, 0)
	require.NoError(t, err)
}

// TestReadOwnedGraphicBufferTransfersReceivedFD preserves incoming ownership.
//
// Example: the caller owns the original received FD after closing its parcel.
func TestReadOwnedGraphicBufferTransfersReceivedFD(t *testing.T) {
	fds := make([]int, 2)
	require.NoError(t, unix.Pipe(fds))
	defer unix.Close(fds[0])
	defer unix.Close(fds[1])
	wire := graphicParcel(1, 0, nil)
	wire.WriteFileDescriptor(int32(fds[0]))
	p := parcel.FromReceivedBytes(wire.Data(), wire.Objects())
	buf, err := ReadOwnedGraphicBuffer(p)
	require.NoError(t, err)
	require.Equal(t, int32(fds[0]), buf.Handle.Fds[0])
	require.NoError(t, p.Close())
	_, err = unix.FcntlInt(uintptr(fds[0]), unix.F_GETFD, 0)
	require.NoError(t, err)
}

// TestReadOwnedGraphicBufferFailurePreservesBorrowedFD checks partial cleanup.
//
// Example: failure after duplicating one FD leaves the parcel's source intact.
func TestReadOwnedGraphicBufferFailurePreservesBorrowedFD(t *testing.T) {
	fds := make([]int, 2)
	require.NoError(t, unix.Pipe(fds))
	sourceOpen := true
	defer // Close the source if an earlier assertion stops the test.
	//
	// Example: a failed decode assertion still releases the test pipe.
	func() {
		if sourceOpen {
			err := unix.Close(fds[0])
			if !errors.Is(err, unix.EBADF) {
				assert.NoError(t, err)
			}
		}
	}()
	defer unix.Close(fds[1])
	p := graphicParcel(2, 0, nil)
	p.WriteFileDescriptor(int32(fds[0]))
	_, err := ReadOwnedGraphicBuffer(p)
	require.Error(t, err)
	_, err = unix.FcntlInt(uintptr(fds[0]), unix.F_GETFD, 0)
	require.NoError(t, err)
	require.NoError(t, unix.Close(fds[0]))
	sourceOpen = false
	_, err = unix.Write(fds[1], []byte{1})
	require.ErrorIs(t, err, unix.EPIPE)
}
