package gralloc

import (
	"errors"
	"fmt"
	"math"

	gfx "github.com/AndroidGoLab/binder/android/hardware/graphics/common"
	"golang.org/x/sys/unix"
)

// The version 30 descriptor encoding is defined by TinkerBoard2's Bifrost
// BufferDescriptor.h at b7a3ba9a7d860ce3301c22c6cb71177a9a6c84c5:
// https://github.com/TinkerBoard2-Android/hardware-rockchip-libgralloc-bifrost/blob/b7a3ba9a7d860ce3301c22c6cb71177a9a6c84c5/src/hidl_common/BufferDescriptor.h#L139-L167
// This source documents the descriptor, not the native handle below.
const (
	maliDescriptorVersion = 30
	maliHandleMagic       = 0x03141592
	maliHandleIntCount    = 129
	maliRGBAUsage         = uint64(gfx.BufferUsageCpuReadOften | gfx.BufferUsageGpuTexture | gfx.BufferUsageGpuRenderTarget | gfx.BufferUsageComposerOverlay)

	// These word offsets are limited to the 129-word profile described in
	// https://github.com/AndroidGoLab/binder/pull/3. That PR reports device
	// testing but does not include a raw handle capture or matching header.
	maliWidthIndex            = 2
	maliHeightIndex           = 3
	maliFormatIndex           = 4
	maliProducerUsageIndex    = 85
	maliConsumerUsageIndex    = 87
	maliStrideIndex           = 89
	maliInternalFormatIndex   = 91
	maliPlaneOffsetIndex      = 93
	maliPlaneByteStrideIndex  = 94
	maliPlaneAllocWidthIndex  = 95
	maliPlaneAllocHeightIndex = 96
)

func maliDescriptor(width, height int32) ([]uint32, error) {
	if width <= 0 || height <= 0 {
		return nil, errors.New("buffer dimensions must be positive")
	}
	return []uint32{
		maliDescriptorVersion,
		uint32(width),
		uint32(height),
		1,
		uint32(gfx.PixelFormatRgba8888),
		uint32(maliRGBAUsage),
		uint32(maliRGBAUsage >> 32),
		0,
		0,
	}, nil
}

func maliRGBALayout(b *Buffer) (rgbaLayout, error) {
	var out rgbaLayout
	if b == nil || len(b.Handle.Fds) != 2 ||
		b.Handle.Fds[0] < 0 ||
		b.Handle.Fds[1] < 0 ||
		len(b.Handle.Ints) != maliHandleIntCount {
		return out, errors.New("unsupported Mali handle shape")
	}
	a := b.Handle.Ints
	if a[0] != maliHandleMagic {
		return out, errors.New("unsupported Mali handle magic")
	}
	if b.Width == 0 || b.Height == 0 ||
		uint32(a[maliWidthIndex]) != b.Width ||
		uint32(a[maliHeightIndex]) != b.Height ||
		a[maliFormatIndex] != int32(gfx.PixelFormatRgba8888) ||
		b.Format != int32(gfx.PixelFormatRgba8888) {
		return out, errors.New("inconsistent Mali buffer dimensions or format")
	}
	producerUsage := handleUint64(a, maliProducerUsageIndex)
	consumerUsage := handleUint64(a, maliConsumerUsageIndex)
	if producerUsage != maliRGBAUsage || consumerUsage != maliRGBAUsage || b.Usage != maliRGBAUsage {
		return out, errors.New("unsupported Mali buffer usage")
	}
	if handleUint64(a, maliInternalFormatIndex) != uint64(gfx.PixelFormatRgba8888) {
		return out, errors.New("unsupported Mali internal format or compression")
	}
	stridePixels := handleUint64(a, maliStrideIndex)
	byteStride := uint64(uint32(a[maliPlaneByteStrideIndex]))
	allocWidth := uint64(uint32(a[maliPlaneAllocWidthIndex]))
	allocHeight := uint64(uint32(a[maliPlaneAllocHeightIndex]))
	offset := uint64(uint32(a[maliPlaneOffsetIndex]))
	if a[maliPlaneByteStrideIndex] <= 0 ||
		a[maliPlaneAllocWidthIndex] <= 0 ||
		a[maliPlaneAllocHeightIndex] <= 0 ||
		stridePixels < uint64(b.Width) ||
		stridePixels > math.MaxInt/rgbaBytesPerPixel ||
		byteStride != stridePixels*rgbaBytesPerPixel ||
		allocWidth < uint64(b.Width) ||
		allocHeight < uint64(b.Height) ||
		offset != 0 ||
		b.Stride <= 0 ||
		uint64(b.Stride) != stridePixels {
		return out, errors.New("invalid Mali RGBA plane layout")
	}
	stride := int(byteStride)
	if uint64(b.Height) > (math.MaxInt-offset)/uint64(stride) {
		return out, errors.New("Mali buffer dimensions overflow")
	}
	length := int(offset + uint64(stride)*uint64(b.Height))
	var stat unix.Stat_t
	if err := unix.Fstat(int(b.Handle.Fds[0]), &stat); err != nil {
		return out, fmt.Errorf("stat Mali buffer: %w", err)
	}
	if stat.Size < int64(length) {
		return out, fmt.Errorf("Mali pixel plane exceeds dma-buf size: need %d, have %d", length, stat.Size)
	}
	out = rgbaLayout{fd: int(b.Handle.Fds[0]), offset: int(offset), stride: stride, length: length}
	return out, nil
}
