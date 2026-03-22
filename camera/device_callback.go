// Package camera provides a high-level API for capturing frames from an
// Android camera via binder.
package camera

import (
	"context"
	"sync"

	fwkDevice "github.com/AndroidGoLab/binder/android/frameworks/cameraservice/device"
)

// deviceCallback implements fwkDevice.ICameraDeviceCallback, tracking
// how many frames the camera service has started processing.
type deviceCallback struct {
	mu             sync.Mutex
	framesReceived int
}

func (c *deviceCallback) OnCaptureStarted(
	_ context.Context,
	_ fwkDevice.CaptureResultExtras,
	_ int64,
) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.framesReceived++
	return nil
}

func (c *deviceCallback) OnDeviceError(
	_ context.Context,
	_ fwkDevice.ErrorCode,
	_ fwkDevice.CaptureResultExtras,
) error {
	return nil
}

func (c *deviceCallback) OnDeviceIdle(_ context.Context) error {
	return nil
}

func (c *deviceCallback) OnPrepared(_ context.Context, _ int32) error {
	return nil
}

func (c *deviceCallback) OnRepeatingRequestError(
	_ context.Context,
	_ int64,
	_ int32,
) error {
	return nil
}

func (c *deviceCallback) OnResultReceived(
	_ context.Context,
	_ fwkDevice.CaptureMetadataInfo,
	_ fwkDevice.CaptureResultExtras,
	_ []fwkDevice.PhysicalCaptureResultInfo,
) error {
	return nil
}
