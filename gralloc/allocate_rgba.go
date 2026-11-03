package gralloc

import (
	"context"
	"errors"
	"fmt"
	"runtime"

	"github.com/AndroidGoLab/binder/android/hardware/common"
	gfx "github.com/AndroidGoLab/binder/android/hardware/graphics/common"
	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/gralloc/hidlalloc"
	"github.com/AndroidGoLab/binder/hwservicemanager"
	"github.com/AndroidGoLab/binder/kernelbinder"
	"github.com/AndroidGoLab/binder/servicemanager"
)

// maliAllocatorFQName identifies the HIDL allocator used by the Mali profile.
//
// Example: query its default instance after allocator@2 is unavailable.
const maliAllocatorFQName = "android.hardware.graphics.allocator@3.0::IAllocator"

// android10Profile selects one verified Android 10 allocator ABI.
//
// Example: select Qualcomm when allocator@2 is registered.
type android10Profile uint8

const (
	// android10Qcom selects the Qualcomm allocator@2 ABI.
	//
	// Example: validate an SDM845 private handle.
	android10Qcom android10Profile = iota
	// android10Mali selects the Mali allocator@3 ABI.
	//
	// Example: validate the supported Bifrost handle.
	android10Mali
)

// allocateAndroid10Buffers allocates and validates a batch for one vendor ABI.
//
// Example: close earlier buffers when a later allocation fails validation.
func allocateAndroid10Buffers(
	ctx context.Context,
	driver binder.Transport,
	handle uint32,
	width int32,
	height int32,
	count int,
	profile android10Profile,
) (_ []*Buffer, resultErr error) {
	var name, token string
	var usage uint64
	var desc []uint32
	var err error
	switch profile {
	case android10Qcom:
		name, token, usage = "Qualcomm", qcomAllocatorFQName, qcomRGBAUsage
		desc, err = qcomDescriptor(width, height)
	case android10Mali:
		name, token, usage = "Mali", maliAllocatorFQName, maliRGBAUsage
		desc, err = maliDescriptor(width, height)
	default:
		return nil, errors.New("unsupported Android 10 allocation profile")
	}
	if count < 1 || count > 64 {
		return nil, fmt.Errorf("invalid %s buffer count", name)
	}
	if err != nil {
		return nil, err
	}
	var buffers []*Buffer
	defer // Release every acquired handle if the batch cannot be returned.
	//
	// Example: reject the second buffer without leaking the first.
	func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, CloseBuffers(buffers))
		}
	}()
	for range count {
		result, err := hidlalloc.AllocateDescriptorForInterface(ctx, driver, handle, token, desc, 1)
		if err != nil {
			return nil, fmt.Errorf("allocate %s buffer: %w", name, err)
		}
		if result.Error != 0 {
			if profile == android10Qcom {
				return nil, fmt.Errorf("HIDL allocator error=%d", result.Error)
			}
			return nil, fmt.Errorf("%s allocator returned error %d", name, result.Error)
		}
		buffer := &Buffer{Handle: common.NativeHandle{Fds: result.Fds, Ints: result.Ints}, Stride: result.Stride, Width: uint32(width), Height: uint32(height), Format: int32(gfx.PixelFormatRgba8888), Usage: usage}
		buffers = append(buffers, buffer)
		switch profile {
		case android10Qcom:
			_, err = qcomRGBALayout(buffer)
		case android10Mali:
			_, err = maliRGBALayout(buffer)
		}
		if err != nil {
			return nil, fmt.Errorf("unsupported %s allocation: %w", name, err)
		}
	}
	return buffers, nil
}

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
	if runtime.GOOS == "android" && runtime.GOARCH == "arm64" {
		return allocateNativeRGBA(width, height, count)
	}
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
		return allocateAndroid10Buffers(ctx, driver, handle, width, height, count, android10Qcom)
	}
	handle, err = sm.GetService(ctx, maliAllocatorFQName, "default")
	if err != nil {
		return nil, fmt.Errorf("get allocator@3.0: %w", err)
	}
	if handle == 0 {
		return nil, errors.New("allocator@3.0 unavailable")
	}
	return allocateAndroid10Buffers(ctx, driver, handle, width, height, count, android10Mali)
}
