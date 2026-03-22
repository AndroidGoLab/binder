package camera

import (
	"context"
	"errors"
	"fmt"
	"time"

	gfxCommon "github.com/xaionaro-go/binder/android/hardware/graphics/common"

	fwkDevice "github.com/xaionaro-go/binder/android/frameworks/cameraservice/device"
	fwkService "github.com/xaionaro-go/binder/android/frameworks/cameraservice/service"
	"github.com/xaionaro-go/binder/binder"
	cameraIGBP "github.com/xaionaro-go/binder/camera/igbp"
	"github.com/xaionaro-go/binder/camera/gralloc"
	"github.com/xaionaro-go/binder/servicemanager"
)

// Device represents a connected camera device with a configured capture
// stream. Use Connect to create, ConfigureStream to set up the stream,
// CaptureFrame to read frames, and Close to disconnect.
type Device struct {
	sm        *servicemanager.ServiceManager
	transport binder.Transport

	deviceUser fwkDevice.ICameraDeviceUser
	callback   *deviceCallback

	// Set after ConfigureStream.
	igbpStub    *cameraIGBP.ProducerStub
	grallocBufs [4]*gralloc.Buffer
	streamID    int32
	metadata    []byte
	width       int32
	height      int32
}

// Connect opens a connection to the camera device identified by cameraID
// (typically "0" for the back camera).
func Connect(
	ctx context.Context,
	sm *servicemanager.ServiceManager,
	transport binder.Transport,
	cameraID string,
) (_ *Device, _err error) {
	svc, err := sm.GetService(ctx, "android.frameworks.cameraservice.service.ICameraService/default")
	if err != nil {
		return nil, fmt.Errorf("getting camera service: %w", err)
	}

	proxy := fwkService.NewCameraServiceProxy(svc)
	cb := &deviceCallback{}
	stub := fwkDevice.NewCameraDeviceCallbackStub(cb)

	stubBinder := stub.AsBinder().(*binder.StubBinder)
	stubBinder.RegisterWithTransport(ctx, transport)
	time.Sleep(100 * time.Millisecond)

	deviceUser, err := proxy.ConnectDevice(ctx, stub, cameraID)
	if err != nil {
		return nil, fmt.Errorf("ConnectDevice: %w", err)
	}

	dev := &Device{
		sm:         sm,
		transport:  transport,
		deviceUser: deviceUser,
		callback:   cb,
	}

	return dev, nil
}

// ConfigureStream sets up a capture stream with the given dimensions and
// pixel format. It allocates gralloc buffers, creates an IGBP surface
// stub, and configures the camera for streaming.
func (d *Device) ConfigureStream(
	ctx context.Context,
	width int32,
	height int32,
	format Format,
) error {
	d.width = width
	d.height = height

	// Allocate gralloc buffers.
	for i := range d.grallocBufs {
		buf, err := gralloc.Allocate(
			ctx,
			d.sm,
			width,
			height,
			format,
			gfxCommon.BufferUsageCpuReadOften|gfxCommon.BufferUsageCameraOutput,
		)
		if err != nil {
			return fmt.Errorf("allocating gralloc buffer %d: %w", i, err)
		}
		if err := buf.Mmap(); err != nil {
			return fmt.Errorf("mmap gralloc buffer %d: %w", i, err)
		}
		d.grallocBufs[i] = buf
	}

	// Begin configuration.
	if err := d.deviceUser.BeginConfigure(ctx); err != nil {
		return fmt.Errorf("BeginConfigure: %w", err)
	}

	// Get default request metadata.
	metadata, err := CreateDefaultRequest(ctx, d.deviceUser, fwkDevice.TemplateIdPREVIEW)
	if err != nil {
		return fmt.Errorf("CreateDefaultRequest: %w", err)
	}
	d.metadata = metadata

	// Create IGBP stub and stream.
	d.igbpStub = cameraIGBP.NewProducerStub(uint32(width), uint32(height), d.grallocBufs)
	igbpStubBinder := binder.NewStubBinder(d.igbpStub)
	igbpStubBinder.RegisterWithTransport(ctx, d.transport)

	streamID, err := CreateStreamWithSurface(ctx, d.deviceUser, d.transport, igbpStubBinder, width, height)
	if err != nil {
		return fmt.Errorf("CreateStream: %w", err)
	}
	d.streamID = streamID

	// End configuration.
	if err := d.deviceUser.EndConfigure(
		ctx,
		fwkDevice.StreamConfigurationModeNormalMode,
		fwkDevice.CameraMetadata{Metadata: []byte{}},
		0,
	); err != nil {
		return fmt.Errorf("EndConfigure: %w", err)
	}

	return nil
}

// CaptureFrame captures a single frame and returns the raw pixel data.
// The caller should have called ConfigureStream first. This method
// submits a repeating capture request on the first call, then reads
// from the IGBP queue channel on subsequent calls.
func (d *Device) CaptureFrame(
	ctx context.Context,
) ([]byte, error) {
	if d.igbpStub == nil {
		return nil, fmt.Errorf("stream not configured; call ConfigureStream first")
	}

	// Submit a repeating capture request (only needs to happen once,
	// but submitting again is harmless and simplifies the API).
	captureReq := fwkDevice.CaptureRequest{
		PhysicalCameraSettings: []fwkDevice.PhysicalCameraSettings{
			{
				Id: "0",
				Settings: fwkDevice.CaptureMetadataInfo{
					Tag:      fwkDevice.CaptureMetadataInfoTagMetadata,
					Metadata: fwkDevice.CameraMetadata{Metadata: d.metadata},
				},
			},
		},
		StreamAndWindowIds: []fwkDevice.StreamAndWindowId{
			{StreamId: d.streamID, WindowId: 0},
		},
	}

	_, err := SubmitRequest(ctx, d.deviceUser, captureReq, true)
	if err != nil {
		// Fallback to single shot.
		_, err = SubmitRequest(ctx, d.deviceUser, captureReq, false)
		if err != nil {
			return nil, fmt.Errorf("SubmitRequestList: %w", err)
		}
	}

	// Wait for a frame to be queued.
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case slot := <-d.igbpStub.QueuedFrames():
		buf := d.igbpStub.SlotBuffer(slot)
		if buf == nil || buf.MmapData == nil {
			return nil, fmt.Errorf("slot %d has no mmap'd buffer", slot)
		}
		// Return a copy so the caller owns the data.
		frame := make([]byte, len(buf.MmapData))
		copy(frame, buf.MmapData)
		return frame, nil
	}
}

// Close disconnects from the camera device and releases gralloc buffers.
func (d *Device) Close(ctx context.Context) error {
	var errs []error

	if d.deviceUser != nil {
		if err := d.deviceUser.Disconnect(ctx); err != nil {
			errs = append(errs, fmt.Errorf("disconnect: %w", err))
		}
	}

	for _, buf := range d.grallocBufs {
		if buf != nil {
			buf.Munmap()
		}
	}

	return errors.Join(errs...)
}
