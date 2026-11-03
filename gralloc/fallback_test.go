package gralloc

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"testing"

	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/parcel"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// allocatorScript simulates hwservicemanager and allocator transactions.
//
// Example: return a null Qualcomm service followed by a Mali allocation.
type allocatorScript struct {
	binder.Transport
	replies  []*parcel.Parcel
	failure  error
	closeErr error
	calls    int
	requests [][]byte
	handles  []uint32
	codes    []binder.TransactionCode
}

// Transact returns the next scripted response.
//
// Example: simulate an unavailable allocator and an available fallback.
func (s *allocatorScript) Transact(_ context.Context, handle uint32, code binder.TransactionCode, _ binder.TransactionFlags, data *parcel.Parcel) (*parcel.Parcel, error) {
	s.requests = append(s.requests, append([]byte(nil), data.Data()...))
	s.handles = append(s.handles, handle)
	s.codes = append(s.codes, code)
	s.calls++
	if len(s.replies) == 0 {
		return nil, s.failure
	}
	reply := s.replies[0]
	s.replies = s.replies[1:]
	return reply, nil
}

// Close simulates releasing the hwbinder connection.
//
// Example: report an error closing a connection after successful allocation.
func (s *allocatorScript) Close(context.Context) error { return s.closeErr }

// serviceReply constructs a HIDL service lookup response.
//
// Example: a zero handle indicates an unregistered service.
func serviceReply(handle uint32) *parcel.Parcel {
	raw := make([]byte, 28)
	binary.LittleEndian.PutUint32(raw[4:], 0x73682a85)
	binary.LittleEndian.PutUint32(raw[12:], handle)
	return parcel.FromReceivedBytes(raw, nil)
}

// maliReply creates a HIDL allocation with owned file descriptors.
//
// Example: corrupt the magic to test rejection and cleanup.
func maliReply(t *testing.T, valid bool) (*parcel.Parcel, []int) {
	t.Helper()
	fd, err := unix.MemfdCreate("mali-test", 0)
	require.NoError(t, err)
	require.NoError(t, unix.Ftruncate(fd, 4))
	attr, err := unix.Dup(fd)
	require.NoError(t, err)
	t.Cleanup(func() { _ = unix.Close(fd); _ = unix.Close(attr) })
	ints := make([]int32, maliHandleIntCount)
	if valid {
		ints[0] = maliHandleMagic
	}
	ints[maliWidthIndex] = 1
	ints[maliHeightIndex] = 1
	ints[maliFormatIndex] = 1
	ints[maliProducerUsageIndex] = int32(maliRGBAUsage)
	ints[maliConsumerUsageIndex] = int32(maliRGBAUsage)
	ints[maliStrideIndex] = 1
	ints[maliInternalFormatIndex] = 1
	ints[maliPlaneByteStrideIndex] = 4
	ints[maliPlaneAllocWidthIndex] = 1
	ints[maliPlaneAllocHeightIndex] = 1
	const off = 164
	data := make([]byte, off+12+8+4*maliHandleIntCount)
	binary.LittleEndian.PutUint32(data[8:], 1)
	for _, pos := range []int{12, 52, 92} {
		binary.LittleEndian.PutUint32(data[pos:], 0x70742a85)
	}
	binary.LittleEndian.PutUint64(data[100:], off)
	binary.LittleEndian.PutUint64(data[108:], uint64(len(data)-off))
	binary.LittleEndian.PutUint32(data[off:], 12)
	binary.LittleEndian.PutUint32(data[off+4:], 2)
	binary.LittleEndian.PutUint32(data[off+8:], maliHandleIntCount)
	binary.LittleEndian.PutUint32(data[off+12:], uint32(fd))
	binary.LittleEndian.PutUint32(data[off+16:], uint32(attr))
	for i, v := range ints {
		binary.LittleEndian.PutUint32(data[off+20+i*4:], uint32(v))
	}
	return parcel.FromReceivedBytes(data, nil, off+12, off+16), []int{fd, attr}
}

// TestAndroid10Fallback exercises service discovery and real Mali reply handling.
//
// Example: an absent Qualcomm allocator falls back to a registered Mali service.
func TestAndroid10Fallback(t *testing.T) {
	reply, fds := maliReply(t, true)
	s := &allocatorScript{replies: []*parcel.Parcel{serviceReply(0), serviceReply(7), reply}}
	buffers, err := allocateAndroid10RGBA(context.Background(), s, 1, 1, 1)
	require.NoError(t, err)
	require.Len(t, buffers, 1)
	require.Equal(t, 3, s.calls)
	require.Equal(t, []uint32{0, 0, 7}, s.handles)
	require.Equal(t, []binder.TransactionCode{1, 1, 2}, s.codes)
	require.True(t, bytes.Contains(s.requests[2], []byte("android.hardware.graphics.allocator@3.0::IAllocator")))
	for _, fd := range fds {
		_, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
		require.NoError(t, err)
	}
	require.NoError(t, CloseBuffers(buffers))
}

// TestAndroid10FallbackFailureCleanup checks that errors do not leak allocations.
//
// Example: an invalid second allocation closes both its descriptors and the first buffer.
func TestAndroid10FallbackFailureCleanup(t *testing.T) {
	first, fds1 := maliReply(t, true)
	second, fds2 := maliReply(t, false)
	s := &allocatorScript{replies: []*parcel.Parcel{serviceReply(0), serviceReply(7), first, second}}
	buffers, err := allocateAndroid10RGBA(context.Background(), s, 1, 1, 2)
	require.ErrorContains(t, err, "unsupported Mali allocation")
	require.Nil(t, buffers)
	require.Equal(t, 4, s.calls)
	for _, fd := range append(fds1, fds2...) {
		_, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
		require.ErrorIs(t, err, unix.EBADF)
	}
}

// TestAndroid10DoesNotFallbackOnDiscoveryError preserves genuine failures.
//
// Example: a failed Qualcomm service transaction must not query Mali.
func TestAndroid10DoesNotFallbackOnDiscoveryError(t *testing.T) {
	failure := errors.New("lookup failure")
	s := &allocatorScript{failure: failure}
	buffers, err := allocateAndroid10RGBA(context.Background(), s, 1, 1, 1)
	require.ErrorIs(t, err, failure)
	require.Nil(t, buffers)
	require.Equal(t, 1, s.calls)
}

// TestAndroid10CloseFailureClosesBuffers protects descriptors on failed return.
//
// Example: a connection close failure after allocation invalidates its buffers.
func TestAndroid10CloseFailureClosesBuffers(t *testing.T) {
	reply, fds := maliReply(t, true)
	failure := errors.New("close failure")
	s := &allocatorScript{replies: []*parcel.Parcel{serviceReply(0), serviceReply(7), reply}, closeErr: failure}
	buffers, err := allocateAndroid10RGBA(context.Background(), s, 1, 1, 1)
	require.ErrorIs(t, err, failure)
	require.Nil(t, buffers)
	for _, fd := range fds {
		_, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
		require.ErrorIs(t, err, unix.EBADF)
	}
}

// TestAndroid10QualcommSuccessStopsDiscovery verifies the preferred allocator.
//
// Example: a valid Qualcomm buffer prevents a Mali lookup.
func TestAndroid10QualcommSuccessStopsDiscovery(t *testing.T) {
	fd, err := unix.MemfdCreate("qcom-test", 0)
	require.NoError(t, err)
	attr, err := unix.Dup(fd)
	require.NoError(t, err)
	t.Cleanup(func() { _ = unix.Close(fd); _ = unix.Close(attr) })
	raw := make([]byte, 308)
	binary.LittleEndian.PutUint32(raw[8:], 1)
	for _, off := range []int{12, 52, 92} {
		binary.LittleEndian.PutUint32(raw[off:], 0x70742a85)
	}
	binary.LittleEndian.PutUint64(raw[20:], 164)
	binary.LittleEndian.PutUint64(raw[28:], 16)
	binary.LittleEndian.PutUint32(raw[172:], 1)
	binary.LittleEndian.PutUint64(raw[100:], 200)
	binary.LittleEndian.PutUint64(raw[108:], 108)
	binary.LittleEndian.PutUint32(raw[132:], 0x66646185)
	binary.LittleEndian.PutUint64(raw[140:], 2)
	binary.LittleEndian.PutUint64(raw[148:], 2)
	binary.LittleEndian.PutUint64(raw[156:], 12)
	binary.LittleEndian.PutUint32(raw[200:], 12)
	binary.LittleEndian.PutUint32(raw[204:], 2)
	binary.LittleEndian.PutUint32(raw[208:], 22)
	binary.LittleEndian.PutUint32(raw[212:], uint32(fd))
	binary.LittleEndian.PutUint32(raw[216:], uint32(attr))
	ints := []int32{qcomHandleMagic, qcomIonFlag, 1, 1, 1, 1, 1, 0, 1, 0, 0, int32(qcomRGBAUsage), 0, 4}
	for i, v := range ints {
		binary.LittleEndian.PutUint32(raw[220+i*4:], uint32(v))
	}
	reply := parcel.FromReceivedBytes(raw, []uint64{12, 52, 92, 132}, 212, 216)
	s := &allocatorScript{replies: []*parcel.Parcel{serviceReply(9), reply}}
	buffers, err := allocateAndroid10RGBA(context.Background(), s, 1, 1, 1)
	require.NoError(t, err)
	require.Len(t, buffers, 1)
	require.Equal(t, []uint32{0, 9}, s.handles)
	require.Equal(t, []binder.TransactionCode{1, 2}, s.codes)
	require.True(t, bytes.Contains(s.requests[1], []byte("android.hardware.graphics.allocator@2.0::IAllocator")))
	require.NoError(t, CloseBuffers(buffers))
}

// TestAndroid10MissingServicesAndCloseFailure reports both failures.
//
// Example: a failed connection close remains visible after both probes fail.
func TestAndroid10MissingServicesAndCloseFailure(t *testing.T) {
	failure := errors.New("close failure")
	s := &allocatorScript{replies: []*parcel.Parcel{serviceReply(0), serviceReply(0)}, closeErr: failure}
	buffers, err := allocateAndroid10RGBA(context.Background(), s, 1, 1, 1)
	require.ErrorContains(t, err, "Mali allocator@3.0 is not registered")
	require.ErrorIs(t, err, failure)
	require.Nil(t, buffers)
	require.Equal(t, 2, s.calls)
}
