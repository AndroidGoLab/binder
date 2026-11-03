package gralloc

import (
	"testing"

	"github.com/AndroidGoLab/binder/android/hardware/common"
	gfx "github.com/AndroidGoLab/binder/android/hardware/graphics/common"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// TestMaliDescriptor checks the version 30 wire descriptor and dimensions.
func TestMaliDescriptor(t *testing.T) {
	descriptor, err := maliDescriptor(720, 1280)
	require.NoError(t, err)
	require.Equal(t, []uint32{30, 720, 1280, 1, 1, 0xb03, 0, 0, 0}, descriptor)

	for _, dimensions := range [][2]int32{{0, 1280}, {720, 0}, {-1, 1}, {1, -1}} {
		_, err = maliDescriptor(dimensions[0], dimensions[1])
		require.Error(t, err)
	}
}

// maliContractBuffer constructs a synthetic handle, not a device capture.
// The explicit indices and values describe the accepted PR3 handle contract;
// they deliberately do not use the implementation's index constants.
func maliContractBuffer(t *testing.T) *Buffer {
	t.Helper()
	fd, err := unix.MemfdCreate("mali-rgba-test", 0)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, unix.Close(fd)) })
	require.NoError(t, unix.Ftruncate(fd, 24))
	attrFD, err := unix.Dup(fd)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, unix.Close(attrFD)) })

	return &Buffer{
		Handle: common.NativeHandle{
			Fds: []int32{int32(fd), int32(attrFD)},
			Ints: []int32{
				0: 0x03141592, 2: 2, 3: 2, 4: 1,
				85: 0xb03, 87: 0xb03, 89: 3, 91: 1,
				93: 0, 94: 12, 95: 3, 96: 2, 128: 0,
			},
		},
		Stride: 3, Width: 2, Height: 2,
		Format: int32(gfx.PixelFormatRgba8888), Usage: 0xb03,
	}
}

// TestMaliRGBALayout checks the supported linear RGBA handle contract.
func TestMaliRGBALayout(t *testing.T) {
	buffer := maliContractBuffer(t)
	layout, err := maliRGBALayout(buffer)
	require.NoError(t, err)
	require.Equal(t, rgbaLayout{fd: int(buffer.Handle.Fds[0]), stride: 12, length: 24}, layout)
	captureLayout, err := captureRGBALayout(buffer)
	require.NoError(t, err)
	require.Equal(t, layout, captureLayout)

	source := []byte{1, 2, 3, 4, 5, 6, 7, 8, 99, 99, 99, 99, 9, 10, 11, 12, 13, 14, 15, 16, 99, 99, 99, 99}
	destination := make([]byte, 16)
	copyRGBARows(destination, source, layout, buffer.Width, buffer.Height)
	require.Equal(t, []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}, destination)
}

// TestMaliRGBALayoutRejectsMalformed checks each consumed handle field.
func TestMaliRGBALayoutRejectsMalformed(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*Buffer)
	}{
		{"nil fds", func(b *Buffer) { b.Handle.Fds = nil }},
		{"invalid pixel fd", func(b *Buffer) { b.Handle.Fds[0] = -1 }},
		{"invalid attribute fd", func(b *Buffer) { b.Handle.Fds[1] = -1 }},
		{"short handle", func(b *Buffer) { b.Handle.Ints = b.Handle.Ints[:128] }},
		{"long handle", func(b *Buffer) { b.Handle.Ints = append(b.Handle.Ints, 0) }},
		{"magic", func(b *Buffer) { b.Handle.Ints[0] = 0 }},
		{"width", func(b *Buffer) { b.Handle.Ints[2] = 1 }},
		{"height", func(b *Buffer) { b.Handle.Ints[3] = 1 }},
		{"format", func(b *Buffer) { b.Handle.Ints[4] = 2 }},
		{"producer usage low", func(b *Buffer) { b.Handle.Ints[85] = 0 }},
		{"producer usage high", func(b *Buffer) { b.Handle.Ints[86] = 1 }},
		{"consumer usage low", func(b *Buffer) { b.Handle.Ints[87] = 0 }},
		{"consumer usage high", func(b *Buffer) { b.Handle.Ints[88] = 1 }},
		{"stride low", func(b *Buffer) { b.Handle.Ints[89] = 1 }},
		{"stride high", func(b *Buffer) { b.Handle.Ints[90] = 1 }},
		{"internal format low", func(b *Buffer) { b.Handle.Ints[91] = 0x101 }},
		{"internal format high", func(b *Buffer) { b.Handle.Ints[92] = 1 }},
		{"plane offset", func(b *Buffer) { b.Handle.Ints[93] = 1 }},
		{"byte stride", func(b *Buffer) { b.Handle.Ints[94] = 8 }},
		{"allocation width", func(b *Buffer) { b.Handle.Ints[95] = 1 }},
		{"allocation height", func(b *Buffer) { b.Handle.Ints[96] = 1 }},
		{"reported stride", func(b *Buffer) { b.Stride = 2 }},
		{"reported format", func(b *Buffer) { b.Format = 2 }},
		{"reported usage", func(b *Buffer) { b.Usage = 0 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			buffer := maliContractBuffer(t)
			tc.change(buffer)
			_, err := maliRGBALayout(buffer)
			require.Error(t, err)
		})
	}
	_, err := maliRGBALayout(nil)
	require.Error(t, err)

	buffer := maliContractBuffer(t)
	require.NoError(t, unix.Ftruncate(int(buffer.Handle.Fds[0]), 23))
	_, err = maliRGBALayout(buffer)
	require.ErrorContains(t, err, "exceeds dma-buf size")
}
