package gralloc

import (
	"context"
	"fmt"

	"github.com/AndroidGoLab/binder/android/hardware/graphics/allocator"
	gfxCommon "github.com/AndroidGoLab/binder/android/hardware/graphics/common"
	common "github.com/AndroidGoLab/binder/android/hardware/common"
	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/gralloc/dmaheap"
	"github.com/AndroidGoLab/binder/gralloc/hidlalloc"
	"github.com/AndroidGoLab/binder/kernelbinder"
	"github.com/AndroidGoLab/binder/logger"
	"github.com/AndroidGoLab/binder/servicemanager"

	"golang.org/x/sys/unix"
)

// Allocate allocates a gralloc buffer using the best available allocator.
// The fallback chain is:
//  1. AIDL IAllocator (android.hardware.graphics.allocator.IAllocator/default)
//  2. HIDL gralloc 3.0 IAllocator via hwbinder
//  3. dma-buf heap (/dev/dma_heap/system)
//  4. memfd (last resort; camera HAL may not accept these)
func Allocate(
	ctx context.Context,
	sm *servicemanager.ServiceManager,
	width int32,
	height int32,
	format gfxCommon.PixelFormat,
	usage gfxCommon.BufferUsage,
) (*Buffer, error) {
	// Try AIDL allocator first.
	buf, err := allocateAIDL(ctx, sm, width, height, format, usage)
	if err == nil {
		return buf, nil
	}
	logger.Debugf(ctx, "AIDL allocator unavailable: %v; trying HIDL gralloc 3.0", err)

	// Try HIDL gralloc 3.0 via hwbinder.
	buf, err = allocateHIDL(ctx, width, height, format, usage)
	if err == nil {
		return buf, nil
	}
	logger.Debugf(ctx, "HIDL gralloc 3.0 unavailable: %v; trying dma-buf heap", err)

	// Try dma-buf heap.
	buf, err = allocateDmaBufHeap(width, height, format, usage)
	if err == nil {
		return buf, nil
	}
	logger.Debugf(ctx, "dma-buf heap unavailable: %v; using memfd fallback", err)

	// Last resort: memfd. The camera HAL likely cannot import these, but
	// at least the IGBP pipeline can proceed and deliver callbacks.
	return allocateMemfd(width, height, format, usage)
}

// allocateAIDL attempts allocation via the AIDL IAllocator HAL service.
func allocateAIDL(
	ctx context.Context,
	sm *servicemanager.ServiceManager,
	width int32,
	height int32,
	format gfxCommon.PixelFormat,
	usage gfxCommon.BufferUsage,
) (*Buffer, error) {
	svc, err := sm.GetService(ctx, "android.hardware.graphics.allocator.IAllocator/default")
	if err != nil {
		return nil, fmt.Errorf("get allocator service: %w", err)
	}

	proxy := allocator.NewAllocatorProxy(svc)

	desc := allocator.BufferDescriptorInfo{
		Name:              []byte("camera-buffer"),
		Width:             width,
		Height:            height,
		LayerCount:        1,
		Format:            format,
		Usage:             usage,
		ReservedSize:      0,
		AdditionalOptions: []gfxCommon.ExtendableType{},
	}

	result, err := proxy.Allocate2(ctx, desc, 1)
	if err != nil {
		return nil, fmt.Errorf("Allocate2: %w", err)
	}

	if len(result.Buffers) == 0 {
		return nil, fmt.Errorf("Allocate2 returned 0 buffers")
	}

	return &Buffer{
		Handle: result.Buffers[0],
		Stride: result.Stride,
		Width:  uint32(width),
		Height: uint32(height),
		Format: int32(format),
		Usage:  uint64(usage),
	}, nil
}

// allocateHIDL attempts allocation via the HIDL gralloc 3.0 IAllocator.
func allocateHIDL(
	ctx context.Context,
	width int32,
	height int32,
	format gfxCommon.PixelFormat,
	usage gfxCommon.BufferUsage,
) (*Buffer, error) {
	// Open a separate hwbinder connection.
	hwDriver, err := kernelbinder.Open(ctx,
		binder.WithDevicePath("/dev/hwbinder"),
		binder.WithMapSize(256*1024),
	)
	if err != nil {
		return nil, fmt.Errorf("open hwbinder: %w", err)
	}
	defer func() {
		if closeErr := hwDriver.Close(ctx); closeErr != nil {
			logger.Warnf(ctx, "close hwbinder: %v", closeErr)
		}
	}()

	allocHandle, err := hidlalloc.GetAllocatorService(ctx, hwDriver)
	if err != nil {
		return nil, fmt.Errorf("get HIDL allocator: %w", err)
	}

	result, err := hidlalloc.Allocate(
		ctx,
		hwDriver,
		allocHandle,
		uint32(width),
		uint32(height),
		1, // layerCount
		uint32(format),
		uint64(usage),
		1, // count
	)
	if err != nil {
		return nil, fmt.Errorf("HIDL allocate: %w", err)
	}

	if result.Error != 0 {
		return nil, fmt.Errorf("HIDL allocate returned error: %d", result.Error)
	}

	return &Buffer{
		Handle: common.NativeHandle{
			Fds:  result.Fds,
			Ints: result.Ints,
		},
		Stride: result.Stride,
		Width:  uint32(width),
		Height: uint32(height),
		Format: int32(format),
		Usage:  uint64(usage),
	}, nil
}

// allocateDmaBufHeap allocates a buffer from /dev/dma_heap/system.
func allocateDmaBufHeap(
	width int32,
	height int32,
	format gfxCommon.PixelFormat,
	usage gfxCommon.BufferUsage,
) (*Buffer, error) {
	bufSize := calcBufferSize(width, height, format)
	fd, err := dmaheap.Allocate(uint64(bufSize))
	if err != nil {
		return nil, fmt.Errorf("dma_heap allocate: %w", err)
	}

	return &Buffer{
		Handle: common.NativeHandle{
			Fds:  []int32{fd},
			Ints: []int32{},
		},
		Stride: width,
		Width:  uint32(width),
		Height: uint32(height),
		Format: int32(format),
		Usage:  uint64(usage),
	}, nil
}

// allocateMemfd allocates a buffer using memfd_create (last resort fallback).
func allocateMemfd(
	width int32,
	height int32,
	format gfxCommon.PixelFormat,
	usage gfxCommon.BufferUsage,
) (*Buffer, error) {
	bufSize := calcBufferSize(width, height, format)
	fd, err := unix.MemfdCreate("gralloc-fallback", 0)
	if err != nil {
		return nil, fmt.Errorf("memfd_create: %w", err)
	}

	if err := unix.Ftruncate(fd, int64(bufSize)); err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("ftruncate: %w", err)
	}

	return &Buffer{
		Handle: common.NativeHandle{
			Fds:  []int32{int32(fd)},
			Ints: []int32{},
		},
		Stride: width,
		Width:  uint32(width),
		Height: uint32(height),
		Format: int32(format),
		Usage:  uint64(usage),
	}, nil
}

// BytesPerPixel returns the bytes per pixel for common formats.
// Returns 0 for planar formats (YCbCr) where bytes-per-pixel is not
// a meaningful concept.
func BytesPerPixel(format gfxCommon.PixelFormat) int {
	switch format {
	case gfxCommon.PixelFormatRgba8888,
		gfxCommon.PixelFormatRgbx8888,
		gfxCommon.PixelFormatBgra8888,
		gfxCommon.PixelFormatRgba1010102:
		return 4
	case gfxCommon.PixelFormatRgb888:
		return 3
	case gfxCommon.PixelFormatRgb565:
		return 2
	case gfxCommon.PixelFormatRgbaFp16:
		return 8
	case gfxCommon.PixelFormatY8:
		return 1
	case gfxCommon.PixelFormatY16, gfxCommon.PixelFormatRAW16:
		return 2
	default:
		return 0
	}
}

// calcBufferSize estimates the buffer size for a given pixel format.
func calcBufferSize(width, height int32, format gfxCommon.PixelFormat) int64 {
	w, h := int64(width), int64(height)
	switch format {
	case gfxCommon.PixelFormatRgba8888,
		gfxCommon.PixelFormatRgbx8888,
		gfxCommon.PixelFormatBgra8888,
		gfxCommon.PixelFormatRgba1010102:
		return w * h * 4
	case gfxCommon.PixelFormatRgb888:
		return w * h * 3
	case gfxCommon.PixelFormatRgb565:
		return w * h * 2
	case gfxCommon.PixelFormatRgbaFp16:
		return w * h * 8
	case gfxCommon.PixelFormatYcbcr420888, gfxCommon.PixelFormatYcrcb420Sp:
		return w * h * 3 / 2
	case gfxCommon.PixelFormatYcbcr422Sp, gfxCommon.PixelFormatYcbcr422I:
		return w * h * 2
	case gfxCommon.PixelFormatRAW16:
		return w * h * 2
	case gfxCommon.PixelFormatRAW10:
		// 4 pixels per 5 bytes, row-aligned to 16 bytes.
		return ((w*5 + 15) / 16 * 16) * h
	case gfxCommon.PixelFormatRAW12:
		// 2 pixels per 3 bytes, row-aligned to 16 bytes.
		return ((w*3 + 15) / 16 * 16) * h
	case gfxCommon.PixelFormatY8:
		return w * h
	case gfxCommon.PixelFormatY16:
		return w * h * 2
	default:
		// Conservative estimate: 4 bytes per pixel.
		return w * h * 4
	}
}
