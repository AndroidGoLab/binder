package gralloc

import (
	"context"
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

// TestAllocateRGBARejectsUnknownAPI ensures unknown allocator ABIs fail closed.
//
// Example: Android versions with unverified gralloc layouts are not guessed.
func TestAllocateRGBARejectsUnknownAPI(t *testing.T) {
	_, err := AllocateRGBA(context.Background(), nil, 28, 1, 1, 1)
	require.Error(t, err)
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
