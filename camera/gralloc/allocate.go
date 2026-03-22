package gralloc

import (
	"context"
	"fmt"

	"github.com/xaionaro-go/binder/android/hardware/graphics/allocator"
	gfxCommon "github.com/xaionaro-go/binder/android/hardware/graphics/common"
	"github.com/xaionaro-go/binder/servicemanager"
)

// Allocate allocates a gralloc buffer using the IAllocator HAL service.
// The returned Buffer contains a dmabuf FD that can be mmap'd for CPU
// read access.
func Allocate(
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
