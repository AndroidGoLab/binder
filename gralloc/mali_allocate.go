package gralloc

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/AndroidGoLab/binder/android/hardware/common"
	gfx "github.com/AndroidGoLab/binder/android/hardware/graphics/common"
	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/gralloc/hidlalloc"
	"golang.org/x/sys/unix"
)

const (
	maliDescriptorVersion = 30
	maliHandleMagic       = 0x03141592
	maliHandleIntCount    = 129
	maliRGBAUsage         = uint64(gfx.BufferUsageCpuReadOften | gfx.BufferUsageGpuTexture | gfx.BufferUsageGpuRenderTarget | gfx.BufferUsageComposerOverlay)

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

func allocateMaliRGBA(ctx context.Context, driver binder.Transport, handle uint32, width, height int32, count int) (_ []*Buffer, resultErr error) {
	if count < 1 || count > 64 {
		return nil, errors.New("invalid Mali buffer count")
	}
	desc, err := maliDescriptor(width, height)
	if err != nil {
		return nil, err
	}
	var buffers []*Buffer
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, CloseBuffers(buffers))
		}
	}()

	for range count {
		result, err := hidlalloc.AllocateDescriptor(ctx, driver, handle, desc, 1)
		if err != nil {
			return nil, fmt.Errorf("allocate Mali buffer: %w", err)
		}
		if result.Error != 0 {
			return nil, fmt.Errorf("Mali allocator returned error %d", result.Error)
		}
		buffer := &Buffer{
			Handle: common.NativeHandle{Fds: result.Fds, Ints: result.Ints},
			Stride: result.Stride,
			Width:  uint32(width),
			Height: uint32(height),
			Format: int32(gfx.PixelFormatRgba8888),
			Usage:  maliRGBAUsage,
		}
		buffers = append(buffers, buffer)
		if _, err := maliRGBALayout(buffer); err != nil {
			return nil, fmt.Errorf("unsupported Mali allocation: %w", err)
		}
	}
	return buffers, nil
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
	producerUsage := uint64(uint32(a[maliProducerUsageIndex])) | uint64(uint32(a[maliProducerUsageIndex+1]))<<32
	consumerUsage := uint64(uint32(a[maliConsumerUsageIndex])) | uint64(uint32(a[maliConsumerUsageIndex+1]))<<32
	if producerUsage != maliRGBAUsage || consumerUsage != maliRGBAUsage || b.Usage != maliRGBAUsage {
		return out, errors.New("unsupported Mali buffer usage")
	}
	if uint64(uint32(a[maliInternalFormatIndex]))|uint64(uint32(a[maliInternalFormatIndex+1]))<<32 != uint64(gfx.PixelFormatRgba8888) {
		return out, errors.New("unsupported Mali internal format or compression")
	}
	stridePixels := uint64(uint32(a[maliStrideIndex])) | uint64(uint32(a[maliStrideIndex+1]))<<32
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
