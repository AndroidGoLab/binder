package gui

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/parcel"
)

type captureSyncTransport struct {
	binder.VersionAwareTransport
	receivers chan binder.TransactionReceiver
	sent      chan struct{}
	released  chan struct{}
}

func (t *captureSyncTransport) RegisterReceiver(_ context.Context, r binder.TransactionReceiver) uintptr {
	t.receivers <- r
	return 1
}
func (t *captureSyncTransport) UnregisterReceiver(context.Context, uintptr) error {
	t.released <- struct{}{}
	return nil
}
func (*captureSyncTransport) CanUnregisterReceiver() bool           { return true }
func (*captureSyncTransport) UsesSurfaceComposerAndroid14R35() bool { return true }
func (*captureSyncTransport) ResolveCode(context.Context, string, string) (binder.TransactionCode, error) {
	return 26, nil
}
func (*captureSyncTransport) ResolveMethodSignature(context.Context, string, string) []string {
	return nil
}
func (t *captureSyncTransport) Transact(context.Context, uint32, binder.TransactionCode, binder.TransactionFlags, *parcel.Parcel) (*parcel.Parcel, error) {
	t.sent <- struct{}{}
	return parcel.New(), nil
}
func TestCaptureDisplayByIdSyncCancelReleasesListener(t *testing.T) {
	tr := &captureSyncTransport{receivers: make(chan binder.TransactionReceiver, 1), sent: make(chan struct{}, 1), released: make(chan struct{}, 1)}
	proxy := NewSurfaceComposerProxy(binder.NewProxyBinder(tr, binder.CallerIdentity{}, 1))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := proxy.CaptureDisplayByIdSync(ctx, 1, CaptureArgs{}); done <- err }()
	<-tr.sent
	<-tr.receivers
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
	select {
	case <-tr.released:
	default:
		t.Fatal("listener not released")
	}
}

func TestCaptureDisplayByIdSyncLateResultDoesNotCompleteNextRequest(t *testing.T) {
	tr := &captureSyncTransport{receivers: make(chan binder.TransactionReceiver, 2), sent: make(chan struct{}, 2), released: make(chan struct{}, 2)}
	proxy := NewSurfaceComposerProxy(binder.NewProxyBinder(tr, binder.CallerIdentity{}, 1))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 2)
	go func() { _, err := proxy.CaptureDisplayByIdSync(ctx, 1, CaptureArgs{}); done <- err }()
	<-tr.sent
	first := <-tr.receivers
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	<-tr.released
	go func() {
		r, err := proxy.CaptureDisplayByIdSync(context.Background(), 1, CaptureArgs{})
		if r != nil {
			_ = r.Close()
		}
		done <- err
	}()
	<-tr.sent
	second := <-tr.receivers
	reply := func(status int32) *parcel.Parcel {
		p := parcel.New()
		p.WriteInterfaceToken(DescriptorIScreenCaptureListener)
		p.WriteInt32(1)
		p.WriteBool(false)
		p.WriteBool(false)
		p.WriteInt32(status)
		p.WriteBool(false)
		p.WriteBool(false)
		p.WriteUint32(0)
		p.SetPosition(0)
		return p
	}
	if _, err := first.OnTransaction(context.Background(), binder.FirstCallTransaction, reply(-1)); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		t.Fatalf("late callback completed next capture: %v", err)
	default:
	}
	if _, err := second.OnTransaction(context.Background(), binder.FirstCallTransaction, reply(0)); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		if !strings.Contains(err.Error(), "no buffer") {
			t.Fatal(err)
		}
	} else {
		t.Fatal("accepted successful capture without buffer")
	}
	<-tr.released
}
