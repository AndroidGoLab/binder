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

func setTestTopology(data []byte, nativeSize, fds uint64) {
	binary.LittleEndian.PutUint64(data[20:], 172)
	binary.LittleEndian.PutUint64(data[28:], 16)
	binary.LittleEndian.PutUint32(data[56:], 1)
	binary.LittleEndian.PutUint64(data[60:], 192)
	binary.LittleEndian.PutUint64(data[68:], 16)
	binary.LittleEndian.PutUint64(data[92:], nativeSize)
	binary.LittleEndian.PutUint32(data[104:], 1)
	binary.LittleEndian.PutUint64(data[124:], 1)
	binary.LittleEndian.PutUint32(data[140:], 0x66646185)
	binary.LittleEndian.PutUint64(data[148:], fds)
	binary.LittleEndian.PutUint64(data[156:], 2)
	binary.LittleEndian.PutUint64(data[164:], 12)
	binary.LittleEndian.PutUint32(data[180:], 1)
}

func TestAllocateTransfersReceivedDescriptors(t *testing.T) {
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	require.NoError(t, err)
	defer unix.Close(fds[0])
	defer unix.Close(fds[1])
	const handleOffset = 208
	data := make([]byte, handleOffset+16)
	for _, pos := range []int{12, 52, 100} {
		binary.LittleEndian.PutUint32(data[pos:], binderTypePTR)
	}
	binary.LittleEndian.PutUint64(data[108:], handleOffset)
	binary.LittleEndian.PutUint64(data[116:], 16)
	binary.LittleEndian.PutUint32(data[handleOffset:], 12)
	binary.LittleEndian.PutUint32(data[handleOffset+4:], 1)
	binary.LittleEndian.PutUint32(data[handleOffset+12:], uint32(fds[0]))
	setTestTopology(data, 16, 1)
	reply := parcel.FromReceivedBytes(data, []uint64{12, 52, 100, 140}, handleOffset+12)
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
	const handleOffset = 208
	data := make([]byte, handleOffset+20)
	for _, pos := range []int{12, 52, 100} {
		binary.LittleEndian.PutUint32(data[pos:], binderTypePTR)
	}
	binary.LittleEndian.PutUint64(data[108:], handleOffset)
	binary.LittleEndian.PutUint64(data[116:], 20)
	binary.LittleEndian.PutUint32(data[handleOffset:], 12)
	binary.LittleEndian.PutUint32(data[handleOffset+4:], 2)
	binary.LittleEndian.PutUint32(data[handleOffset+12:], uint32(fds[0]))
	binary.LittleEndian.PutUint32(data[handleOffset+16:], uint32(fds[1]))
	setTestTopology(data, 20, 2)
	reply := parcel.FromReceivedBytes(data, []uint64{12, 52, 100, 140}, handleOffset+12)
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

	result, err := ParseAllocateResponse(parcel.FromReceivedBytes(raw, nil))
	require.NoError(t, err)
	require.Equal(t, int32(1), result.Error)
	require.Equal(t, int32(64), result.Stride)
}

func TestParseAllocateResponseRejectsTransportError(t *testing.T) {
	raw := make([]byte, 12)
	binary.LittleEndian.PutUint32(raw, 1)

	_, err := ParseAllocateResponse(parcel.FromReceivedBytes(raw, nil))
	require.ErrorContains(t, err, "HIDL allocator transport status: 1")
}

func TestAllocateDescriptorValidatesRequest(t *testing.T) {
	_, err := AllocateDescriptor(context.Background(), nil, 0, nil, 1)
	require.ErrorContains(t, err, "empty buffer descriptor")

	_, err = AllocateDescriptor(context.Background(), nil, 0, []uint32{1}, 0)
	require.ErrorContains(t, err, "buffer count must be positive")
}

func TestAllocateRejectsMultipleBuffers(t *testing.T) {
	for _, allocate := range []struct {
		name string
		call func() (*AllocateResult, error)
	}{
		{"descriptor", func() (*AllocateResult, error) {
			return AllocateDescriptor(context.Background(), allocationTransport{reply: parcel.FromReceivedBytes(make([]byte, 12), nil)}, 0, []uint32{1}, 2)
		}},
		{"legacy", func() (*AllocateResult, error) {
			return Allocate(context.Background(), allocationTransport{reply: parcel.FromReceivedBytes(make([]byte, 12), nil)}, 0, 1, 1, 1, 1, 1, 2)
		}},
	} {
		t.Run(allocate.name, func(t *testing.T) {
			result, err := allocate.call()
			require.ErrorContains(t, err, "only one buffer")
			require.Nil(t, result)
		})
	}
}

// A successful allocation must contain registered Binder objects, not merely
// object-shaped bytes embedded in the reply.
func TestParseAllocateResponseRejectsUnregisteredObjects(t *testing.T) {
	raw := make([]byte, 188)
	for _, off := range []int{12, 52, 100} {
		binary.LittleEndian.PutUint32(raw[off:], binderTypePTR)
	}
	binary.LittleEndian.PutUint64(raw[108:], 172)
	binary.LittleEndian.PutUint64(raw[116:], 16)
	binary.LittleEndian.PutUint32(raw[172:], 12)
	_, err := ParseAllocateResponse(parcel.FromReceivedBytes(raw, nil))
	require.Error(t, err)
}

func TestParseAllocateResponseRejectsInvalidHandle(t *testing.T) {
	for _, tc := range []struct {
		name               string
		version, fds, ints uint32
	}{
		{"version", 11, 0, 0},
		{"oversized counts", 12, ^uint32(0), 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := make([]byte, 220)
			for _, off := range []int{12, 52, 100} {
				binary.LittleEndian.PutUint32(raw[off:], binderTypePTR)
			}
			binary.LittleEndian.PutUint64(raw[108:], 208)
			binary.LittleEndian.PutUint64(raw[116:], 12)
			binary.LittleEndian.PutUint32(raw[208:], tc.version)
			binary.LittleEndian.PutUint32(raw[212:], tc.fds)
			binary.LittleEndian.PutUint32(raw[216:], tc.ints)
			setTestTopology(raw, 12, uint64(tc.fds))
			_, err := ParseAllocateResponse(parcel.FromReceivedBytes(raw, []uint64{12, 52, 100, 140}))
			require.Error(t, err)
		})
	}
}

func TestParseAllocateResponseRejectsInvalidObjectTable(t *testing.T) {
	for _, objects := range [][]uint64{{12, 52, 1000, 140}, {12, 52, 100, 140}} {
		raw := make([]byte, 172)
		binary.LittleEndian.PutUint32(raw[12:], binderTypePTR)
		binary.LittleEndian.PutUint32(raw[52:], binderTypePTR)
		_, err := ParseAllocateResponse(parcel.FromReceivedBytes(raw, objects))
		require.Error(t, err)
	}
}

// This synthetic reply follows Android 10 libhwbinder's embedded native-handle
// layout, including the size word before the native buffer object. It is not a
// capture from a device.
func TestParseAllocateResponseAndroid10ObjectLayout(t *testing.T) {
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	require.NoError(t, err)
	defer unix.Close(fds[0])
	defer unix.Close(fds[1])
	raw := make([]byte, 228)
	for _, off := range []int{12, 52, 100} {
		binary.LittleEndian.PutUint32(raw[off:], binderTypePTR)
	}
	binary.LittleEndian.PutUint64(raw[20:], 172)
	binary.LittleEndian.PutUint64(raw[28:], 16)
	binary.LittleEndian.PutUint32(raw[56:], 1)
	binary.LittleEndian.PutUint64(raw[60:], 192)
	binary.LittleEndian.PutUint64(raw[68:], 16)
	binary.LittleEndian.PutUint64(raw[92:], 20)
	binary.LittleEndian.PutUint32(raw[104:], 1)
	binary.LittleEndian.PutUint64(raw[108:], 208)
	binary.LittleEndian.PutUint64(raw[116:], 20)
	binary.LittleEndian.PutUint64(raw[124:], 1)
	binary.LittleEndian.PutUint32(raw[140:], 0x66646185)
	binary.LittleEndian.PutUint64(raw[148:], 1)
	binary.LittleEndian.PutUint64(raw[156:], 2)
	binary.LittleEndian.PutUint64(raw[164:], 12)
	binary.LittleEndian.PutUint32(raw[180:], 1)
	binary.LittleEndian.PutUint32(raw[208:], 12)
	binary.LittleEndian.PutUint32(raw[212:], 1)
	binary.LittleEndian.PutUint32(raw[216:], 1)
	binary.LittleEndian.PutUint32(raw[220:], uint32(fds[0]))
	binary.LittleEndian.PutUint32(raw[224:], 123)
	p := parcel.FromReceivedBytes(raw, []uint64{12, 52, 100, 140}, 220)
	result, err := ParseAllocateResponse(p)
	require.NoError(t, err)
	require.Equal(t, []int32{int32(fds[0])}, result.Fds)
	require.Equal(t, []int32{123}, result.Ints)
	require.NoError(t, p.Recycle())
	_, err = unix.FcntlInt(uintptr(fds[0]), unix.F_GETFD, 0)
	require.NoError(t, err)
}

func TestParseAllocateResponseRejectsInvalidHandleExtent(t *testing.T) {
	for _, tc := range []struct {
		name  string
		size  int
		count uint32
	}{
		{"trailing data", 16, 0},
		{"too many ints", 12 + 1025*4, 1025},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := make([]byte, 208+tc.size)
			for _, off := range []int{12, 52, 100} {
				binary.LittleEndian.PutUint32(raw[off:], binderTypePTR)
			}
			setTestTopology(raw, uint64(tc.size), 0)
			binary.LittleEndian.PutUint64(raw[108:], 208)
			binary.LittleEndian.PutUint64(raw[116:], uint64(tc.size))
			binary.LittleEndian.PutUint32(raw[208:], 12)
			binary.LittleEndian.PutUint32(raw[216:], tc.count)
			_, err := ParseAllocateResponse(parcel.FromReceivedBytes(raw, []uint64{12, 52, 100, 140}))
			require.ErrorContains(t, err, "invalid native_handle size or counts")
		})
	}
}
