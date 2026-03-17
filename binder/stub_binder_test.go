package binder

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xaionaro-go/binder/parcel"
)

type testReceiver struct {
	lastCode TransactionCode
}

func (r *testReceiver) Descriptor() string {
	return "test.ITestReceiver"
}

func (r *testReceiver) OnTransaction(
	_ context.Context,
	code TransactionCode,
	_ *parcel.Parcel,
) (*parcel.Parcel, error) {
	r.lastCode = code
	return parcel.New(), nil
}

func TestStubBinder_CookieZeroBeforeRegistration(t *testing.T) {
	receiver := &testReceiver{}
	stub := NewStubBinder(receiver)
	assert.Equal(t, uintptr(0), stub.Cookie())
}

func TestStubBinder_Handle(t *testing.T) {
	receiver := &testReceiver{}
	stub := NewStubBinder(receiver)
	assert.Equal(t, uint32(0), stub.Handle())
}

func TestStubBinder_IsAlive(t *testing.T) {
	receiver := &testReceiver{}
	stub := NewStubBinder(receiver)
	assert.True(t, stub.IsAlive(context.Background()))
}

func TestStubBinder_TransactReturnsError(t *testing.T) {
	receiver := &testReceiver{}
	stub := NewStubBinder(receiver)
	_, err := stub.Transact(context.Background(), 1, 0, parcel.New())
	require.Error(t, err)
}

func TestStubBinder_ImplementsIBinder(t *testing.T) {
	receiver := &testReceiver{}
	stub := NewStubBinder(receiver)

	var _ IBinder = stub
	assert.NotNil(t, stub)
}

func TestWriteBinderToParcel_ProxyBinder(t *testing.T) {
	// For a ProxyBinder with a known handle, WriteBinderToParcel should
	// write a BINDER_TYPE_HANDLE object (same as WriteStrongBinder).
	proxy := &ProxyBinder{handle: 42}
	p := parcel.New()

	WriteBinderToParcel(context.Background(), p, proxy, nil)

	// The parcel should contain a flat_binder_object (24 bytes) + stability int32 (4 bytes).
	assert.Equal(t, 28, p.Len())
}
