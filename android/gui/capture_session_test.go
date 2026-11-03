package gui

import (
	"context"
	"errors"

	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/gralloc"
	"github.com/AndroidGoLab/binder/igbp"
	"github.com/AndroidGoLab/binder/parcel"
	"testing"
)

func TestOpenCaptureRejectsInvalidDimensions(t *testing.T) {
	_, err := OpenCapture(context.Background(), CaptureOptions{SourceWidth: 1, SourceHeight: 1, Width: 0, Height: 1})
	if err == nil {
		t.Fatal("expected invalid dimensions to be rejected")
	}
}

func TestCaptureSessionCloseWaitsForBorrowedFrame(t *testing.T) {
	producer, err := igbp.NewFrameProducer(34, []*gralloc.Buffer{{Width: 1, Height: 1, Stride: 1, Format: 1}})
	if err != nil {
		t.Fatal(err)
	}
	req := parcel.New()
	req.WriteInterfaceToken(igbp.Descriptor)
	req.WriteUint32(1)
	req.WriteUint32(1)
	req.WriteInt32(1)
	req.WriteUint64(0)
	req.WriteBool(false)
	req.SetPosition(0)
	if _, err = producer.OnTransaction(context.Background(), igbp.DequeueBuffer, req); err != nil {
		t.Fatal(err)
	}
	req = parcel.New()
	req.WriteInterfaceToken(igbp.Descriptor)
	req.WriteInt32(0)
	req.WriteInt32(61)
	req.WriteInt32(0)
	req.WriteRawBytes(make([]byte, 61))
	req.SetPosition(0)
	if _, err = producer.OnTransaction(context.Background(), igbp.QueueBuffer, req); err != nil {
		t.Fatal(err)
	}
	s := &CaptureSession{producer: producer}
	frame, err := s.Next(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.Close(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("close with borrowed frame: %v", err)
	}
	if err := frame.Release(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Next(context.Background()); err == nil {
		t.Fatal("closed session accepted Next")
	}
}

type failingDisplayRemote struct {
	binder.IBinder
	destroyFails bool
}

func (r *failingDisplayRemote) Transact(_ context.Context, code binder.TransactionCode, _ binder.TransactionFlags, _ *parcel.Parcel) (*parcel.Parcel, error) {
	switch code {
	case 5:
		p := parcel.New()
		p.WriteStrongBinderLegacy(19)
		return p, nil
	case 8:
		return nil, errors.New("configure failed")
	case 6:
		if r.destroyFails {
			return nil, errors.New("destroy failed")
		}
		return parcel.New(), nil
	}
	return nil, errors.New("unexpected transaction")
}
func TestCaptureRetainsDisplayWhenDestroyFails(t *testing.T) {
	remote := &failingDisplayRemote{destroyFails: true}
	_, err := CreateVirtualDisplay(context.Background(), remote, nil, &virtualDisplayTransport{}, 29, binder.NewStubBinder(nil), 1, 1, 1, 1)
	var pending *VirtualDisplayCleanupError
	if !errors.As(err, &pending) {
		t.Fatalf("missing retryable display: %v", err)
	}
	s := &CaptureSession{display: pending.Display}
	remote.destroyFails = false
	if err := s.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestCaptureConfigureFailureWithSuccessfulDestroyNeedsNoRetry(t *testing.T) {
	remote := &failingDisplayRemote{}
	_, err := CreateVirtualDisplay(context.Background(), remote, nil, &virtualDisplayTransport{}, 29, binder.NewStubBinder(nil), 1, 1, 1, 1)
	var pending *VirtualDisplayCleanupError
	if err == nil || errors.As(err, &pending) {
		t.Fatalf("expected cleaned configure failure, got %v", err)
	}
}

type failingCaptureDriver struct{}

func (failingCaptureDriver) Close(context.Context) error { return errors.New("driver close failed") }
func TestCaptureCloseContinuesAfterDriverError(t *testing.T) {
	s := &CaptureSession{driver: failingCaptureDriver{}, stub: binder.NewStubBinder(nil)}
	if err := s.Close(context.Background()); err == nil {
		t.Fatal("expected driver error")
	}
	if s.driver != nil || s.stub != nil {
		t.Fatal("terminal driver or stub was retained")
	}
	if err := s.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}
