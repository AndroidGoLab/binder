package gralloc

import (
	"testing"

	"github.com/AndroidGoLab/binder/android/hardware/common"
	gfx "github.com/AndroidGoLab/binder/android/hardware/graphics/common"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestMaliDescriptor(t *testing.T) {
	descriptor, err := maliDescriptor(720, 1280)
	require.NoError(t, err)
	require.Equal(t, []uint32{30, 720, 1280, 1, 1, 0xb03, 0, 0, 0}, descriptor)

	_, err = maliDescriptor(0, 1280)
	require.Error(t, err)
}

func TestMaliRGBALayout(t *testing.T) {
	fd, err := unix.MemfdCreate("mali-rgba-test", 0)
	require.NoError(t, err)
	defer unix.Close(fd)
	require.NoError(t, unix.Ftruncate(fd, 24))
	attrFD, err := unix.Dup(fd)
	require.NoError(t, err)
	defer unix.Close(attrFD)

	ints := make([]int32, maliHandleIntCount)
	ints[0] = maliHandleMagic
	ints[maliWidthIndex] = 2
	ints[maliHeightIndex] = 2
	ints[maliFormatIndex] = int32(gfx.PixelFormatRgba8888)
	ints[maliProducerUsageIndex] = int32(maliRGBAUsage)
	ints[maliConsumerUsageIndex] = int32(maliRGBAUsage)
	ints[maliStrideIndex] = 3
	ints[maliInternalFormatIndex] = int32(gfx.PixelFormatRgba8888)
	ints[maliPlaneByteStrideIndex] = 12
	ints[maliPlaneAllocWidthIndex] = 3
	ints[maliPlaneAllocHeightIndex] = 2

	buffer := &Buffer{
		Handle: common.NativeHandle{Fds: []int32{int32(fd), int32(attrFD)}, Ints: ints},
		Stride: 3,
		Width:  2,
		Height: 2,
		Format: int32(gfx.PixelFormatRgba8888),
		Usage:  maliRGBAUsage,
	}
	layout, err := maliRGBALayout(buffer)
	require.NoError(t, err)
	require.Equal(t, rgbaLayout{fd: fd, stride: 12, length: 24}, layout)
	captureLayout, err := captureRGBALayout(buffer)
	require.NoError(t, err)
	require.Equal(t, layout, captureLayout)

	source := []byte{1, 2, 3, 4, 5, 6, 7, 8, 99, 99, 99, 99, 9, 10, 11, 12, 13, 14, 15, 16, 99, 99, 99, 99}
	destination := make([]byte, 16)
	copyRGBARows(destination, source, layout, buffer.Width, buffer.Height)
	require.Equal(t, []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}, destination)

	buffer.Handle.Ints[maliInternalFormatIndex] = 0x101
	_, err = maliRGBALayout(buffer)
	require.Error(t, err)

	buffer.Handle.Ints[maliInternalFormatIndex] = int32(gfx.PixelFormatRgba8888)
	buffer.Handle.Ints[maliPlaneOffsetIndex] = 1
	_, err = maliRGBALayout(buffer)
	require.Error(t, err)

	buffer.Handle.Ints[maliPlaneOffsetIndex] = 0
	buffer.Handle.Fds[1] = -1
	_, err = maliRGBALayout(buffer)
	require.Error(t, err)
}
