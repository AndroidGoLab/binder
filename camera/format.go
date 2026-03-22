package camera

import gfxCommon "github.com/AndroidGoLab/binder/android/hardware/graphics/common"

// Format identifies a pixel format for camera capture.
type Format = gfxCommon.PixelFormat

// Supported pixel formats for camera capture.
const (
	FormatYCbCr420 Format = gfxCommon.PixelFormatYcbcr420888
)
