//go:build !android || !arm64

package gralloc

import "errors"

func allocateNativeRGBA(width, height int32, count int) ([]*Buffer, error) {
	return nil, errors.New("AHardwareBuffer requires Android arm64")
}
