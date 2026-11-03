package gui

import (
	"context"
	"errors"

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
