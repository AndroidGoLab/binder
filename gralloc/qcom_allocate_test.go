package gralloc

import (
	"context"
	"encoding/binary"
	"github.com/AndroidGoLab/binder/parcel"
	"golang.org/x/sys/unix"
	"testing"

	"github.com/AndroidGoLab/binder/android/hardware/common"
	"github.com/stretchr/testify/require"
)

// TestQcomDescriptor checks the Android 10 SDM845 allocator descriptor.
//
// Example: CPU-read-rarely requests uncached linear RGBA output.
func TestQcomDescriptor(t *testing.T) {
	d, err := qcomDescriptor(360, 800)
	require.NoError(t, err)
	require.Equal(t, []uint32{0x76312e30, 360, 800, 1, 1, 0xb02, 0}, d)
	_, err = qcomDescriptor(0, 800)
	require.Error(t, err)
}

// TestQcomLayoutRejectsUnsafeBuffers checks source-specific handle validation.
//
// Example: cached or compressed handles cannot use the uncached reader.
func TestQcomLayoutRejectsUnsafeBuffers(t *testing.T) {
	ints := make([]int32, 22)
	ints[0] = 0x676d736d
	ints[1] = 8
	ints[2] = 16
	ints[3] = 16
	ints[4] = 10
	ints[5] = 10
	ints[6] = 1
	ints[8] = 1
	ints[11] = 0xb02
	ints[13] = 1024
	b := &Buffer{Handle: common.NativeHandle{Fds: []int32{3, 4}, Ints: ints}, Stride: 16, Width: 10, Height: 10, Format: 1, Usage: 0xb02}
	layout, err := qcomRGBALayout(b)
	require.NoError(t, err)
	require.Equal(t, 64, layout.stride)
	require.Equal(t, 0, layout.offset)
	for _, flag := range []int32{1, 0x200, 0x400, 0x4000, 0x1000000, 0x2000000, 0x8000000} {
		b.Handle.Ints[1] = 8 | flag
		_, err = qcomRGBALayout(b)
		require.Error(t, err)
	}
	b.Handle.Ints[1] = 8
	b.Handle.Ints[11] = 0xb03
	_, err = qcomRGBALayout(b)
	require.Error(t, err)
	b.Handle.Ints[11] = 0xb02
	b.Width = ^uint32(0)
	b.Handle.Ints[4] = -1
	_, err = qcomRGBALayout(b)
	require.Error(t, err)
	b.Width = 10
	b.Handle.Ints[4] = 10
	b.Handle.Ints[14] = 1
	_, err = qcomRGBALayout(b)
	require.Error(t, err)
}

// TestQcomAllocationReplyOwnsDescriptors validates HIDL status and FD transfer.
//
// Example: allocated descriptors outlive recycling the HIDL reply.
func TestQcomAllocationReplyOwnsDescriptors(t *testing.T) {
	var fds [2]int
	require.NoError(t, unix.Pipe(fds[:]))
	defer unix.Close(fds[0])
	defer unix.Close(fds[1])
	fd0, err := unix.Dup(fds[0])
	require.NoError(t, err)
	fd1, err := unix.Dup(fds[0])
	require.NoError(t, err)
	raw := make([]byte, 316)
	binary.LittleEndian.PutUint32(raw[8:], 16)
	for _, off := range []int{12, 52, 100} {
		binary.LittleEndian.PutUint32(raw[off:], 0x70742a85)
	}
	binary.LittleEndian.PutUint32(raw[56:], 1)
	binary.LittleEndian.PutUint32(raw[104:], 1)
	binary.LittleEndian.PutUint64(raw[124:], 1)
	binary.LittleEndian.PutUint64(raw[20:], 172)
	binary.LittleEndian.PutUint64(raw[28:], 16)
	binary.LittleEndian.PutUint32(raw[180:], 1)
	binary.LittleEndian.PutUint64(raw[60:], 192)
	binary.LittleEndian.PutUint64(raw[68:], 16)
	binary.LittleEndian.PutUint64(raw[92:], 108)
	binary.LittleEndian.PutUint64(raw[108:], 208)
	binary.LittleEndian.PutUint64(raw[116:], 108)
	binary.LittleEndian.PutUint32(raw[140:], 0x66646185)
	binary.LittleEndian.PutUint64(raw[148:], 2)
	binary.LittleEndian.PutUint64(raw[156:], 2)
	binary.LittleEndian.PutUint64(raw[164:], 12)
	binary.LittleEndian.PutUint32(raw[208:], 12)
	binary.LittleEndian.PutUint32(raw[212:], 2)
	binary.LittleEndian.PutUint32(raw[216:], 22)
	binary.LittleEndian.PutUint32(raw[220:], uint32(fd0))
	binary.LittleEndian.PutUint32(raw[224:], uint32(fd1))
	p := parcel.FromReceivedBytes(raw, []uint64{12, 52, 100, 140}, 220, 224)
	stride, h, err := readQcomAllocation(p)
	require.NoError(t, err)
	require.Equal(t, int32(16), stride)
	require.Len(t, h.Fds, 2)
	require.NoError(t, p.Recycle())
	require.NoError(t, unix.Close(int(h.Fds[0])))
	require.NoError(t, unix.Close(int(h.Fds[1])))
}

// TestQcomAllocationReplyRejectsHALFailure prevents interpreting errors as buffers.
//
// Example: a successful HIDL transaction may still return an allocator error.
func TestQcomAllocationReplyRejectsHALFailure(t *testing.T) {
	for _, offset := range []int{0, 4} {
		raw := make([]byte, 12)
		binary.LittleEndian.PutUint32(raw[offset:], 1)
		p := parcel.FromReceivedBytes(raw, nil)
		_, _, err := readQcomAllocation(p)
		require.Error(t, err)
	}
}

// TestQcomAllocationReplyRejectsMissingHandle checks a successful empty reply.
func TestQcomAllocationReplyRejectsMissingHandle(t *testing.T) {
	p := parcel.FromReceivedBytes(make([]byte, 12), nil)
	_, _, err := readQcomAllocation(p)
	require.Error(t, err)
	require.NoError(t, p.Recycle())
}

// TestQcomAllocationReplyRejectsWrongShapeClosesDescriptors checks vendor validation.
func TestQcomAllocationReplyRejectsWrongShapeClosesDescriptors(t *testing.T) {
	var pipe [2]int
	require.NoError(t, unix.Pipe(pipe[:]))
	defer unix.Close(pipe[0])
	defer unix.Close(pipe[1])
	fd0, err := unix.Dup(pipe[0])
	require.NoError(t, err)
	fd1, err := unix.Dup(pipe[0])
	require.NoError(t, err)
	raw := make([]byte, 312)
	for _, off := range []int{12, 52, 100} {
		binary.LittleEndian.PutUint32(raw[off:], 0x70742a85)
	}
	binary.LittleEndian.PutUint32(raw[56:], 1)
	binary.LittleEndian.PutUint32(raw[104:], 1)
	binary.LittleEndian.PutUint64(raw[124:], 1)
	binary.LittleEndian.PutUint64(raw[20:], 172)
	binary.LittleEndian.PutUint64(raw[28:], 16)
	binary.LittleEndian.PutUint32(raw[180:], 1)
	binary.LittleEndian.PutUint64(raw[60:], 192)
	binary.LittleEndian.PutUint64(raw[68:], 16)
	binary.LittleEndian.PutUint64(raw[92:], 104)
	binary.LittleEndian.PutUint64(raw[108:], 208)
	binary.LittleEndian.PutUint64(raw[116:], 104)
	binary.LittleEndian.PutUint32(raw[140:], 0x66646185)
	binary.LittleEndian.PutUint64(raw[148:], 2)
	binary.LittleEndian.PutUint64(raw[156:], 2)
	binary.LittleEndian.PutUint64(raw[164:], 12)
	binary.LittleEndian.PutUint32(raw[208:], 12)
	binary.LittleEndian.PutUint32(raw[212:], 2)
	binary.LittleEndian.PutUint32(raw[216:], 21)
	binary.LittleEndian.PutUint32(raw[220:], uint32(fd0))
	binary.LittleEndian.PutUint32(raw[224:], uint32(fd1))
	p := parcel.FromReceivedBytes(raw, []uint64{12, 52, 100, 140}, 220, 224)
	_, _, err = readQcomAllocation(p)
	require.ErrorContains(t, err, "unsupported Qualcomm handle shape")
	require.NoError(t, p.Recycle())
	for _, fd := range []int{fd0, fd1} {
		_, err = unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
		require.ErrorIs(t, err, unix.EBADF)
	}
}

// TestAllocateRGBARejectsUnknownAPI ensures unknown allocator ABIs fail closed.
//
// Example: Android versions with unverified gralloc layouts are not guessed.
func TestAllocateRGBARejectsUnknownAPI(t *testing.T) {
	_, err := AllocateRGBA(context.Background(), nil, 28, 1, 1, 1)
	require.Error(t, err)
}

// TestQcomAllocationReplyClosesPartialTransfer checks cleanup on an FD ownership failure.
//
// Example: failure to transfer the second descriptor closes the first exactly once.
func TestQcomAllocationReplyClosesPartialTransfer(t *testing.T) {
	var fds [2]int
	require.NoError(t, unix.Pipe(fds[:]))
	defer unix.Close(fds[0])
	defer unix.Close(fds[1])
	first, err := unix.Dup(fds[0])
	require.NoError(t, err)
	raw := make([]byte, 316)
	binary.LittleEndian.PutUint32(raw[8:], 16)
	for _, off := range []int{12, 52, 100} {
		binary.LittleEndian.PutUint32(raw[off:], 0x70742a85)
	}
	binary.LittleEndian.PutUint32(raw[56:], 1)
	binary.LittleEndian.PutUint32(raw[104:], 1)
	binary.LittleEndian.PutUint64(raw[124:], 1)
	binary.LittleEndian.PutUint64(raw[20:], 172)
	binary.LittleEndian.PutUint64(raw[28:], 16)
	binary.LittleEndian.PutUint32(raw[180:], 1)
	binary.LittleEndian.PutUint64(raw[60:], 192)
	binary.LittleEndian.PutUint64(raw[68:], 16)
	binary.LittleEndian.PutUint64(raw[92:], 108)
	binary.LittleEndian.PutUint64(raw[108:], 208)
	binary.LittleEndian.PutUint64(raw[116:], 108)
	binary.LittleEndian.PutUint32(raw[140:], 0x66646185)
	binary.LittleEndian.PutUint64(raw[148:], 2)
	binary.LittleEndian.PutUint64(raw[156:], 2)
	binary.LittleEndian.PutUint64(raw[164:], 12)
	binary.LittleEndian.PutUint32(raw[208:], 12)
	binary.LittleEndian.PutUint32(raw[212:], 2)
	binary.LittleEndian.PutUint32(raw[216:], 22)
	binary.LittleEndian.PutUint32(raw[220:], uint32(first))
	binary.LittleEndian.PutUint32(raw[224:], uint32(fds[0]))
	p := parcel.FromReceivedBytes(raw, []uint64{12, 52, 100, 140}, 220)
	_, _, err = readQcomAllocation(p)
	require.Error(t, err)
	require.NoError(t, p.Recycle())
	_, err = unix.FcntlInt(uintptr(first), unix.F_GETFD, 0)
	require.ErrorIs(t, err, unix.EBADF)
	_, err = unix.FcntlInt(uintptr(fds[0]), unix.F_GETFD, 0)
	require.NoError(t, err)
}

// TestQcomMappedRGBAIntoReusesDestination checks padded rows and length validation.
//
// Example: a stream reuses one caller-owned pixel slice across frames.
func TestQcomMappedRGBAIntoReusesDestination(t *testing.T) {
	fd, err := unix.MemfdCreate("qcom-row-test", 0)
	require.NoError(t, err)
	defer unix.Close(fd)
	raw := []byte{1, 2, 3, 4, 5, 6, 7, 8, 90, 90, 90, 90, 9, 10, 11, 12, 13, 14, 15, 16, 90, 90, 90, 90}
	require.NoError(t, unix.Ftruncate(fd, int64(len(raw))))
	_, err = unix.Pwrite(fd, raw, 0)
	require.NoError(t, err)
	b := &Buffer{Width: 2, Height: 2}
	layout := rgbaLayout{fd: fd, stride: 12, length: 24}
	dst := make([]byte, 16)
	require.NoError(t, copyQcomMappedRGBAInto(b, layout, dst))
	require.Equal(t, []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}, dst)
	require.Error(t, copyQcomMappedRGBAInto(b, layout, dst[:15]))
	require.Error(t, copyQcomMappedRGBAInto(b, layout, make([]byte, 17)))
	_, err = unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
	require.NoError(t, err)
}
