package gui

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/binder/versionaware"
	"github.com/AndroidGoLab/binder/gralloc"
	"github.com/AndroidGoLab/binder/igbp"
	"github.com/AndroidGoLab/binder/kernelbinder"
	"github.com/AndroidGoLab/binder/servicemanager"
)

// CaptureOptions specifies the source projection and capture buffer dimensions.
// DisplayID zero selects the primary layer stack.
type CaptureOptions struct {
	SourceWidth, SourceHeight, Width, Height int32
	DisplayID                                uint64
}

// CaptureSession owns a virtual display and its buffer producer. Release every
// acquired frame before closing the session. Methods must not run concurrently.
type CaptureSession struct {
	driver   interface{ Close(context.Context) error }
	stub     *binder.StubBinder
	producer *igbp.FrameProducer
	display  *VirtualDisplay
	buffers  []*gralloc.Buffer
	closing  bool
}

// OpenCapture starts a virtual display capture session. If it returns a non-nil
// session with an error, call Close again to finish cleanup.
func OpenCapture(ctx context.Context, opts CaptureOptions) (result *CaptureSession, resultErr error) {
	if opts.SourceWidth <= 0 || opts.SourceHeight <= 0 || opts.Width <= 0 || opts.Height <= 0 {
		return nil, errors.New("capture dimensions must be positive")
	}
	s := &CaptureSession{}
	defer func() {
		if resultErr != nil {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cleanupErr := s.Close(cleanupCtx)
			resultErr = errors.Join(resultErr, cleanupErr)
			if cleanupErr != nil {
				result = s
			}
		}
	}()
	var err error
	driver, err := kernelbinder.Open(ctx, binder.WithMapSize(4*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("open binder: %w", err)
	}
	s.driver = driver
	transport, err := versionaware.NewTransport(ctx, driver, 0, versionaware.OptionCachePath(""))
	if err != nil {
		return nil, fmt.Errorf("configure binder: %w", err)
	}
	api := transport.APILevel()
	if api == 34 {
		if err := transport.CheckSurfaceComposerAndroid14R35(); err != nil {
			return nil, err
		}
	}
	sm := servicemanager.New(transport)
	raw, err := sm.GetService(ctx, "SurfaceFlinger")
	if err != nil {
		return nil, fmt.Errorf("get SurfaceFlinger: %w", err)
	}
	var aidl binder.IBinder
	if api == 34 {
		aidl, err = sm.GetService(ctx, "SurfaceFlingerAIDL")
		if err != nil {
			return nil, fmt.Errorf("get SurfaceFlinger AIDL: %w", err)
		}
	}
	var stack int32
	if opts.DisplayID != 0 {
		if api != 34 {
			return nil, errors.New("secondary display capture requires a known layer stack")
		}
		ids, err := PhysicalDisplayIDs(ctx, aidl, api)
		if err != nil {
			return nil, fmt.Errorf("list physical displays: %w", err)
		}
		found := false
		for _, id := range ids {
			if id == opts.DisplayID {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("display %d not found", opts.DisplayID)
		}
		info, err := GetPhysicalDisplayInfo(ctx, aidl, api, opts.DisplayID)
		if err != nil {
			return nil, fmt.Errorf("read display info: %w", err)
		}
		if info.LayerStack == nil {
			return nil, errors.New("display layer stack unavailable")
		}
		stack = *info.LayerStack
	}
	s.buffers, err = gralloc.AllocateRGBA(ctx, sm, api, opts.Width, opts.Height, 4)
	if err != nil {
		return nil, fmt.Errorf("allocate capture buffers: %w", err)
	}
	s.producer, err = igbp.NewFrameProducer(api, s.buffers)
	if err != nil {
		return nil, err
	}
	s.stub = binder.NewStubBinder(s.producer)
	s.display, err = CreateVirtualDisplayOnLayerStackWithOptions(ctx, raw, aidl, transport, api, s.stub, opts.SourceWidth, opts.SourceHeight, opts.Width, opts.Height, stack, VirtualDisplayOptions{Secure: api == binder.APILevelAndroid10})
	if err != nil {
		var pending *VirtualDisplayCleanupError
		if errors.As(err, &pending) {
			s.display = pending.Display
		}
		return nil, err
	}
	return s, nil
}

// Next waits for a frame. The caller must release the returned lease.
func (s *CaptureSession) Next(ctx context.Context) (*igbp.Frame, error) {
	if s.closing || s.producer == nil {
		return nil, errors.New("capture session closed")
	}
	return s.producer.Next(ctx)
}

// TryNext acquires a queued frame without waiting.
func (s *CaptureSession) TryNext() (*igbp.Frame, bool) {
	if s.closing || s.producer == nil {
		return nil, false
	}
	return s.producer.TryNext()
}

// Close stops capture and frees resources after all frame leases are released.
// If teardown fails, it can be retried with a fresh context.
func (s *CaptureSession) Close(ctx context.Context) error {
	s.closing = true
	if s.display != nil {
		if err := s.display.Close(ctx); err != nil {
			return err
		}
		s.display = nil
	}
	if s.producer != nil {
		if err := s.producer.Close(ctx); err != nil {
			return err
		}
		s.producer = nil
	}
	if s.buffers != nil {
		if err := gralloc.CloseBuffers(s.buffers); err != nil {
			return err
		}
		s.buffers = nil
	}
	var closeErr error
	if s.driver != nil {
		closeErr = s.driver.Close(ctx)
		s.driver = nil
	}
	if s.stub != nil {
		if err := s.stub.Release(ctx); err != nil {
			return errors.Join(closeErr, err)
		}
		s.stub = nil
	}
	return closeErr
}
