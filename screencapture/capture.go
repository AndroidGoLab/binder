// Package screencapture captures Android displays as owned RGBA frames.
package screencapture

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"

	"github.com/AndroidGoLab/binder/android/gui"
	gfxCommon "github.com/AndroidGoLab/binder/android/hardware/graphics/common"
	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/gralloc"
)

// unsetCaptureUID requests all application UIDs in a display capture.
//
// Example: include layers from multiple applications.
const unsetCaptureUID = -1

// Frame contains tightly packed RGBA pixels and SurfaceFlinger's capture metadata.
//
// Example: write Pixels as width * height consecutive RGBA8888 pixels.
type Frame struct {
	Pixels               []byte
	Width                uint32
	Height               uint32
	CapturedSecureLayers bool
	CapturedHDRLayers    bool
	Dataspace            uint32
}

// ErrCapturePending indicates that another capture request is in progress.
//
// Example: retry after the other request completes.
var ErrCapturePending = errors.New("previous capture is still pending")

// receiverUnregisterer removes a local Binder callback from its transport.
//
// Example: release a completed screen capture listener.
type receiverUnregisterer interface {
	// UnregisterReceiver removes a registered listener.
	//
	// Example: remove a callback after cancellation.
	UnregisterReceiver(context.Context, uintptr) error
	// CanUnregisterReceiver reports whether removal is supported.
	//
	// Example: reject a transport that cannot release listeners.
	CanUnregisterReceiver() bool
}

// surfaceComposerR35 identifies the explicitly selected native wire protocol.
//
// Example: reject an API 34 device whose SurfaceFlinger signature is unknown.
type surfaceComposerR35 interface {
	// UsesSurfaceComposerAndroid14R35 reports the selected native contract.
	//
	// Example: reject a transport whose native wire format is unknown.
	UsesSurfaceComposerAndroid14R35() bool
}

// Capture coordinates requests over a caller-owned SurfaceFlinger connection.
// Only one request may be outstanding at a time.
//
// Example: create a client for an existing SurfaceFlinger Binder connection.
type Capture struct {
	composer   *gui.SurfaceComposerProxy
	transport  binder.VersionAwareTransport
	unregister receiverUnregisterer
	pending    chan struct{}
	closed     chan struct{}
	closeOnce  sync.Once
}

// New creates a capture client using an existing SurfaceFlinger Binder object.
// Its transport must explicitly select the android-14.0.0_r35 protocol and
// support receiver unregistration. The caller owns the transport and must keep
// it alive through callbacks.
//
// Example: pass the SurfaceFlingerAIDL service returned by the service manager.
func New(remote binder.IBinder) (*Capture, error) {
	if remote == nil || remote.Transport() == nil {
		return nil, errors.New("screen capture requires a transport")
	}
	protocol, ok := remote.Transport().(surfaceComposerR35)
	if !ok || !protocol.UsesSurfaceComposerAndroid14R35() {
		return nil, errors.New("screen capture requires the explicit android-14.0.0_r35 SurfaceFlinger protocol")
	}
	unregister, ok := remote.Transport().(receiverUnregisterer)
	if !ok || !unregister.CanUnregisterReceiver() {
		return nil, errors.New("transport cannot unregister callbacks")
	}
	return &Capture{composer: gui.NewSurfaceComposerProxy(remote), transport: remote.Transport(), unregister: unregister, pending: make(chan struct{}, 1), closed: make(chan struct{})}, nil
}

// Close ends the client; the caller remains responsible for closing its transport.
//
// Example: defer capture.Close() after constructing the client.
func (c *Capture) Close() error {
	c.closeOnce.Do(
		// Signal all waiting requests exactly once.
		//
		// Example: concurrent Close calls share the same signal.
		func() { close(c.closed) },
	)
	return nil
}

// begin reserves the one outstanding capture slot.
//
// Example: reject a second request while the first is unresolved.
func (c *Capture) begin() error {
	select {
	case c.pending <- struct{}{}:
		return nil
	default:
		return ErrCapturePending
	}
}

// end releases the outstanding capture slot.
//
// Example: a callback allows the next capture to start.
func (c *Capture) end() { <-c.pending }

// PhysicalDisplayIDs returns stable IDs of connected physical displays.
//
// Example: use the first returned ID in CaptureDisplay.
func (c *Capture) PhysicalDisplayIDs(ctx context.Context) ([]int64, error) {
	select {
	case <-c.closed:
		return nil, errors.New("capture closed")
	default:
	}
	return c.composer.GetPhysicalDisplayIds(ctx)
}

// CaptureDisplay captures a physical display in RGBA8888 at the given scale.
// It returns an error when the native buffer layout cannot be mapped.
//
// Example: use scale 1 for the display's natural size.
func (c *Capture) CaptureDisplay(
	ctx context.Context,
	id int64,
	scale float32,
) (*Frame, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if scale <= 0 || math.IsNaN(float64(scale)) || math.IsInf(float64(scale), 0) {
		return nil, fmt.Errorf("invalid capture scale %v", scale)
	}
	select {
	case <-c.closed:
		return nil, errors.New("capture closed")
	default:
	}
	if _, err := c.composer.Remote.ResolveCode(ctx, gui.DescriptorISurfaceComposer, gui.MethodISurfaceComposerCaptureDisplayById); err != nil {
		return nil, fmt.Errorf("resolve capture method: %w", err)
	}
	if err := c.begin(); err != nil {
		return nil, err
	}
	callback := newCaptureCallback()
	callback.owner = c
	callback.stub = binder.NewStubBinder(callback)
	// The shared Binder read loop must outlive this request context.
	callback.stub.RegisterWithTransport(context.Background(), c.transport)
	args := gui.CaptureArgs{PixelFormat: int32(gfxCommon.PixelFormatRgba8888), FrameScaleX: scale, FrameScaleY: scale, Uid: unsetCaptureUID}
	if err := c.composer.CaptureDisplayById(ctx, id, args, gui.NewScreenCaptureListenerProxy(callback.stub)); err != nil {
		callback.abandon()
		callback.finish(context.Background())
		return nil, err
	}
	select {
	case result := <-callback.result:
		if result.err != nil {
			return nil, result.err
		}
		return copyFrame(ctx, result.frame)
	case <-ctx.Done():
		callback.abandon()
		callback.finish(context.Background())
		return nil, ctx.Err()
	case <-c.closed:
		callback.abandon()
		callback.finish(context.Background())
		return nil, errors.New("capture closed")
	}
}

// copyFrame waits for rendering and copies pixels into an owned Go slice.
//
// Example: the native descriptors are closed before the frame is returned.
func copyFrame(
	ctx context.Context,
	native *nativeFrame,
) (_ *Frame, resultErr error) {
	if native == nil {
		return nil, errors.New("missing native frame")
	}
	defer // Release native descriptors on every return path.
	//
	// Example: an unsuccessful pixel copy still releases its fence.
	func() { resultErr = errors.Join(resultErr, native.Close()) }()
	if native.Buffer == nil {
		return nil, errors.New("missing capture buffer")
	}
	if err := waitFence(ctx, native.FenceFD); err != nil {
		return nil, err
	}
	pixels, err := gralloc.CopyRGBA(native.Buffer)
	if err != nil {
		return nil, fmt.Errorf("copy capture pixels: %w", err)
	}
	return &Frame{Pixels: pixels, Width: native.Buffer.Width, Height: native.Buffer.Height, CapturedSecureLayers: native.CapturedSecureLayers, CapturedHDRLayers: native.CapturedHdrLayers, Dataspace: native.CapturedDataspace}, nil
}
