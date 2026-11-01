package versionaware

import (
	"context"
	"errors"
	"testing"

	"github.com/AndroidGoLab/binder/binder"
	"github.com/stretchr/testify/require"
)

// unregisterErrorTransport returns an error from receiver removal.
//
// Example: verify nested wrappers preserve the underlying failure.
type unregisterErrorTransport struct {
	binder.Transport
	err error
}

// UnregisterReceiver returns the configured failure.
//
// Example: simulate a failed receiver removal.
func (t *unregisterErrorTransport) UnregisterReceiver(
	context.Context,
	uintptr,
) error {
	return t.err
}

// unregisterDisabledTransport advertises that removal is unavailable.
//
// Example: ensure wrappers honor explicit capability denial.
type unregisterDisabledTransport struct{ unregisterTransport }

// CanUnregisterReceiver reports that receiver removal is unavailable.
//
// Example: prevent an unsupported cleanup operation.
func (*unregisterDisabledTransport) CanUnregisterReceiver() bool { return false }

// TestUnregisterReceiverPropagatesError checks nested cleanup failures.
//
// Example: return an underlying driver's removal error to the caller.
func TestUnregisterReceiverPropagatesError(t *testing.T) {
	want := errors.New("unregister failed")
	inner := &unregisterErrorTransport{err: want}
	outer := &Transport{inner: &Transport{inner: inner}}
	require.True(t, outer.CanUnregisterReceiver())
	require.ErrorIs(t, outer.UnregisterReceiver(context.Background(), 42), want)
}

// TestUnregisterReceiverHonorsDisabledCapability checks capability propagation.
//
// Example: do not call a driver that reports removal is unavailable.
func TestUnregisterReceiverHonorsDisabledCapability(t *testing.T) {
	inner := &unregisterDisabledTransport{}
	require.False(t, (&Transport{inner: inner}).CanUnregisterReceiver())
	outer := &Transport{inner: &Transport{inner: inner}}
	require.False(t, outer.CanUnregisterReceiver())
	require.Error(t, outer.UnregisterReceiver(context.Background(), 42))
	require.Zero(t, inner.cookie)
}

// unregisterTransport records the removed receiver cookie.
//
// Example: verify a wrapper forwards receiver cleanup.
type unregisterTransport struct {
	binder.Transport
	cookie uintptr
}

// unsupportedUnregisterTransport has no receiver removal capability.
//
// Example: check behavior with a transport without cleanup support.
type unsupportedUnregisterTransport struct{ binder.Transport }

// UnregisterReceiver records the removed receiver cookie.
//
// Example: verify forwarding to a void-returning transport.
func (t *unregisterTransport) UnregisterReceiver(
	_ context.Context,
	cookie uintptr,
) {
	t.cookie = cookie
}

// TestUnregisterReceiverForwards checks a void-returning transport.
//
// Example: remove a previously registered receiver.
func TestUnregisterReceiverForwards(t *testing.T) {
	inner := &unregisterTransport{}
	transport := &Transport{inner: inner}
	require.NoError(t, transport.UnregisterReceiver(context.Background(), 42))
	require.Equal(t, uintptr(42), inner.cookie)
}

// TestUnregisterReceiverRejectsUnsupportedTransport checks missing cleanup support.
//
// Example: reject receiver removal when the driver cannot perform it.
func TestUnregisterReceiverRejectsUnsupportedTransport(t *testing.T) {
	transport := &Transport{inner: &unsupportedUnregisterTransport{}}
	require.Error(t, transport.UnregisterReceiver(context.Background(), 42))
}

// TestCanUnregisterReceiverReflectsUnderlyingTransport checks cleanup support.
//
// Example: distinguish drivers that can remove receiver registrations.
func TestCanUnregisterReceiverReflectsUnderlyingTransport(t *testing.T) {
	supported := &Transport{inner: &unregisterTransport{}}
	unsupported := &Transport{inner: &unsupportedUnregisterTransport{}}
	require.True(t, supported.CanUnregisterReceiver())
	require.False(t, unsupported.CanUnregisterReceiver())
}
