package gui

import (
	"context"
	"errors"
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

// CheckSurfaceComposerAndroid14R35 accepts the simulated capture protocol.
//
// Example: submit a request on a verified test transport.
func (*captureSyncTransport) CheckSurfaceComposerAndroid14R35() error { return nil }

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
	// Run capture concurrently so cancellation can interrupt its wait.
	// Example: a canceled request unregisters its listener.
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
	// Submit the first request while the test controls its cancellation.
	// Example: cancel before a callback is delivered.
	go func() { _, err := proxy.CaptureDisplayByIdSync(ctx, 1, CaptureArgs{}); done <- err }()
	<-tr.sent
	first := <-tr.receivers
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
	<-tr.released
	// Submit a second request before the first callback arrives.
	// Example: a late first callback must not complete this request.
	go func() {
		r, err := proxy.CaptureDisplayByIdSync(context.Background(), 1, CaptureArgs{})
		if r != nil {
			err = errors.Join(err, r.Close())
		}
		done <- err
	}()
	<-tr.sent
	second := <-tr.receivers
	// Construct a native callback with a chosen capture status.
	// Example: encode a failure for the abandoned request.
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

// unsupportedCaptureTransport simulates a resolvable code without a verified contract.
//
// Example: a caller supplied an override but the capture wire format is unknown.
type unsupportedCaptureTransport struct{ *captureSyncTransport }

// CheckSurfaceComposerAndroid14R35 rejects the unknown capture contract.
//
// Example: report device-specific recognition evidence.
func (*unsupportedCaptureTransport) CheckSurfaceComposerAndroid14R35() error {
	return errors.New("unrecognized test device")
}

// Transact records an unexpected request instead of waiting for a callback.
//
// Example: ensure an unsupported profile never reaches Binder IPC.
func (t *unsupportedCaptureTransport) Transact(
	_ context.Context,
	_ uint32,
	_ binder.TransactionCode,
	_ binder.TransactionFlags,
	_ *parcel.Parcel,
) (*parcel.Parcel, error) {
	t.sent <- struct{}{}
	return nil, errors.New("unexpected transaction")
}

// TestCaptureDisplayByIdSyncRejectsUnverifiedContractBeforeIPC protects capture callers.
//
// Example: a manually resolved transaction code cannot bypass protocol checking.
func TestCaptureDisplayByIdSyncRejectsUnverifiedContractBeforeIPC(t *testing.T) {
	inner := &captureSyncTransport{receivers: make(chan binder.TransactionReceiver, 1), sent: make(chan struct{}, 1), released: make(chan struct{}, 1)}
	proxy := NewSurfaceComposerProxy(binder.NewProxyBinder(&unsupportedCaptureTransport{inner}, binder.CallerIdentity{}, 1))
	result, err := proxy.CaptureDisplayByIdSync(context.Background(), 1, CaptureArgs{})
	require.Nil(t, result)
	require.ErrorContains(t, err, "unrecognized test device")
	require.Empty(t, inner.receivers)
	require.Empty(t, inner.sent)
}
