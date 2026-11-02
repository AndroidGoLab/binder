package binder

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"testing"

	"github.com/AndroidGoLab/binder/parcel"
	"github.com/stretchr/testify/require"
)

// releaseTransport records receiver registration and release.
//
// Example: emulate a transport whose unregister operation fails.
type releaseTransport struct {
	Transport
	registrations, releases int
	releaseErr              error
}

// RegisterReceiver records one local receiver.
//
// Example: register a stub before releasing it.
func (t *releaseTransport) RegisterReceiver(
	_ context.Context,
	_ TransactionReceiver,
) uintptr {
	t.registrations++
	return 42
}

// UnregisterReceiver records a release and returns its configured error.
//
// Example: verify that repeated Release calls invoke this once.
func (t *releaseTransport) UnregisterReceiver(
	_ context.Context,
	_ uintptr,
) error {
	t.releases++
	return t.releaseErr
}

// CanUnregisterReceiver reports the test transport's capability.
//
// Example: allow the wrapped callback to be released.
func (*releaseTransport) CanUnregisterReceiver() bool { return true }

// TestStubReleaseIsTerminalAndIdempotent prevents a released node from being revived.
//
// Example: repeated release and registration do not touch the transport again.
func TestStubReleaseIsTerminalAndIdempotent(t *testing.T) {
	tr := &releaseTransport{}
	stub := NewStubBinder(&testReceiver{})
	cookie := stub.RegisterWithTransport(context.Background(), tr)
	require.NoError(t, stub.Release(context.Background()))
	require.NoError(t, stub.Release(context.Background()))
	require.Equal(t, cookie, stub.RegisterWithTransport(context.Background(), tr))
	require.Equal(t, 1, tr.registrations)
	require.Equal(t, 1, tr.releases)
}

// TestStubReleaseReportsUnregisterError retains the original cleanup error.
//
// Example: an unsuccessful release is reported on every call.
func TestStubReleaseReportsUnregisterError(t *testing.T) {
	want := errors.New("unregister failed")
	tr := &releaseTransport{releaseErr: want}
	stub := NewStubBinder(&testReceiver{})
	stub.RegisterWithTransport(context.Background(), tr)
	require.ErrorIs(t, stub.Release(context.Background()), want)
	require.ErrorIs(t, stub.Release(context.Background()), want)
	require.Equal(t, 1, tr.releases)
}

// voidReleaseTransport simulates the kernel driver's unregister signature.
//
// Example: release a receiver without an error return.
type voidReleaseTransport struct {
	Transport
	releases int
}

// RegisterReceiver returns a fixed test cookie.
//
// Example: register a local stub.
func (*voidReleaseTransport) RegisterReceiver(
	_ context.Context,
	_ TransactionReceiver,
) uintptr {
	return 42
}

// UnregisterReceiver counts releases.
//
// Example: verify that Release supports the driver signature.
func (t *voidReleaseTransport) UnregisterReceiver(
	_ context.Context,
	_ uintptr,
) {
	t.releases++
}

// TestStubReleaseSupportsDriverStyleUnregistration exercises the void signature.
//
// Example: the driver unregisters the receiver once.
func TestStubReleaseSupportsDriverStyleUnregistration(t *testing.T) {
	tr := &voidReleaseTransport{}
	stub := NewStubBinder(&testReceiver{})
	stub.RegisterWithTransport(context.Background(), tr)
	require.NoError(t, stub.Release(context.Background()))
	require.Equal(t, 1, tr.releases)
}

// TestStubReleaseWaitsForSerializationLeaseAndCanRetry verifies local registration lease behavior.
//
// Example: prevent unregistration while a request owns a lease.
func TestStubReleaseWaitsForSerializationLeaseAndCanRetry(t *testing.T) {
	tr := &releaseTransport{}
	stub := NewStubBinder(&testReceiver{})
	stub.RegisterWithTransport(context.Background(), tr)
	lease, err := stub.acquireSerializationLease(tr)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, stub.Release(ctx), context.Canceled)
	require.Zero(t, tr.releases)
	_, err = stub.acquireSerializationLease(tr)
	require.Error(t, err)
	lease.Release()
	lease.Release()
	require.NoError(t, stub.Release(context.Background()))
	require.Equal(t, 1, tr.releases)
}

// TestStubSerializationLeaseRejectsWrongTransport verifies local registration lease behavior.
//
// Example: prevent unregistration while a request owns a lease.
func TestStubSerializationLeaseRejectsWrongTransport(t *testing.T) {
	tr := &releaseTransport{}
	stub := NewStubBinder(&testReceiver{})
	stub.RegisterWithTransport(context.Background(), tr)
	_, err := stub.acquireSerializationLease(&releaseTransport{})
	require.Error(t, err)
	require.NoError(t, stub.Release(context.Background()))
	_, err = stub.acquireSerializationLease(tr)
	require.Error(t, err)
}

// TestBinderSerializationContextPinsRegisteredStub verifies local registration lease behavior.
//
// Example: prevent unregistration while a request owns a lease.
func TestBinderSerializationContextPinsRegisteredStub(t *testing.T) {
	tr := &releaseTransport{}
	stub := NewStubBinder(&testReceiver{})
	cookie := stub.RegisterWithTransport(context.Background(), tr)
	p := parcel.New()
	p.SetTargetAPILevel(34)
	p.SetBinderSerializationContext(NewBinderSerializationContext(tr))
	wrong := parcel.New()
	wrong.SetTargetAPILevel(34)
	wrong.SetBinderSerializationContext(NewBinderSerializationContext(&releaseTransport{}))
	require.Error(t, wrong.WriteBinderReference(parcel.BinderReference{Kind: parcel.BinderReferenceLocal, LocalOwner: stub}))
	require.Empty(t, wrong.Data())
	require.NoError(t, p.WriteBinderReference(parcel.BinderReference{Kind: parcel.BinderReferenceLocal, LocalOwner: stub}))
	require.NotEmpty(t, p.Objects())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, stub.Release(ctx), context.Canceled)
	require.Equal(t, cookie, stub.Cookie())
	require.Zero(t, tr.releases)
	require.NoError(t, p.Recycle())
	require.NoError(t, p.Recycle())
	require.NoError(t, stub.Release(context.Background()))
	require.Equal(t, 1, tr.releases)
	p.SetTargetAPILevel(34)
	p.SetBinderSerializationContext(NewBinderSerializationContext(tr))
	require.Error(t, p.WriteBinderReference(parcel.BinderReference{Kind: parcel.BinderReferenceLocal, LocalOwner: stub}))
}

// wrappedLeaseTransport provides a transport for lease tests.
//
// Example: exercise transport identity and release behavior.
type wrappedLeaseTransport struct{ Transport }

// UnderlyingBinderTransport returns the wrapped namespace owner.
//
// Example: compare a wrapper with its underlying driver.
func (w *wrappedLeaseTransport) UnderlyingBinderTransport() Transport { return w.Transport }

// nonComparableLeaseTransport provides a transport for lease tests.
//
// Example: exercise transport identity and release behavior.
type nonComparableLeaseTransport struct {
	Transport
	values []int
}

// nestedLeaseTransport provides a transport for lease tests.
//
// Example: exercise transport identity and release behavior.
type nestedLeaseTransport struct{ Transport }

// TestStubSerializationLeaseTransportNamespace verifies local registration lease behavior.
//
// Example: prevent unregistration while a request owns a lease.
func TestStubSerializationLeaseTransportNamespace(t *testing.T) {
	tr := &releaseTransport{}
	stub := NewStubBinder(&testReceiver{})
	stub.RegisterWithTransport(context.Background(), tr)
	lease, err := stub.acquireSerializationLease(&wrappedLeaseTransport{Transport: tr})
	require.NoError(t, err)
	lease.Release()
	_, err = stub.acquireSerializationLease(nonComparableLeaseTransport{Transport: tr})
	require.Error(t, err)
	_, err = stub.acquireSerializationLease(nestedLeaseTransport{Transport: nonComparableLeaseTransport{Transport: tr}})
	require.Error(t, err)
	require.NoError(t, stub.Release(context.Background()))
}

// TestStubReleaseWaitsForConcurrentLease verifies local registration lease behavior.
//
// Example: prevent unregistration while a request owns a lease.
func TestStubReleaseWaitsForConcurrentLease(t *testing.T) {
	tr := &releaseTransport{}
	stub := NewStubBinder(&testReceiver{})
	stub.RegisterWithTransport(context.Background(), tr)
	lease, err := stub.acquireSerializationLease(tr)
	require.NoError(t, err)
	done := make(chan error, 1)
	// Release concurrently so the test can hold the lease while it waits.
	//
	// Example: verify unregister does not finish until the pin is dropped.
	go func() { done <- stub.Release(context.Background()) }()
	for {
		stub.mu.Lock()
		closing := stub.released
		stub.mu.Unlock()
		if closing {
			break
		}
		runtime.Gosched()
	}
	_, err = stub.acquireSerializationLease(tr)
	require.Error(t, err)
	select {
	case err := <-done:
		t.Fatalf("release finished while leased: %v", err)
	default:
	}
	lease.Release()
	require.NoError(t, <-done)
	require.Equal(t, 1, tr.releases)
}

// blockingReleaseTransport provides a transport for lease tests.
//
// Example: exercise transport identity and release behavior.
type blockingReleaseTransport struct {
	Transport
	entered chan struct{}
	resume  chan struct{}
	err     error
}

// RegisterReceiver supplies a test registration.
//
// Example: register a stub before concurrent release.
func (t *blockingReleaseTransport) RegisterReceiver(context.Context, TransactionReceiver) uintptr {
	return 42
}

// UnregisterReceiver blocks until the test permits completion.
//
// Example: keep one release in progress while another starts.
func (t *blockingReleaseTransport) UnregisterReceiver(context.Context, uintptr) error {
	close(t.entered)
	<-t.resume
	return t.err
}

// TestConcurrentStubReleaseSharesResult verifies local registration lease behavior.
//
// Example: prevent unregistration while a request owns a lease.
func TestConcurrentStubReleaseSharesResult(t *testing.T) {
	want := errors.New("unregister failed")
	tr := &blockingReleaseTransport{entered: make(chan struct{}), resume: make(chan struct{}), err: want}
	stub := NewStubBinder(&testReceiver{})
	stub.RegisterWithTransport(context.Background(), tr)
	results := make(chan error, 2)
	// Start the first release and hold its unregister call open.
	//
	// Example: establish an unregister owner before another caller arrives.
	go func() { results <- stub.Release(context.Background()) }()
	<-tr.entered
	// Start a second release while the first owns unregistration.
	//
	// Example: both callers observe the same cleanup result.
	go func() { results <- stub.Release(context.Background()) }()
	close(tr.resume)
	require.ErrorIs(t, <-results, want)
	require.ErrorIs(t, <-results, want)
}

// TestStubReleaseRetriesCanceledUnregister verifies local registration lease behavior.
//
// Example: prevent unregistration while a request owns a lease.
func TestStubReleaseRetriesCanceledUnregister(t *testing.T) {
	tr := &releaseTransport{releaseErr: fmt.Errorf("interrupted: %w", context.Canceled)}
	stub := NewStubBinder(&testReceiver{})
	stub.RegisterWithTransport(context.Background(), tr)
	require.ErrorIs(t, stub.Release(context.Background()), context.Canceled)
	tr.releaseErr = nil
	require.NoError(t, stub.Release(context.Background()))
	require.Equal(t, 2, tr.releases)
}
