package testutil

import (
	"context"

	"github.com/xaionaro-go/aidl/binder"
	aidlerrors "github.com/xaionaro-go/aidl/errors"
	"github.com/xaionaro-go/aidl/parcel"
)

// MockBinder is a test double for binder.IBinder that returns a
// SecurityException reply from every Transact call. This allows
// testing generated proxy methods without a real binder driver.
type MockBinder struct{}

var _ binder.IBinder = (*MockBinder)(nil)

// NewMockBinder creates a new MockBinder.
func NewMockBinder() *MockBinder {
	return &MockBinder{}
}

// Transact returns a reply parcel containing a SecurityException status.
func (m *MockBinder) Transact(
	_ context.Context,
	_ binder.TransactionCode,
	_ binder.TransactionFlags,
	_ *parcel.Parcel,
) (*parcel.Parcel, error) {
	reply := parcel.New()
	binder.WriteStatus(reply, &aidlerrors.StatusError{
		Exception: aidlerrors.ExceptionSecurity,
		Message:   "mock: permission denied",
	})
	reply.SetPosition(0)
	return reply, nil
}

// LinkToDeath is a no-op for the mock.
func (m *MockBinder) LinkToDeath(
	_ context.Context,
	_ binder.DeathRecipient,
) error {
	return nil
}

// UnlinkToDeath is a no-op for the mock.
func (m *MockBinder) UnlinkToDeath(
	_ context.Context,
	_ binder.DeathRecipient,
) error {
	return nil
}

// IsAlive always returns true for the mock.
func (m *MockBinder) IsAlive(_ context.Context) bool {
	return true
}

// Handle returns a fixed handle value of 42.
func (m *MockBinder) Handle() uint32 {
	return 42
}

// Transport returns nil since there is no underlying transport.
func (m *MockBinder) Transport() binder.Transport {
	return nil
}
