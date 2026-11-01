package binder

import (
	"context"
	"errors"
	"testing"

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
