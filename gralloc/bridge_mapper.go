package gralloc

import (
	"fmt"

	gfxCommon "github.com/AndroidGoLab/binder/android/hardware/graphics/common"
	"github.com/AndroidGoLab/binder/gralloc/bridge"
)

// bridgeMapper implements Mapper via the HIDL IMapper bridge .so.
type bridgeMapper struct{}

var _ Mapper = (*bridgeMapper)(nil)

func newBridgeMapper() (*bridgeMapper, error) {
	if err := bridge.Open(); err != nil {
		return nil, err
	}
	return &bridgeMapper{}, nil
}

func (bridgeMapper) LockBuffer(b *Buffer) ([]byte, error) {
	if len(b.Handle.Fds) == 0 || len(b.Handle.Ints) == 0 {
		return nil, fmt.Errorf("empty handle")
	}

	h, err := bridge.ImportBuffer(b.Handle.Fds, b.Handle.Ints)
	if err != nil {
		return nil, err
	}
	defer bridge.Free(h)

	switch gfxCommon.PixelFormat(b.Format) {
	case gfxCommon.PixelFormatYcbcr420888:
		return bridge.LockYCbCr(h, int32(b.Width), int32(b.Height))
	default:
		return bridge.LockGeneric(h, int32(b.Width), int32(b.Height), b.BufferSize())
	}
}
