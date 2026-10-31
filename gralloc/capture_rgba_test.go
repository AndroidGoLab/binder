package gralloc

import (
	"os"
	"strings"
	"testing"

	common "github.com/AndroidGoLab/binder/android/hardware/common"
)

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

func TestCopyRGBARejectsTruncatedFD(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "buffer")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	b := pixelHandle()
	b.Handle.Fds[0] = int32(f.Fd())
	_, err = CopyRGBA(b)
	if err == nil || !strings.Contains(err.Error(), "pixel fd too short") {
		t.Fatalf("got %v", err)
	}
}
func TestPixelRGBALayout(t *testing.T) {
	b := pixelHandle()
	l, err := pixelRGBALayout(b)
	if err != nil {
		t.Fatal(err)
	}
	if l.fd != 7 || l.stride != 16 || l.offset != 0 || l.length != 32 {
		t.Fatalf("wrong layout: %+v", l)
	}
}
func TestPixelRGBALayoutRejectsUnsupported(t *testing.T) {
	cases := map[string]func(*Buffer){
		"magic": func(b *Buffer) { b.Handle.Ints[2] = 0 }, "compressed": func(b *Buffer) { b.Handle.Ints[16] = 1 },
		"bad fd": func(b *Buffer) { b.Handle.Ints[19] = 1 }, "short allocation": func(b *Buffer) { b.Handle.Ints[63] = 8 },
		"short stride": func(b *Buffer) { b.Handle.Ints[23] = 4 }, "dimensions": func(b *Buffer) { b.Handle.Ints[5] = 3 },
		"layers": func(b *Buffer) { b.Handle.Ints[53] = 2 }, "wrong handle length": func(b *Buffer) { b.Handle.Ints = b.Handle.Ints[:60] },
		"mapper version": func(b *Buffer) { b.Handle.Ints[75] = 500 },
		"short plane":    func(b *Buffer) { b.Handle.Ints[21] = 8 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			b := pixelHandle()
			mutate(b)
			if _, err := pixelRGBALayout(b); err == nil {
				t.Fatal("accepted unsupported layout")
			}
		})
	}
}
