//go:build linux

package kernelbinder

import (
	"context"
	"encoding/binary"
	"errors"
	"testing"
	"unsafe"

	"github.com/AndroidGoLab/binder/parcel"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// closeReplyTestFD closes a descriptor unless the test already closed it.
//
// Example: release pipe ends after an early test failure.
func closeReplyTestFD(
	t *testing.T,
	fd int,
) {
	t.Helper()
	if fd < 0 {
		return
	}
	if err := unix.Close(fd); err != nil && !errors.Is(err, unix.EBADF) {
		t.Errorf("close test fd: %v", err)
	}
}

// TestReplyParcelClosesUnreadDescriptor covers descriptor ownership on BR_REPLY.
//
// Example: recycling an undecoded reply releases the delivered FD.
func TestReplyParcelClosesUnreadDescriptor(t *testing.T) {
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	require.NoError(t, err)
	// Close pipe ends that remain open after the assertion.
	//
	// Example: release the peer even when the reply closes its descriptor.
	t.Cleanup(func() { closeReplyTestFD(t, fds[0]); closeReplyTestFD(t, fds[1]) })
	p := parcel.New()
	p.WriteFileDescriptor(int32(fds[0]))
	mapped := make([]byte, len(p.Data())+8)
	copy(mapped, p.Data())
	binary.LittleEndian.PutUint64(mapped[len(p.Data()):], p.Objects()[0])
	d := &Driver{fd: -1, mapped: mapped}
	txn := binderTransactionData{dataSize: uint64(len(p.Data())), offsetsSize: 8, dataBuffer: uint64(uintptr(unsafe.Pointer(&mapped[0]))), offsetsBuffer: uint64(uintptr(unsafe.Pointer(&mapped[len(p.Data())])))}
	raw := make([]byte, 4+int(unsafe.Sizeof(txn)))
	binary.LittleEndian.PutUint32(raw, uint32(brReply))
	copy(raw[4:], unsafe.Slice((*byte)(unsafe.Pointer(&txn)), int(unsafe.Sizeof(txn))))
	reply, got, _, err := d.parseReadBuffer(context.Background(), raw)
	require.NoError(t, err)
	require.True(t, got)
	require.NoError(t, reply.Recycle())
	_, err = unix.FcntlInt(uintptr(fds[0]), unix.F_GETFD, 0)
	require.ErrorIs(t, err, unix.EBADF)
	fds[0] = -1
	_, err = unix.FcntlInt(uintptr(fds[1]), unix.F_GETFD, 0)
	require.NoError(t, err)
}

// TestReplyParcelTransfersReadDescriptor verifies ownership leaves the parcel.
//
// Example: a returned native result retains its buffer after reply recycling.
func TestReplyParcelTransfersReadDescriptor(t *testing.T) {
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	require.NoError(t, err)
	// Release the socket pair after checking transfer.
	//
	// Example: close the transferred descriptor when the test ends.
	t.Cleanup(func() { closeReplyTestFD(t, fds[0]); closeReplyTestFD(t, fds[1]) })
	p := parcel.New()
	p.WriteFileDescriptor(int32(fds[0]))
	mapped := make([]byte, len(p.Data())+8)
	copy(mapped, p.Data())
	binary.LittleEndian.PutUint64(mapped[len(p.Data()):], p.Objects()[0])
	d := &Driver{fd: -1, mapped: mapped}
	txn := binderTransactionData{dataSize: uint64(len(p.Data())), offsetsSize: 8, dataBuffer: uint64(uintptr(unsafe.Pointer(&mapped[0]))), offsetsBuffer: uint64(uintptr(unsafe.Pointer(&mapped[len(p.Data())])))}
	raw := make([]byte, 4+int(unsafe.Sizeof(txn)))
	binary.LittleEndian.PutUint32(raw, uint32(brReply))
	copy(raw[4:], unsafe.Slice((*byte)(unsafe.Pointer(&txn)), int(unsafe.Sizeof(txn))))
	reply, got, _, err := d.parseReadBuffer(context.Background(), raw)
	require.NoError(t, err)
	require.True(t, got)
	fd, err := reply.ReadOwnedFileDescriptor()
	require.NoError(t, err)
	require.Equal(t, int32(fds[0]), fd)
	require.NoError(t, reply.Recycle())
	_, err = unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
	require.NoError(t, err)
}

// TestReplyParcelRejectsUnreadableOffsets avoids returning borrowed FDs.
//
// Example: a truncated offsets array cannot establish reply ownership.
func TestReplyParcelRejectsUnreadableOffsets(t *testing.T) {
	mapped := make([]byte, 4)
	d := &Driver{fd: -1, mapped: mapped}
	txn := binderTransactionData{dataSize: 4, offsetsSize: 8, dataBuffer: uint64(uintptr(unsafe.Pointer(&mapped[0]))), offsetsBuffer: uint64(uintptr(unsafe.Pointer(&mapped[0]))) + 4}
	raw := make([]byte, 4+int(unsafe.Sizeof(txn)))
	binary.LittleEndian.PutUint32(raw, uint32(brReply))
	copy(raw[4:], unsafe.Slice((*byte)(unsafe.Pointer(&txn)), int(unsafe.Sizeof(txn))))
	reply, got, _, err := d.parseReadBuffer(context.Background(), raw)
	require.True(t, got)
	require.Nil(t, reply)
	require.ErrorContains(t, err, "copy reply offsets")
}

// TestReplyParcelPreservesHIDLFDArray verifies the separate scatter-gather lifecycle.
//
// Example: recycling a reply does not close an FDA descriptor duplicate.
func TestReplyParcelPreservesHIDLFDArray(t *testing.T) {
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	require.NoError(t, err)
	// Release the source descriptors after the scatter-gather check.
	//
	// Example: close the peer after checking the duplicated FD.
	t.Cleanup(func() { closeReplyTestFD(t, fds[0]); closeReplyTestFD(t, fds[1]) })
	// dataSize holds one PTR and one FDA object in the simulated reply.
	//
	// Example: place the FDA object directly after the PTR object.
	const dataSize = 72
	mapped := make([]byte, dataSize+16+4)
	binary.LittleEndian.PutUint32(mapped[0:], uint32(binderTypePTR))
	binary.LittleEndian.PutUint64(mapped[8:], uint64(uintptr(unsafe.Pointer(&mapped[dataSize+16]))))
	binary.LittleEndian.PutUint64(mapped[16:], 4)
	binary.LittleEndian.PutUint32(mapped[40:], uint32(binderTypeFDA))
	binary.LittleEndian.PutUint64(mapped[48:], 1)
	binary.LittleEndian.PutUint64(mapped[dataSize:], 0)
	binary.LittleEndian.PutUint64(mapped[dataSize+8:], 40)
	binary.LittleEndian.PutUint32(mapped[dataSize+16:], uint32(fds[0]))
	d := &Driver{fd: -1, mapped: mapped}
	txn := binderTransactionData{dataSize: dataSize, offsetsSize: 16, dataBuffer: uint64(uintptr(unsafe.Pointer(&mapped[0]))), offsetsBuffer: uint64(uintptr(unsafe.Pointer(&mapped[dataSize])))}
	raw := make([]byte, 4+int(unsafe.Sizeof(txn)))
	binary.LittleEndian.PutUint32(raw, uint32(brReply))
	copy(raw[4:], unsafe.Slice((*byte)(unsafe.Pointer(&txn)), int(unsafe.Sizeof(txn))))
	reply, got, _, err := d.parseReadBuffer(context.Background(), raw)
	require.NoError(t, err)
	require.True(t, got)
	parent := binary.LittleEndian.Uint64(reply.Data()[8:])
	require.LessOrEqual(t, parent+4, uint64(len(reply.Data())))
	dup := int(int32(binary.LittleEndian.Uint32(reply.Data()[parent:])))
	require.NotEqual(t, fds[0], dup)
	// Release the scatter-gather duplicate after the test.
	//
	// Example: a successful FDA copy leaves one owned descriptor.
	t.Cleanup(func() { closeReplyTestFD(t, dup) })
	require.NoError(t, reply.Recycle())
	_, err = unix.FcntlInt(uintptr(dup), unix.F_GETFD, 0)
	require.NoError(t, err)
}
