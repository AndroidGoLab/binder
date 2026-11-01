package gui

import (
	"context"
	"testing"

	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/parcel"
	"github.com/stretchr/testify/require"
)

// captureSyncTransport simulates asynchronous capture callbacks.
//
// Example: deliver a late response after a request is canceled.
type captureSyncTransport struct {
	binder.VersionAwareTransport
	receivers chan binder.TransactionReceiver
	sent      chan struct{}
	released  chan struct{}
}

// RegisterReceiver records a callback receiver.
//
// Example: deliver a response to the recorded receiver.
func (t *captureSyncTransport) RegisterReceiver(
	_ context.Context,
	r binder.TransactionReceiver,
) uintptr {
	t.receivers <- r
	return 1
}

// UnregisterReceiver records callback cleanup.
//
// Example: observe listener removal after cancellation.
func (t *captureSyncTransport) UnregisterReceiver(
	_ context.Context,
	_ uintptr,
) error {
	t.released <- struct{}{}
	return nil
}

// CanUnregisterReceiver reports the test transport's cleanup capability.
//
// Example: permit a listener registration.
func (*captureSyncTransport) CanUnregisterReceiver() bool { return true }

// UsesSurfaceComposerAndroid14R35 reports the simulated capture protocol.
//
// Example: allow the synchronous helper to submit a capture.
func (*captureSyncTransport) UsesSurfaceComposerAndroid14R35() bool { return true }

// ResolveCode supplies the test transaction code.
//
// Example: resolve the capture method.
func (*captureSyncTransport) ResolveCode(
	_ context.Context,
	_ string,
	_ string,
) (binder.TransactionCode, error) {
	return 26, nil
}

// ResolveMethodSignature uses the generated proxy's signature.
//
// Example: marshal CaptureArgs without signature adaptation.
func (*captureSyncTransport) ResolveMethodSignature(
	_ context.Context,
	_ string,
	_ string,
) []string {
	return nil
}

// Transact signals that the request was submitted.
//
// Example: wait until a callback can be delivered.
func (t *captureSyncTransport) Transact(
	_ context.Context,
	_ uint32,
	_ binder.TransactionCode,
	_ binder.TransactionFlags,
	_ *parcel.Parcel,
) (*parcel.Parcel, error) {
	t.sent <- struct{}{}
	return parcel.New(), nil
}

// TestCaptureDisplayByIdSyncCancelReleasesListener verifies cancellation cleanup.
//
// Example: a canceled request removes its callback.
func TestCaptureDisplayByIdSyncCancelReleasesListener(t *testing.T) {
	tr := &captureSyncTransport{receivers: make(chan binder.TransactionReceiver, 1), sent: make(chan struct{}, 1), released: make(chan struct{}, 1)}
	proxy := NewSurfaceComposerProxy(binder.NewProxyBinder(tr, binder.CallerIdentity{}, 1))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := proxy.CaptureDisplayByIdSync(ctx, 1, CaptureArgs{}); done <- err }()
	<-tr.sent
	<-tr.receivers
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
	select {
	case <-tr.released:
	default:
		require.FailNow(t, "listener not released")
	}
}

// TestCaptureDisplayByIdSyncLateResultDoesNotCompleteNextRequest verifies isolation.
//
// Example: a late callback cannot finish a later request.
func TestCaptureDisplayByIdSyncLateResultDoesNotCompleteNextRequest(t *testing.T) {
	tr := &captureSyncTransport{receivers: make(chan binder.TransactionReceiver, 2), sent: make(chan struct{}, 2), released: make(chan struct{}, 2)}
	proxy := NewSurfaceComposerProxy(binder.NewProxyBinder(tr, binder.CallerIdentity{}, 1))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 2)
	go func() { _, err := proxy.CaptureDisplayByIdSync(ctx, 1, CaptureArgs{}); done <- err }()
	<-tr.sent
	first := <-tr.receivers
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
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
	_, err := first.OnTransaction(context.Background(), binder.FirstCallTransaction, reply(-1))
	require.NoError(t, err)
	select {
	case err := <-done:
		require.FailNowf(t, "late callback completed next capture", "%v", err)
	default:
	}
	_, err = second.OnTransaction(context.Background(), binder.FirstCallTransaction, reply(0))
	require.NoError(t, err)
	require.ErrorContains(t, <-done, "no buffer")
	<-tr.released
}
