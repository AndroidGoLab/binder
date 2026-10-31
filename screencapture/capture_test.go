package screencapture

import (
	"context"
	"testing"

	"github.com/AndroidGoLab/binder/android/gui"
	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/parcel"
	"github.com/stretchr/testify/require"
)

// TestPendingRequestBoundsCallbacks limits simultaneous capture requests.
//
// Example: a stalled service cannot accumulate unlimited callbacks.
func TestPendingRequestBoundsCallbacks(t *testing.T) {
	c := &Capture{pending: make(chan struct{}, 1)}
	require.NoError(t, c.begin())
	require.ErrorIs(t, c.begin(), ErrCapturePending)
	c.end()
	require.NoError(t, c.begin())
	c.end()
}

// testTransport captures the registered listener and simulates an asynchronous service.
//
// Example: deliver a late callback after the request context is canceled.
type testTransport struct {
	binder.VersionAwareTransport
	receivers    chan binder.TransactionReceiver
	nextCookie   uintptr
	sent         chan struct{}
	unregistered chan struct{}
}

// RegisterReceiver records the capture callback.
//
// Example: emulate registering a local listener.
func (t *testTransport) RegisterReceiver(_ context.Context, r binder.TransactionReceiver) uintptr {
	t.nextCookie++
	t.receivers <- r
	return t.nextCookie
}

// UnregisterReceiver reports callback cleanup.
//
// Example: callback completion releases its registration.
func (t *testTransport) UnregisterReceiver(_ context.Context, _ uintptr) error {
	t.unregistered <- struct{}{}
	return nil
}

// CanUnregisterReceiver reports the test transport's cleanup support.
//
// Example: allow a capture client to be created.
func (*testTransport) CanUnregisterReceiver() bool { return true }

// UsesSurfaceComposerAndroid14R35 reports the simulated wire protocol.
//
// Example: permit a request using the r35 capture signature.
func (*testTransport) UsesSurfaceComposerAndroid14R35() bool { return true }

// ResolveCode supplies the simulated capture transaction code.
//
// Example: resolve the generated proxy's capture method.
func (*testTransport) ResolveCode(context.Context, string, string) (binder.TransactionCode, error) {
	return 26, nil
}

// ResolveMethodSignature uses the proxy's compiled signature.
//
// Example: encode capture arguments using the current generated method.
func (*testTransport) ResolveMethodSignature(context.Context, string, string) []string { return nil }

// Transact accepts the request and lets the test deliver its response later.
//
// Example: emulate an asynchronous SurfaceFlinger request.
func (t *testTransport) Transact(context.Context, uint32, binder.TransactionCode, binder.TransactionFlags, *parcel.Parcel) (*parcel.Parcel, error) {
	t.sent <- struct{}{}
	return parcel.New(), nil
}

// TestCanceledRequestKeepsLateCallbackIsolated verifies bounded cancellation end to end through the client.
//
// Example: a late first result cannot become the second request's result.
func TestCanceledRequestKeepsLateCallbackIsolated(t *testing.T) {
	transport := &testTransport{sent: make(chan struct{}, 2), unregistered: make(chan struct{}, 2), receivers: make(chan binder.TransactionReceiver, 2)}
	remote := binder.NewProxyBinder(transport, binder.CallerIdentity{}, 1)
	capture, err := New(remote)
	require.NoError(t, err)
	defer capture.Close()
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go // Run the first capture until its context is canceled.
	//
	// Example: the test receives its cancellation error.
	func() { _, e := capture.CaptureDisplay(ctx, 1, 1); result <- e }()
	<-transport.sent
	first := <-transport.receivers
	cancel()
	require.ErrorIs(t, <-result, context.Canceled)
	<-transport.unregistered
	go // Start a second request before delivering the first callback.
	//
	// Example: the second request receives only its own result.
	func() { _, e := capture.CaptureDisplay(context.Background(), 1, 1); result <- e }()
	<-transport.sent
	second := <-transport.receivers
	p := parcel.New()
	p.WriteInterfaceToken(gui.DescriptorIScreenCaptureListener)
	p.WriteInt32(1)
	p.WriteBool(false)
	p.WriteBool(false)
	p.WriteInt32(-1)
	p.WriteBool(false)
	p.WriteBool(false)
	p.WriteUint32(0)
	_, err = first.OnTransaction(context.Background(), binder.FirstCallTransaction, p)
	require.NoError(t, err)
	select {
	case err = <-result:
		t.Fatalf("late result completed the next capture: %v", err)
	default:
	}
	p = parcel.New()
	p.WriteInterfaceToken(gui.DescriptorIScreenCaptureListener)
	p.WriteInt32(1)
	p.WriteBool(false)
	p.WriteBool(false)
	p.WriteInt32(-2)
	p.WriteBool(false)
	p.WriteBool(false)
	p.WriteUint32(0)
	_, err = second.OnTransaction(context.Background(), binder.FirstCallTransaction, p)
	require.NoError(t, err)
	require.ErrorContains(t, <-result, "-2")
	<-transport.unregistered
}

// unsupportedTransport represents a connection without the selected native protocol.
//
// Example: an unknown Android 14 revision is rejected before capture.
type unsupportedTransport struct{ *testTransport }

// UsesSurfaceComposerAndroid14R35 rejects the unknown protocol.
//
// Example: refuse to guess the capture method signature.
func (*unsupportedTransport) UsesSurfaceComposerAndroid14R35() bool { return false }

// TestNewRejectsUnknownProtocol prevents guessing a native wire signature.
//
// Example: an API 34 connection without an explicit r35 profile is rejected.
func TestNewRejectsUnknownProtocol(t *testing.T) {
	remote := binder.NewProxyBinder(&unsupportedTransport{testTransport: &testTransport{}}, binder.CallerIdentity{}, 1)
	_, err := New(remote)
	require.ErrorContains(t, err, "explicit")
}
