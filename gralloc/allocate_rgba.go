package gralloc

import (
	"context"
	"errors"
	"fmt"

	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/hwservicemanager"
	"github.com/AndroidGoLab/binder/kernelbinder"
	"github.com/AndroidGoLab/binder/servicemanager"
)

// maliAllocatorFQName identifies the HIDL allocator used by the Mali profile.
//
// Example: query its default instance after allocator@2 is unavailable.
const maliAllocatorFQName = "android.hardware.graphics.allocator@3.0::IAllocator"

// AllocateRGBA allocates buffers for a verified platform-specific capture ABI.
//
// Example: allocate CPU-readable buffers for a virtual display.
func AllocateRGBA(
	ctx context.Context,
	sm *servicemanager.ServiceManager,
	api int,
	width int32,
	height int32,
	count int,
) ([]*Buffer, error) {
	switch api {
	case 34:
		return AllocatePixelRGBA(ctx, sm, api, width, height, count)
	case 29:
	default:
		return nil, fmt.Errorf("unsupported capture allocator API %d", api)
	}
	if width <= 0 || height <= 0 {
		return nil, errors.New("buffer dimensions must be positive")
	}
	if count < 1 || count > 64 {
		return nil, errors.New("invalid buffer count")
	}
	driver, err := kernelbinder.Open(ctx, binder.WithDevicePath("/dev/hwbinder"), binder.WithMapSize(256*1024))
	if err != nil {
		return nil, fmt.Errorf("open hwbinder: %w", err)
	}
	return allocateAndroid10RGBA(ctx, driver, width, height, count)
}

// allocateAndroid10RGBA probes Android 10 allocators and owns the connection.
//
// Example: try allocator@3 when allocator@2 has no service handle.
func allocateAndroid10RGBA(
	ctx context.Context,
	driver binder.Transport,
	width int32,
	height int32,
	count int,
) (buffers []*Buffer, resultErr error) {
	defer // Release the connection and discard buffers if closing fails.
	//
	// Example: a driver close failure closes a successfully allocated batch.
	func() {
		resultErr = errors.Join(resultErr, driver.Close(context.Background()))
		if resultErr != nil {
			resultErr = errors.Join(resultErr, CloseBuffers(buffers))
			buffers = nil
		}
	}()
	sm := hwservicemanager.New(driver)
	handle, err := sm.GetService(ctx, qcomAllocatorFQName, "default")
	if err != nil {
		return nil, fmt.Errorf("get allocator@2.0: %w", err)
	}
	if handle != 0 {
		return allocateQcomRGBA(ctx, driver, handle, width, height, count)
	}
	handle, err = sm.GetService(ctx, maliAllocatorFQName, "default")
	if err != nil {
		return nil, fmt.Errorf("get allocator@3.0: %w", err)
	}
	if handle == 0 {
		return nil, errors.New("allocator@3.0 unavailable")
	}
	return allocateMaliRGBA(ctx, driver, handle, width, height, count)
}
