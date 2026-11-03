package hidlalloc

import (
	"context"
	"encoding/binary"
	"testing"

	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/parcel"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

type allocationTransport struct {
	binder.Transport
	reply *parcel.Parcel
}

func (t allocationTransport) Transact(context.Context, uint32, binder.TransactionCode, binder.TransactionFlags, *parcel.Parcel) (*parcel.Parcel, error) {
	return t.reply, nil
}

func TestAllocateTransfersReceivedDescriptors(t *testing.T) {
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	require.NoError(t, err)
	defer unix.Close(fds[0])
	defer unix.Close(fds[1])
	const handleOffset = 164
	data := make([]byte, handleOffset+16)
	for _, pos := range []int{12, 52, 92} {
		binary.LittleEndian.PutUint32(data[pos:], binderTypePTR)
	}
	binary.LittleEndian.PutUint64(data[100:], handleOffset)
	binary.LittleEndian.PutUint64(data[108:], 16)
	binary.LittleEndian.PutUint32(data[handleOffset:], 12)
	binary.LittleEndian.PutUint32(data[handleOffset+4:], 1)
	binary.LittleEndian.PutUint32(data[handleOffset+12:], uint32(fds[0]))
	reply := parcel.FromReceivedBytes(data, nil, handleOffset+12)
	result, err := Allocate(context.Background(), allocationTransport{reply: reply}, 1, 1, 1, 1, 1, 1, 1)
	require.NoError(t, err)
	require.Equal(t, []int32{int32(fds[0])}, result.Fds)
	_, err = unix.FcntlInt(uintptr(result.Fds[0]), unix.F_GETFD, 0)
	require.NoError(t, err)
}

func TestAllocateClosesTransferredDescriptorsOnFailure(t *testing.T) {
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	require.NoError(t, err)
	defer unix.Close(fds[0])
	defer unix.Close(fds[1])
	const handleOffset = 164
	data := make([]byte, handleOffset+20)
	for _, pos := range []int{12, 52, 92} {
		binary.LittleEndian.PutUint32(data[pos:], binderTypePTR)
	}
	binary.LittleEndian.PutUint64(data[100:], handleOffset)
	binary.LittleEndian.PutUint64(data[108:], 20)
	binary.LittleEndian.PutUint32(data[handleOffset:], 12)
	binary.LittleEndian.PutUint32(data[handleOffset+4:], 2)
	binary.LittleEndian.PutUint32(data[handleOffset+12:], uint32(fds[0]))
	binary.LittleEndian.PutUint32(data[handleOffset+16:], uint32(fds[1]))
	reply := parcel.FromReceivedBytes(data, nil, handleOffset+12)
	result, err := Allocate(context.Background(), allocationTransport{reply: reply}, 1, 1, 1, 1, 1, 1, 1)
	require.Error(t, err)
	require.Nil(t, result)
	_, err = unix.FcntlInt(uintptr(fds[0]), unix.F_GETFD, 0)
	require.ErrorIs(t, err, unix.EBADF)
	_, err = unix.FcntlInt(uintptr(fds[1]), unix.F_GETFD, 0)
	require.NoError(t, err)
}

func TestParseAllocateResponseReadsAllocatorError(t *testing.T) {
	raw := make([]byte, 12)
	binary.LittleEndian.PutUint32(raw[4:], 1)
	binary.LittleEndian.PutUint32(raw[8:], 64)

	result, err := parseAllocateResponse(parcel.FromReceivedBytes(raw, nil))
	require.NoError(t, err)
	require.Equal(t, int32(1), result.Error)
	require.Equal(t, int32(64), result.Stride)
}

func TestParseAllocateResponseRejectsTransportError(t *testing.T) {
	raw := make([]byte, 12)
	binary.LittleEndian.PutUint32(raw, 1)

	_, err := parseAllocateResponse(parcel.FromReceivedBytes(raw, nil))
	require.ErrorContains(t, err, "HIDL allocator transport status: 1")
}

func TestAllocateDescriptorValidatesRequest(t *testing.T) {
	_, err := AllocateDescriptor(context.Background(), nil, 0, nil, 1)
	require.ErrorContains(t, err, "empty buffer descriptor")

	_, err = AllocateDescriptor(context.Background(), nil, 0, []uint32{1}, 0)
	require.ErrorContains(t, err, "buffer count must be positive")
}
