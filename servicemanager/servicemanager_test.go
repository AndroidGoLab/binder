package servicemanager

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xaionaro-go/aidl/binder"
	aidlerrors "github.com/xaionaro-go/aidl/errors"
	"github.com/xaionaro-go/aidl/parcel"
)

// mockTransport captures the last Transact call and returns a predetermined reply.
type mockTransport struct {
	lastHandle uint32
	lastCode   binder.TransactionCode
	lastFlags  binder.TransactionFlags
	lastData   *parcel.Parcel
	replyFunc  func() *parcel.Parcel
	err        error
}

func (m *mockTransport) Transact(
	_ context.Context,
	handle uint32,
	code binder.TransactionCode,
	flags binder.TransactionFlags,
	data *parcel.Parcel,
) (*parcel.Parcel, error) {
	m.lastHandle = handle
	m.lastCode = code
	m.lastFlags = flags
	m.lastData = data
	if m.err != nil {
		return nil, m.err
	}
	return m.replyFunc(), nil
}

func (m *mockTransport) AcquireHandle(_ context.Context, _ uint32) error { return nil }
func (m *mockTransport) ReleaseHandle(_ context.Context, _ uint32) error { return nil }

func (m *mockTransport) RequestDeathNotification(
	_ context.Context,
	_ uint32,
	_ binder.DeathRecipient,
) error {
	return nil
}

func (m *mockTransport) ClearDeathNotification(
	_ context.Context,
	_ uint32,
	_ binder.DeathRecipient,
) error {
	return nil
}

func (m *mockTransport) Close(_ context.Context) error { return nil }

func (m *mockTransport) ResolveCode(_ string, _ string) (binder.TransactionCode, error) {
	return binder.FirstCallTransaction, nil
}

func buildSuccessReply(
	writePayload func(p *parcel.Parcel),
) func() *parcel.Parcel {
	return func() *parcel.Parcel {
		p := parcel.New()
		binder.WriteStatus(p, nil)
		writePayload(p)
		p.SetPosition(0)
		return p
	}
}

func TestGetService(t *testing.T) {
	ctx := context.Background()

	const expectedHandle = uint32(42)
	mt := &mockTransport{
		replyFunc: buildSuccessReply(func(p *parcel.Parcel) {
			p.WriteStrongBinder(expectedHandle)
		}),
	}

	sm := New(mt)
	result, err := sm.GetService(ctx, "my.service")
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify we sent to handle 0 (ServiceManager) with correct code.
	assert.Equal(t, uint32(0), mt.lastHandle)
	assert.Equal(t, binder.FirstCallTransaction, mt.lastCode)
	assert.Equal(t, binder.TransactionFlags(0), mt.lastFlags)

	// Verify the sent data has correct interface token and service name.
	mt.lastData.SetPosition(0)
	token, err := mt.lastData.ReadInterfaceToken()
	require.NoError(t, err)
	assert.Equal(t, serviceManagerDescriptor, token)

	svcName, err := mt.lastData.ReadString16()
	require.NoError(t, err)
	assert.Equal(t, "my.service", svcName)

	// Verify the returned binder wraps the expected handle.
	pb, ok := result.(*binder.ProxyBinder)
	require.True(t, ok)
	assert.Equal(t, expectedHandle, pb.Handle())
	assert.Equal(t, mt, pb.Transport())
}

func TestCheckService_Found(t *testing.T) {
	ctx := context.Background()

	const expectedHandle = uint32(7)
	mt := &mockTransport{
		replyFunc: buildSuccessReply(func(p *parcel.Parcel) {
			p.WriteStrongBinder(expectedHandle)
		}),
	}

	sm := New(mt)
	result, err := sm.CheckService(ctx, "found.service")
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, binder.FirstCallTransaction, mt.lastCode)

	pb, ok := result.(*binder.ProxyBinder)
	require.True(t, ok)
	assert.Equal(t, expectedHandle, pb.Handle())
}

func TestCheckService_NotFound(t *testing.T) {
	ctx := context.Background()

	mt := &mockTransport{
		replyFunc: buildSuccessReply(func(p *parcel.Parcel) {
			p.WriteNullStrongBinder()
		}),
	}

	sm := New(mt)
	result, err := sm.CheckService(ctx, "missing.service")
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestListServices(t *testing.T) {
	ctx := context.Background()

	expected := []string{"service.a", "service.b", "service.c"}
	mt := &mockTransport{
		replyFunc: buildSuccessReply(func(p *parcel.Parcel) {
			p.WriteInt32(int32(len(expected)))
			for _, name := range expected {
				p.WriteString16(name)
			}
		}),
	}

	sm := New(mt)
	result, err := sm.ListServices(ctx)
	require.NoError(t, err)
	assert.Equal(t, expected, result)

	assert.Equal(t, binder.FirstCallTransaction, mt.lastCode)

	// Verify the sent data contains dumpPriority=DUMP_FLAG_PRIORITY_ALL.
	mt.lastData.SetPosition(0)
	token, err := mt.lastData.ReadInterfaceToken()
	require.NoError(t, err)
	assert.Equal(t, serviceManagerDescriptor, token)

	priority, err := mt.lastData.ReadInt32()
	require.NoError(t, err)
	assert.Equal(t, dumpFlagPriorityAll, priority)
}

func TestListServices_Empty(t *testing.T) {
	ctx := context.Background()

	mt := &mockTransport{
		replyFunc: buildSuccessReply(func(p *parcel.Parcel) {
			p.WriteInt32(0)
		}),
	}

	sm := New(mt)
	result, err := sm.ListServices(ctx)
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestIsDeclared_True(t *testing.T) {
	ctx := context.Background()

	mt := &mockTransport{
		replyFunc: buildSuccessReply(func(p *parcel.Parcel) {
			p.WriteBool(true)
		}),
	}

	sm := New(mt)
	result, err := sm.IsDeclared(ctx, "declared.service")
	require.NoError(t, err)
	assert.True(t, result)

	assert.Equal(t, binder.FirstCallTransaction, mt.lastCode)
}

func TestIsDeclared_False(t *testing.T) {
	ctx := context.Background()

	mt := &mockTransport{
		replyFunc: buildSuccessReply(func(p *parcel.Parcel) {
			p.WriteBool(false)
		}),
	}

	sm := New(mt)
	result, err := sm.IsDeclared(ctx, "undeclared.service")
	require.NoError(t, err)
	assert.False(t, result)
}

func TestGetService_StatusError(t *testing.T) {
	ctx := context.Background()

	mt := &mockTransport{
		replyFunc: func() *parcel.Parcel {
			p := parcel.New()
			binder.WriteStatus(p, &aidlerrors.StatusError{
				Exception: aidlerrors.ExceptionSecurity,
				Message:   "permission denied",
			})
			p.SetPosition(0)
			return p
		},
	}

	sm := New(mt)
	result, err := sm.GetService(ctx, "secure.service")
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "permission denied")
}

func TestGetService_TransportError(t *testing.T) {
	ctx := context.Background()

	mt := &mockTransport{
		err: assert.AnError,
	}

	sm := New(mt)
	result, err := sm.GetService(ctx, "any.service")
	require.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, assert.AnError)
}
