package servicemanager

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/AndroidGoLab/binder/binder"
	aidlerrors "github.com/AndroidGoLab/binder/errors"
	"github.com/AndroidGoLab/binder/parcel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockTransport captures the last Transact call and returns a predetermined reply.
//
// Example: inspect a service lookup without a Binder device.
type mockTransport struct {
	TransactionCount int
	lastHandle       uint32
	lastCode         binder.TransactionCode
	lastFlags        binder.TransactionFlags
	lastData         *parcel.Parcel
	replyFunc        func() *parcel.Parcel
	err              error
	lastReceiver     binder.TransactionReceiver
	nextCookie       uintptr
	resolvedMethod   string
	resolvedType     string
}

// Transact records the request and returns the configured reply or error.
//
// Example: count service manager lookup transactions.
func (m *mockTransport) Transact(
	_ context.Context,
	handle uint32,
	code binder.TransactionCode,
	flags binder.TransactionFlags,
	data *parcel.Parcel,
) (*parcel.Parcel, error) {
	m.TransactionCount++
	m.lastHandle = handle
	m.lastCode = code
	m.lastFlags = flags
	// Snapshot the data parcel so that tests can inspect it after the
	// caller recycles the original (defer data.Recycle()).
	snapshot := make([]byte, len(data.Data()))
	copy(snapshot, data.Data())
	m.lastData = parcel.FromBytes(snapshot)
	if m.err != nil {
		return nil, m.err
	}
	return m.replyFunc(), nil
}

func (m *mockTransport) AcquireHandle(_ context.Context, _ uint32) error { return nil }
func (m *mockTransport) ReleaseHandle(_ context.Context, _ uint32) error { return nil }
func (m *mockTransport) RegisterReceiver(_ context.Context, receiver binder.TransactionReceiver) uintptr {
	m.lastReceiver = receiver
	m.nextCookie++
	return m.nextCookie
}

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

// ResolveCode records modern method resolution and returns the test transaction.
//
// Example: verify that a modern lookup still resolves the getService method.
func (m *mockTransport) ResolveCode(
	_ context.Context,
	descriptor string,
	method string,
) (binder.TransactionCode, error) {
	m.resolvedType, m.resolvedMethod = descriptor, method
	return binder.FirstCallTransaction, nil
}

func (m *mockTransport) ResolveMethodSignature(_ context.Context, _ string, _ string) []string {
	return nil
}

func (m *mockTransport) APILevel() int {
	return 0
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
	result, err := sm.GetService(ctx, ServiceName("my.service"))
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify we sent to handle 0 (ServiceManager) with correct code.
	// ProxyBinder.Transact always ORs in FlagAcceptFDs.
	assert.Equal(t, uint32(0), mt.lastHandle)
	assert.Equal(t, binder.FirstCallTransaction, mt.lastCode)
	assert.Equal(t, serviceManagerDescriptor, mt.resolvedType)
	assert.Equal(t, "getService", mt.resolvedMethod)
	assert.Equal(t, binder.FlagAcceptFDs, mt.lastFlags)

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
	result, err := sm.CheckService(ctx, ServiceName("found.service"))
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, binder.FirstCallTransaction, mt.lastCode)
	assert.Equal(t, serviceManagerDescriptor, mt.resolvedType)
	assert.Equal(t, "checkService", mt.resolvedMethod)

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
	result, err := sm.CheckService(ctx, ServiceName("missing.service"))
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestListServices(t *testing.T) {
	ctx := context.Background()

	expected := []ServiceName{"service.a", "service.b", "service.c"}
	mt := &mockTransport{
		replyFunc: buildSuccessReply(func(p *parcel.Parcel) {
			p.WriteInt32(int32(len(expected)))
			for _, name := range expected {
				p.WriteString16(string(name))
			}
		}),
	}

	sm := New(mt)
	result, err := sm.ListServices(ctx)
	require.NoError(t, err)
	assert.Equal(t, expected, result)

	assert.Equal(t, binder.FirstCallTransaction, mt.lastCode)
	assert.Equal(t, serviceManagerDescriptor, mt.resolvedType)
	assert.Equal(t, "listServices", mt.resolvedMethod)

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

// TestListServicesModernPreservesNativeStatus keeps API 29 termination local.
//
// Example: a newer service manager's status -1 remains a transaction error.
func TestListServicesModernPreservesNativeStatus(t *testing.T) {
	expected := &aidlerrors.TransactionStatusError{Code: -1}
	mt := &mockTransport{err: expected}
	services, err := New(mt).ListServices(context.Background())
	require.ErrorIs(t, err, expected)
	assert.Nil(t, services)
	assert.Equal(t, 1, mt.TransactionCount)
	assert.Equal(t, "listServices", mt.resolvedMethod)
}

func TestIsDeclared_True(t *testing.T) {
	ctx := context.Background()

	mt := &mockTransport{
		replyFunc: buildSuccessReply(func(p *parcel.Parcel) {
			p.WriteBool(true)
		}),
	}

	sm := New(mt)
	result, err := sm.IsDeclared(ctx, ServiceName("declared.service"))
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
	result, err := sm.IsDeclared(ctx, ServiceName("undeclared.service"))
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
	result, err := sm.GetService(ctx, ServiceName("secure.service"))
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
	result, err := sm.GetService(ctx, ServiceName("any.service"))
	require.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, assert.AnError)
}

// mockReceiver is a minimal TransactionReceiver for testing.
type mockReceiver struct{}

func (r *mockReceiver) Descriptor() string {
	return "test.IMockReceiver"
}

func (r *mockReceiver) OnTransaction(
	_ context.Context,
	_ binder.TransactionCode,
	_ *parcel.Parcel,
) (*parcel.Parcel, error) {
	return parcel.New(), nil
}

func TestAddService(t *testing.T) {
	ctx := context.Background()

	mt := &mockTransport{
		replyFunc: buildSuccessReply(func(_ *parcel.Parcel) {}),
	}

	receiver := &mockReceiver{}
	sm := New(mt)
	err := sm.AddService(ctx, ServiceName("my.new.service"), receiver, true, 4)
	require.NoError(t, err)

	// Verify the receiver was registered.
	assert.Equal(t, binder.TransactionReceiver(receiver), mt.lastReceiver)

	// Verify we sent to handle 0 (ServiceManager) with correct code.
	// ProxyBinder.Transact always ORs in FlagAcceptFDs.
	assert.Equal(t, uint32(0), mt.lastHandle)
	assert.Equal(t, binder.FirstCallTransaction, mt.lastCode)
	assert.Equal(t, binder.FlagAcceptFDs, mt.lastFlags)

	// Verify the sent parcel data.
	mt.lastData.SetPosition(0)

	token, err := mt.lastData.ReadInterfaceToken()
	require.NoError(t, err)
	assert.Equal(t, serviceManagerDescriptor, token)

	svcName, err := mt.lastData.ReadString16()
	require.NoError(t, err)
	assert.Equal(t, "my.new.service", svcName)

	// Read the local binder object (flat_binder_object).
	// ReadStrongBinder returns the uint32 at offset 8, which for a
	// BINDER_TYPE_BINDER is the low 32 bits of the heap-allocated binder
	// pointer (not the cookie). We cannot predict the pointer value, so
	// just verify the read succeeds (the type is valid).
	_, err = mt.lastData.ReadStrongBinder()
	require.NoError(t, err)

	// allowIsolated = true -> 1
	allowIsolated, err := mt.lastData.ReadInt32()
	require.NoError(t, err)
	assert.Equal(t, int32(1), allowIsolated)

	// dumpPriority = 4
	dumpPriority, err := mt.lastData.ReadInt32()
	require.NoError(t, err)
	assert.Equal(t, int32(4), dumpPriority)
}

func TestAddService_NotIsolated(t *testing.T) {
	ctx := context.Background()

	mt := &mockTransport{
		replyFunc: buildSuccessReply(func(_ *parcel.Parcel) {}),
	}

	sm := New(mt)
	err := sm.AddService(ctx, ServiceName("isolated.service"), &mockReceiver{}, false, 0)
	require.NoError(t, err)

	// Verify allowIsolated is written as 0.
	mt.lastData.SetPosition(0)
	_, _ = mt.lastData.ReadInterfaceToken()
	_, _ = mt.lastData.ReadString16()
	_, _ = mt.lastData.ReadStrongBinder()

	allowIsolated, err := mt.lastData.ReadInt32()
	require.NoError(t, err)
	assert.Equal(t, int32(0), allowIsolated)
}

func TestAddService_StatusError(t *testing.T) {
	ctx := context.Background()

	mt := &mockTransport{
		replyFunc: func() *parcel.Parcel {
			p := parcel.New()
			binder.WriteStatus(p, &aidlerrors.StatusError{
				Exception: aidlerrors.ExceptionSecurity,
				Message:   "not allowed",
			})
			p.SetPosition(0)
			return p
		},
	}

	sm := New(mt)
	err := sm.AddService(ctx, ServiceName("blocked.service"), &mockReceiver{}, false, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not allowed")
}

func TestAddService_TransportError(t *testing.T) {
	ctx := context.Background()

	mt := &mockTransport{
		err: assert.AnError,
	}

	sm := New(mt)
	err := sm.AddService(ctx, ServiceName("fail.service"), &mockReceiver{}, false, 0)
	require.Error(t, err)
	assert.ErrorIs(t, err, assert.AnError)
}

// android10Transport supplies the Android 10 API level to the mock.
//
// Example: exercise the native service manager protocol.
type android10Transport struct{ *mockTransport }

// APILevel returns the Android 10 API level.
//
// Example: select the legacy service manager request.
func (m *android10Transport) APILevel() int { return binder.APILevelAndroid10 }

// ResolveCode rejects resolution because Android 10 uses a fixed code.
//
// Example: detect an unexpected AIDL code lookup in the test.
func (m *android10Transport) ResolveCode(
	_ context.Context,
	_ string,
	_ string,
) (binder.TransactionCode, error) {
	return 0, assert.AnError
}

// android10BinderReply returns a binder without a stability suffix.
//
// Example: simulate a successful native service lookup.
func android10BinderReply() *parcel.Parcel {
	p := parcel.New()
	p.WriteStrongBinder(42)
	return parcel.FromBytes(p.Data()[:24])
}

// android10MissingReply returns the native absent-service response.
//
// Example: simulate a service that has not been registered.
func android10MissingReply() *parcel.Parcel {
	p := parcel.New()
	p.WriteInt32(0)
	return p
}

// TestGetServiceAndroid10 checks the native lookup request and reply.
//
// Example: retrieve an Android 10 service handle.
func TestGetServiceAndroid10(t *testing.T) {
	mt := &android10Transport{&mockTransport{replyFunc: android10BinderReply}}
	result, err := New(mt).GetService(context.Background(), "activity")
	require.NoError(t, err)
	require.Equal(t, uint32(42), result.(*binder.ProxyBinder).Handle())
	assert.Equal(t, binder.FirstCallTransaction, mt.lastCode)
	mt.lastData.SetPosition(0)
	strict, err := mt.lastData.ReadInt32()
	require.NoError(t, err)
	assert.Equal(t, int32(-1<<31), strict)
	workSource, err := mt.lastData.ReadInt32()
	require.NoError(t, err)
	assert.Equal(t, int32(-1), workSource)
	descriptor, err := mt.lastData.ReadString16()
	require.NoError(t, err)
	assert.Equal(t, serviceManagerDescriptor, descriptor)
	name, err := mt.lastData.ReadString16()
	require.NoError(t, err)
	assert.Equal(t, "activity", name)
}

// TestGetServiceAndroid10NotFound checks the native null reply.
//
// Example: report a missing Android 10 service.
func TestGetServiceAndroid10NotFound(t *testing.T) {
	mt := &android10Transport{&mockTransport{replyFunc: android10MissingReply}}
	result, err := New(mt).GetService(context.Background(), "missing")
	require.Nil(t, result)
	require.ErrorContains(t, err, "service not found")
	assert.Equal(t, 1, mt.TransactionCount)
}

// TestCheckServiceAndroid10 verifies native lookup and the absence of AIDL resolution.
//
// Example: find SurfaceFlinger with transaction two and a legacy Binder reply.
func TestCheckServiceAndroid10(t *testing.T) {
	mt := &android10Transport{&mockTransport{replyFunc: android10BinderReply}}
	result, err := New(mt).CheckService(context.Background(), "SurfaceFlinger")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, uint32(42), result.(*binder.ProxyBinder).Handle())
	assert.Equal(t, binder.FirstCallTransaction+1, mt.lastCode)
	assert.Equal(t, uint32(0), mt.lastHandle)
	assert.Equal(t, binder.FlagAcceptFDs, mt.lastFlags)
	mt.lastData.SetPosition(0)
	strict, err := mt.lastData.ReadInt32()
	require.NoError(t, err)
	assert.Equal(t, int32(-1<<31), strict)
	workSource, err := mt.lastData.ReadInt32()
	require.NoError(t, err)
	assert.Equal(t, int32(-1), workSource)
	descriptor, err := mt.lastData.ReadString16()
	require.NoError(t, err)
	assert.Equal(t, serviceManagerDescriptor, descriptor)
	name, err := mt.lastData.ReadString16()
	require.NoError(t, err)
	assert.Equal(t, "SurfaceFlinger", name)
	assert.Equal(t, mt.lastData.Len(), mt.lastData.Position())
}

// TestCheckServiceAndroid10NotFound preserves the nonblocking absence result.
//
// Example: a missing service returns neither a Binder nor an error.
func TestCheckServiceAndroid10NotFound(t *testing.T) {
	mt := &android10Transport{&mockTransport{replyFunc: android10MissingReply}}
	result, err := New(mt).CheckService(context.Background(), "missing")
	require.NoError(t, err)
	assert.Nil(t, result)
	assert.Equal(t, 1, mt.TransactionCount)
}

// TestCheckServiceAndroid10TransportError preserves transport failures.
//
// Example: a failed Binder transaction must not look like an absent service.
func TestCheckServiceAndroid10TransportError(t *testing.T) {
	mt := &android10Transport{&mockTransport{err: assert.AnError}}
	result, err := New(mt).CheckService(context.Background(), "activity")
	require.ErrorIs(t, err, assert.AnError)
	assert.Nil(t, result)
	assert.Equal(t, 1, mt.TransactionCount)
}

// TestCheckServiceAndroid10Malformed rejects invalid native lookup responses.
//
// Example: a nonzero missing-service word or truncated Binder must remain an error.
func TestCheckServiceAndroid10Malformed(t *testing.T) {
	for _, payload := range [][]byte{{1, 0, 0, 0}, {0, 0}} {
		mt := &android10Transport{&mockTransport{
			replyFunc: // Return native bytes without a modern status envelope.
			//
			// Example: expose a truncated Binder object to the decoder.
			func() *parcel.Parcel { return parcel.FromBytes(payload) },
		}}
		result, err := New(mt).CheckService(context.Background(), "activity")
		require.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, 1, mt.TransactionCount)
	}
}

// android10ListTransport returns one native service name for each transaction.
//
// Example: reproduce indexed Android 10 enumeration without framework JARs.
type android10ListTransport struct {
	android10Transport
	names     []ServiceName
	requests  [][]byte
	endErr    error
	replyData []byte
}

// Transact records each request and returns a raw name or the configured error.
//
// Example: return TF_STATUS_CODE -1 after the final registered service.
func (m *android10ListTransport) Transact(
	_ context.Context,
	handle uint32,
	code binder.TransactionCode,
	flags binder.TransactionFlags,
	data *parcel.Parcel,
) (*parcel.Parcel, error) {
	m.TransactionCount++
	m.lastHandle, m.lastCode, m.lastFlags = handle, code, flags
	m.requests = append(m.requests, append([]byte(nil), data.Data()...))
	if m.TransactionCount > len(m.names) {
		return nil, m.endErr
	}
	if m.replyData != nil {
		return parcel.FromBytes(m.replyData), nil
	}
	p := parcel.New()
	p.WriteString16(string(m.names[m.TransactionCount-1]))
	return p, nil
}

// TestListServicesAndroid10 checks indexed requests and raw native responses.
//
// Example: list two services and stop only after the explicit terminal status.
func TestListServicesAndroid10(t *testing.T) {
	mt := &android10ListTransport{
		android10Transport: android10Transport{mockTransport: &mockTransport{}},
		names:              []ServiceName{"SurfaceFlinger", "activity"},
		endErr:             fmt.Errorf("wrapped: %w", &aidlerrors.TransactionStatusError{Code: -1}),
	}
	services, err := New(mt).ListServices(context.Background())
	require.NoError(t, err)
	assert.Equal(t, mt.names, services)
	assert.Equal(t, 3, mt.TransactionCount)
	assert.Equal(t, uint32(0), mt.lastHandle)
	assert.Equal(t, binder.TransactionCode(4), mt.lastCode)
	assert.Equal(t, binder.FlagAcceptFDs, mt.lastFlags)
	for index, request := range mt.requests {
		p := parcel.FromBytes(request)
		strict, err := p.ReadInt32()
		require.NoError(t, err)
		assert.Equal(t, int32(-1<<31), strict)
		workSource, err := p.ReadInt32()
		require.NoError(t, err)
		assert.Equal(t, int32(-1), workSource)
		descriptor, err := p.ReadString16()
		require.NoError(t, err)
		assert.Equal(t, serviceManagerDescriptor, descriptor)
		requestedIndex, err := p.ReadInt32()
		require.NoError(t, err)
		assert.Equal(t, int32(index), requestedIndex)
		priority, err := p.ReadInt32()
		require.NoError(t, err)
		assert.Equal(t, int32(15), priority)
		assert.Equal(t, p.Len(), p.Position())
	}
}

// TestListServicesAndroid10Empty checks the first enumeration probe's termination.
//
// Example: native status -1 before any name produces an empty list.
func TestListServicesAndroid10Empty(t *testing.T) {
	mt := &android10ListTransport{
		android10Transport: android10Transport{mockTransport: &mockTransport{}},
		endErr:             &aidlerrors.TransactionStatusError{Code: -1},
	}
	services, err := New(mt).ListServices(context.Background())
	require.NoError(t, err)
	assert.Empty(t, services)
	assert.Equal(t, 1, mt.TransactionCount)
}

// TestListServicesAndroid10PreservesFailures distinguishes exhaustion from failures.
//
// Example: a nonterminal status or unrelated error must not return a partial list.
func TestListServicesAndroid10PreservesFailures(t *testing.T) {
	for _, expected := range []error{
		assert.AnError,
		&aidlerrors.TransactionStatusError{Code: -13},
		&aidlerrors.TransactionError{Code: aidlerrors.TransactionErrorDeadObject},
		errors.New("binder: kernel status error: -1 (0xffffffff)"),
	} {
		mt := &android10ListTransport{
			android10Transport: android10Transport{mockTransport: &mockTransport{}},
			names:              []ServiceName{"activity"},
			endErr:             expected,
		}
		services, err := New(mt).ListServices(context.Background())
		require.ErrorIs(t, err, expected)
		assert.Nil(t, services)
		assert.Equal(t, 2, mt.TransactionCount)
	}
}

// TestListServicesAndroid10Malformed rejects a truncated native service name.
//
// Example: malformed replies must not become an empty successful enumeration.
func TestListServicesAndroid10Malformed(t *testing.T) {
	mt := &android10ListTransport{
		android10Transport: android10Transport{mockTransport: &mockTransport{}},
		names:              []ServiceName{"activity"},
		replyData:          []byte{1, 0},
	}
	services, err := New(mt).ListServices(context.Background())
	require.ErrorContains(t, err, "reading service 0")
	assert.Nil(t, services)
	assert.Equal(t, 1, mt.TransactionCount)
}

// TestListServicesAndroid10Canceled avoids another probe after cancellation.
//
// Example: a canceled discovery request returns its context error.
func TestListServicesAndroid10Canceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	mt := &android10ListTransport{android10Transport: android10Transport{mockTransport: &mockTransport{}}}
	services, err := New(mt).ListServices(ctx)
	require.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, services)
	assert.Zero(t, mt.TransactionCount)
}

// TestListServicesAndroid10Limit allows the terminal probe at the accepted limit.
//
// Example: exactly ten thousand services succeed; one additional name is rejected.
func TestListServicesAndroid10Limit(t *testing.T) {
	for _, count := range []int{int(maxServiceCount), int(maxServiceCount) + 1} {
		names := make([]ServiceName, count)
		for index := range names {
			names[index] = "activity"
		}
		mt := &android10ListTransport{
			android10Transport: android10Transport{mockTransport: &mockTransport{}},
			names:              names,
			endErr:             &aidlerrors.TransactionStatusError{Code: -1},
		}
		services, err := New(mt).ListServices(context.Background())
		if count == int(maxServiceCount) {
			require.NoError(t, err)
			assert.Len(t, services, count)
		} else {
			require.ErrorContains(t, err, "exceeds maximum")
			assert.Nil(t, services)
		}
		assert.Equal(t, int(maxServiceCount)+1, mt.TransactionCount)
	}
}
