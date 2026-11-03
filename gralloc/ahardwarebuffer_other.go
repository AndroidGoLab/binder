//go:build !android || !arm64

package gralloc

import "errors"

// allocateNativeRGBA reports that this target has no native Android backend.
//
// Example: retain the platform-specific allocator on a Linux build.
func allocateNativeRGBA(width, height int32, count int) ([]*Buffer, error) {
	return nil, errors.New("AHardwareBuffer requires Android arm64")
}
