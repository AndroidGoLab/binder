package gralloc

import (
	"os"
	"testing"

	common "github.com/AndroidGoLab/binder/android/hardware/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pixelHandle constructs a small valid gchips handle for layout tests.
//
// Example: change one field to exercise rejection of a malformed layout.
func pixelHandle() *Buffer {
	ints := make([]int32, 77)
	ints[0] = -1
	ints[1] = -1
	ints[2] = 0x3141592
	ints[4] = 1
	ints[5] = 2
	ints[6] = 2
	ints[7] = 1
	ints[13] = 4
	ints[15] = 1
	ints[23] = 16
	ints[25] = 4
	ints[27] = 2
	ints[53] = 1
	ints[63] = 32
	ints[75] = 400
	return &Buffer{Handle: common.NativeHandle{Fds: []int32{7, 8}, Ints: ints}, Width: 2, Height: 2, Stride: 4, Format: 1}
}

// TestCopyRGBARejectsTruncatedFD verifies that a short allocation is rejected.
//
// Example: a zero-length FD cannot contain a two-row image.
func TestCopyRGBARejectsTruncatedFD(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "buffer")
	require.NoError(t, err)
	defer // Close the temporary allocation after checking its length.
	//
	// Example: an unexpected close error fails the test.
	func() { assert.NoError(t, f.Close()) }()
	b := pixelHandle()
	b.Handle.Fds[0] = int32(f.Fd())
	_, err = CopyRGBA(b)
	require.ErrorContains(t, err, "pixel fd too short")
}

// TestPixelRGBALayout verifies the supported plane geometry.
//
// Example: a two-pixel image can have a four-pixel stride.
func TestPixelRGBALayout(t *testing.T) {
	b := pixelHandle()
	l, err := pixelRGBALayout(b)
	require.NoError(t, err)
	require.Equal(t, rgbaLayout{fd: 7, stride: 16, offset: 0, length: 32}, l)
}

// TestPixelRGBALayoutOffset includes the plane offset in the mapped extent.
//
// Example: a 32-byte plane beginning at byte eight needs a 40-byte mapping.
func TestPixelRGBALayoutOffset(t *testing.T) {
	b := pixelHandle()
	b.Handle.Ints[gchipsPlaneOffsetIndex] = 8
	b.Handle.Ints[gchipsAllocSizeIndex] = 40
	l, err := pixelRGBALayout(b)
	require.NoError(t, err)
	require.Equal(t, 8, l.offset)
	require.Equal(t, 40, l.length)
}

// TestPixelRGBALayoutRejectsUnsupported checks the ABI validation boundaries.
//
// Example: compressed or undersized planes must not be read as RGBA.
func TestPixelRGBALayoutRejectsUnsupported(t *testing.T) {
	cases := map[string]struct {
		index    int
		value    int32
		truncate bool
	}{
		"magic":               {index: gchipsMagicIndex, value: 0},
		"compressed":          {index: gchipsAllocFormatIndex + 1, value: 1},
		"bad fd":              {index: gchipsPlaneFDIndex, value: 1},
		"short allocation":    {index: gchipsAllocSizeIndex, value: 8},
		"short stride":        {index: gchipsPlaneByteStride, value: 4},
		"dimensions":          {index: gchipsWidthIndex, value: 3},
		"layers":              {index: gchipsLayerCountIndex, value: 2},
		"wrong handle length": {truncate: true},
		"mapper version":      {index: gchipsMapperVersionIndex, value: 500},
		"short plane":         {index: gchipsPlaneSizeIndex, value: 8},
	}
	for name, mutation := range cases {
		t.Run(name,
			// Reject this single malformed ABI field.
			//
			// Example: a short plane must not be read as a full image.
			func(t *testing.T) {
				b := pixelHandle()
				if mutation.truncate {
					b.Handle.Ints = b.Handle.Ints[:60]
				} else {
					b.Handle.Ints[mutation.index] = mutation.value
				}
				_, err := pixelRGBALayout(b)
				require.Error(t, err)
			},
		)
	}
}
