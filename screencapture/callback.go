package screencapture

import (
	"context"
	"fmt"
	"sync"

	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/parcel"
)

// captureResult transfers a frame or error to a waiting capture call.
//
// Example: an asynchronous SurfaceFlinger callback yields one result.
type captureResult struct {
	frame *nativeFrame
	err   error
}

// captureCallback tracks ownership when the waiting call is canceled.
//
// Example: a late frame is closed when its caller has abandoned it.
type captureCallback struct {
	mu         sync.Mutex
	abandoned  bool
	result     chan captureResult
	owner      *Capture
	stub       *binder.StubBinder
	finishOnce sync.Once
}

// newCaptureCallback creates a callback with a single buffered result.
//
// Example: one callback is registered for each capture request.
func newCaptureCallback() *captureCallback {
	return &captureCallback{result: make(chan captureResult, 1)}
}

// Descriptor identifies the SurfaceFlinger listener interface.
//
// Example: Binder validates this token on incoming calls.
func (*captureCallback) Descriptor() string { return "android.gui.IScreenCaptureListener" }

// complete transfers the result or releases a frame that arrived too late.
//
// Example: a canceled capture does not leak a returned descriptor.
func (c *captureCallback) complete(
	frame *nativeFrame,
	err error,
) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.abandoned {
		if frame != nil {
			_ = frame.Close()
		}
		return
	}
	c.result <- captureResult{frame: frame, err: err}
}

// abandon releases an already queued result and rejects later results.
//
// Example: cancellation races safely with completion.
func (c *captureCallback) abandon() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.abandoned = true
	select {
	case r := <-c.result:
		if r.frame != nil {
			_ = r.frame.Close()
		}
	default:
	}
}

// finish unregisters the listener and releases its request slot once.
//
// Example: a transport failure racing with the callback releases one slot.
func (c *captureCallback) finish(ctx context.Context) {
	c.finishOnce.Do(
		// Release this registration and request slot once.
		//
		// Example: a callback and error path may both request cleanup.
		func() {
			if c.owner != nil && c.owner.unregister != nil && c.stub != nil {
				_ = c.owner.unregister.UnregisterReceiver(ctx, c.stub.Cookie())
			}
			if c.owner != nil {
				c.owner.end()
			}
		},
	)
}

// OnTransaction decodes a native screen capture callback.
//
// Example: the one result is delivered to the requesting capture.
func (c *captureCallback) OnTransaction(
	ctx context.Context,
	code binder.TransactionCode,
	p *parcel.Parcel,
) (*parcel.Parcel, error) {
	if code != binder.FirstCallTransaction {
		return nil, fmt.Errorf("unexpected screen capture callback %d", code)
	}
	token, err := p.ReadInterfaceToken()
	if err == nil && token != c.Descriptor() {
		err = fmt.Errorf("unexpected callback descriptor %q", token)
	}
	var frame *nativeFrame
	if err == nil {
		var present int32
		present, err = p.ReadInt32()
		if err == nil && present != 1 {
			err = fmt.Errorf("capture result missing")
		}
	}
	if err == nil {
		frame, err = readResult(p)
	}
	c.finish(ctx)
	c.complete(frame, err)
	return nil, nil
}
