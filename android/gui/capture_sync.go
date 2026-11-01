package gui

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/logger"
	"github.com/AndroidGoLab/binder/parcel"
)

// captureReply carries a decoded capture result or its error.
//
// Example: deliver one result to the waiting caller.
type captureReply struct {
	result *ScreenCaptureResults
	err    error
}

// captureListener coordinates callback delivery and abandonment.
//
// Example: a late callback closes its result after cancellation.
type captureListener struct {
	mu        sync.Mutex
	abandoned bool
	completed bool
	result    chan captureReply
}

// Descriptor identifies the callback interface.
//
// Example: validate the callback's Binder interface token.
func (*captureListener) Descriptor() string { return DescriptorIScreenCaptureListener }

// OnTransaction decodes and delivers one callback.
//
// Example: close a late result after the caller cancels.
func (c *captureListener) OnTransaction(
	ctx context.Context,
	code binder.TransactionCode,
	p *parcel.Parcel,
) (*parcel.Parcel, error) {
	var reply captureReply
	if code != TransactionIScreenCaptureListenerOnScreenCaptureCompleted {
		reply.err = fmt.Errorf("unexpected capture callback code %d", code)
	} else {
		token, err := p.ReadInterfaceToken()
		if err != nil {
			reply.err = err
		} else if token != DescriptorIScreenCaptureListener {
			reply.err = fmt.Errorf("unexpected capture callback descriptor %q", token)
		} else {
			present, err := p.ReadInt32()
			if err != nil {
				reply.err = err
			} else if present != 1 {
				reply.err = errors.New("capture result missing")
			} else {
				result := new(ScreenCaptureResults)
				if err = result.UnmarshalParcel(p); err != nil {
					reply.err = err
				} else if result.FenceStatus != 0 {
					reply.err = errors.Join(fmt.Errorf("capture failed: status %d", result.FenceStatus), result.Close())
				} else if result.Buffer == nil {
					reply.err = errors.Join(errors.New("capture returned no buffer"), result.Close())
				} else {
					reply.result = result
				}
			}
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.abandoned || c.completed {
		if reply.result != nil {
			if err := reply.result.Close(); err != nil {
				logger.Warnf(ctx, "closing late capture result: %v", err)
			}
		}
		return nil, nil
	}
	c.completed = true
	c.result <- reply
	return nil, nil
}

// abandon rejects later callbacks and closes any queued result.
//
// Example: release a result racing with context cancellation.
func (c *captureListener) abandon() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.abandoned = true
	select {
	case reply := <-c.result:
		if reply.result != nil {
			return reply.result.Close()
		}
	default:
	}
	return nil
}

// CaptureDisplayByIdSync submits a capture and waits for its result. The caller
// owns the returned result and must close it; CopyRGBA waits for its fence.
// Android 14 r35 honors pixel format, scale, dataspace, and seamless-transition
// hint in args; its by-ID implementation ignores the other capture fields.
//
// Example: capture a physical display and defer the result's Close.
func (p *SurfaceComposerProxy) CaptureDisplayByIdSync(
	ctx context.Context,
	id int64,
	args CaptureArgs,
) (*ScreenCaptureResults, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if p == nil || p.Remote == nil || p.Remote.Transport() == nil {
		return nil, errors.New("capture requires a transport")
	}
	t := p.Remote.Transport()
	protocol, ok := t.(interface{ UsesSurfaceComposerAndroid14R35() bool })
	if !ok || !protocol.UsesSurfaceComposerAndroid14R35() {
		if _, err := p.Remote.ResolveCode(ctx, DescriptorISurfaceComposer, MethodISurfaceComposerCaptureDisplayById); err != nil {
			return nil, fmt.Errorf("unsupported SurfaceComposer capture protocol: %w", err)
		}
		return nil, errors.New("unsupported SurfaceComposer capture protocol")
	}
	unregister, ok := t.(interface{ CanUnregisterReceiver() bool })
	if !ok || !unregister.CanUnregisterReceiver() {
		return nil, errors.New("transport cannot unregister capture listener")
	}
	listener := &captureListener{result: make(chan captureReply, 1)}
	stub := binder.NewStubBinder(listener)
	stub.RegisterWithTransport(context.Background(), t)
	if err := p.CaptureDisplayById(ctx, id, args, NewScreenCaptureListenerProxy(stub)); err != nil {
		return nil, errors.Join(err, listener.abandon(), stub.Release(context.Background()))
	}
	select {
	case reply := <-listener.result:
		if err := stub.Release(context.Background()); err != nil {
			if reply.result != nil {
				err = errors.Join(err, reply.result.Close())
			}
			return nil, errors.Join(reply.err, err)
		}
		return reply.result, reply.err
	case <-ctx.Done():
		return nil, errors.Join(ctx.Err(), listener.abandon(), stub.Release(context.Background()))
	}
}
