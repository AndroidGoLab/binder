package gralloc

import (
	"testing"

	"github.com/AndroidGoLab/binder/android/hardware/common"
	gfx "github.com/AndroidGoLab/binder/android/hardware/graphics/common"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// TestMaliDescriptor checks the version 30 wire descriptor and dimensions.
//
// Example: reject a descriptor with zero width.
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
//
// Example: test a two-pixel-wide buffer with padded rows.
func maliContractBuffer(t *testing.T) *Buffer {
	t.Helper()
	fd, err := unix.MemfdCreate("mali-rgba-test", 0)
	require.NoError(t, err)
	t.Cleanup(
		// Close the fd descriptor after the test.
		//
		// Example: release the test memfd.
		func() { require.NoError(t, unix.Close(fd)) })
	require.NoError(t, unix.Ftruncate(fd, 24))
	attrFD, err := unix.Dup(fd)
	require.NoError(t, err)
	t.Cleanup(
		// Close the attrFD descriptor after the test.
		//
		// Example: release the test memfd.
		func() { require.NoError(t, unix.Close(attrFD)) })

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
//
// Example: copy padded rows into a tightly packed image.
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
//
// Example: reject a handle with an incorrect byte stride.
func TestMaliRGBALayoutRejectsMalformed(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*Buffer)
	}{
		{"nil fds",
			// Change nil fds to exercise rejection.
			//
			// Example: reject a malformed nil fds.
			func(b *Buffer) { b.Handle.Fds = nil }},
		{"invalid pixel fd",
			// Change invalid pixel fd to exercise rejection.
			//
			// Example: reject a malformed invalid pixel fd.
			func(b *Buffer) { b.Handle.Fds[0] = -1 }},
		{"invalid attribute fd",
			// Change invalid attribute fd to exercise rejection.
			//
			// Example: reject a malformed invalid attribute fd.
			func(b *Buffer) { b.Handle.Fds[1] = -1 }},
		{"short handle",
			// Change short handle to exercise rejection.
			//
			// Example: reject a malformed short handle.
			func(b *Buffer) { b.Handle.Ints = b.Handle.Ints[:128] }},
		{"long handle",
			// Change long handle to exercise rejection.
			//
			// Example: reject a malformed long handle.
			func(b *Buffer) { b.Handle.Ints = append(b.Handle.Ints, 0) }},
		{"magic",
			// Change magic to exercise rejection.
			//
			// Example: reject a malformed magic.
			func(b *Buffer) { b.Handle.Ints[0] = 0 }},
		{"width",
			// Change width to exercise rejection.
			//
			// Example: reject a malformed width.
			func(b *Buffer) { b.Handle.Ints[2] = 1 }},
		{"height",
			// Change height to exercise rejection.
			//
			// Example: reject a malformed height.
			func(b *Buffer) { b.Handle.Ints[3] = 1 }},
		{"format",
			// Change format to exercise rejection.
			//
			// Example: reject a malformed format.
			func(b *Buffer) { b.Handle.Ints[4] = 2 }},
		{"producer usage low",
			// Change producer usage low to exercise rejection.
			//
			// Example: reject a malformed producer usage low.
			func(b *Buffer) { b.Handle.Ints[85] = 0 }},
		{"producer usage high",
			// Change producer usage high to exercise rejection.
			//
			// Example: reject a malformed producer usage high.
			func(b *Buffer) { b.Handle.Ints[86] = 1 }},
		{"consumer usage low",
			// Change consumer usage low to exercise rejection.
			//
			// Example: reject a malformed consumer usage low.
			func(b *Buffer) { b.Handle.Ints[87] = 0 }},
		{"consumer usage high",
			// Change consumer usage high to exercise rejection.
			//
			// Example: reject a malformed consumer usage high.
			func(b *Buffer) { b.Handle.Ints[88] = 1 }},
		{"stride low",
			// Change stride low to exercise rejection.
			//
			// Example: reject a malformed stride low.
			func(b *Buffer) { b.Handle.Ints[89] = 1 }},
		{"stride high",
			// Change stride high to exercise rejection.
			//
			// Example: reject a malformed stride high.
			func(b *Buffer) { b.Handle.Ints[90] = 1 }},
		{"internal format low",
			// Change internal format low to exercise rejection.
			//
			// Example: reject a malformed internal format low.
			func(b *Buffer) { b.Handle.Ints[91] = 0x101 }},
		{"internal format high",
			// Change internal format high to exercise rejection.
			//
			// Example: reject a malformed internal format high.
			func(b *Buffer) { b.Handle.Ints[92] = 1 }},
		{"plane offset",
			// Change plane offset to exercise rejection.
			//
			// Example: reject a malformed plane offset.
			func(b *Buffer) { b.Handle.Ints[93] = 1 }},
		{"byte stride",
			// Change byte stride to exercise rejection.
			//
			// Example: reject a malformed byte stride.
			func(b *Buffer) { b.Handle.Ints[94] = 8 }},
		{"allocation width",
			// Change allocation width to exercise rejection.
			//
			// Example: reject a malformed allocation width.
			func(b *Buffer) { b.Handle.Ints[95] = 1 }},
		{"allocation height",
			// Change allocation height to exercise rejection.
			//
			// Example: reject a malformed allocation height.
			func(b *Buffer) { b.Handle.Ints[96] = 1 }},
		{"reported stride",
			// Change reported stride to exercise rejection.
			//
			// Example: reject a malformed reported stride.
			func(b *Buffer) { b.Stride = 2 }},
		{"reported format",
			// Change reported format to exercise rejection.
			//
			// Example: reject a malformed reported format.
			func(b *Buffer) { b.Format = 2 }},
		{"reported usage",
			// Change reported usage to exercise rejection.
			//
			// Example: reject a malformed reported usage.
			func(b *Buffer) { b.Usage = 0 }},
	} {
		t.Run(tc.name,
			// Verify the selected mutation is rejected.
			//
			// Example: reject a changed handle magic.
			func(t *testing.T) {
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
