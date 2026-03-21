package testutil

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/xaionaro-go/binder/binder"
	"github.com/xaionaro-go/binder/parcel"
)

// nilPanicker is a custom interface used to trigger a nil-dereference
// panic in PanicMethod. Unlike binder.IBinder, buildZeroArgs does not
// provide a mock for arbitrary interfaces, so it remains nil.
type nilPanicker interface {
	DoSomething()
}

// testProxy is a minimal proxy that exercises the smoke test helper.
// It mimics the pattern of generated AIDL proxies.
type testProxy struct {
	remote binder.IBinder
}

func (p *testProxy) AsBinder() binder.IBinder {
	return p.remote
}

// SimpleMethod writes an interface token, transacts, and reads status.
func (p *testProxy) SimpleMethod(
	ctx context.Context,
) (bool, error) {
	data := parcel.New()
	data.WriteInterfaceToken("test.ITestProxy")

	reply, err := p.remote.Transact(ctx, binder.FirstCallTransaction, 0, data)
	if err != nil {
		return false, err
	}
	defer reply.Recycle()

	if err = binder.ReadStatus(reply); err != nil {
		return false, err
	}

	return reply.ReadBool()
}

// MethodWithArgs writes arguments, transacts, and reads status.
func (p *testProxy) MethodWithArgs(
	ctx context.Context,
	value int32,
	name string,
) error {
	data := parcel.New()
	data.WriteInterfaceToken("test.ITestProxy")
	data.WriteInt32(value)
	data.WriteString16(name)

	reply, err := p.remote.Transact(ctx, binder.FirstCallTransaction+1, 0, data)
	if err != nil {
		return err
	}
	defer reply.Recycle()

	return binder.ReadStatus(reply)
}

// MethodWithIBinder takes a binder.IBinder arg, which buildZeroArgs
// now correctly fills with a MockBinder. This method should pass.
func (p *testProxy) MethodWithIBinder(
	ctx context.Context,
	b binder.IBinder,
) error {
	// With the fix, b is a *MockBinder, so Handle() works fine.
	_ = b.Handle()

	data := parcel.New()
	data.WriteInterfaceToken("test.ITestProxy")

	reply, err := p.remote.Transact(ctx, binder.FirstCallTransaction+2, 0, data)
	if err != nil {
		return err
	}
	defer reply.Recycle()

	return binder.ReadStatus(reply)
}

// PanicMethod takes a custom interface that buildZeroArgs passes as nil,
// triggering a nil-dereference panic.
func (p *testProxy) PanicMethod(
	_ context.Context,
	n nilPanicker,
) error {
	n.DoSomething()
	return nil
}

func TestSmokeTestAllMethods(t *testing.T) {
	mock := NewMockBinder()
	proxy := &testProxy{remote: mock}

	result := SmokeTestAllMethods(t, proxy)

	assert.Equal(t, 3, result.Total, "should test 3 methods (AsBinder skipped, PanicMethod skipped for unmockable interface)")
	assert.Equal(t, 0, result.Passed, "no methods should pass (mock returns SecurityException)")
	assert.Equal(t, 0, result.Panicked, "no panics expected (unmockable methods are skipped)")
	assert.Equal(t, 3, result.Failed, "SimpleMethod, MethodWithArgs, and MethodWithIBinder should fail (SecurityException)")
}

func TestSmokeTestAllMethods_EmptyProxy(t *testing.T) {
	type emptyProxy struct{}
	proxy := &emptyProxy{}

	result := SmokeTestAllMethods(t, proxy)

	assert.Equal(t, 0, result.Total)
	assert.Equal(t, 0, result.Passed)
	assert.Equal(t, 0, result.Panicked)
	assert.Equal(t, 0, result.Failed)
}

// testProxyWithDescriptor has both AsBinder and InterfaceDescriptor,
// verifying both are skipped.
type testProxyWithDescriptor struct {
	remote binder.IBinder
}

func (p *testProxyWithDescriptor) AsBinder() binder.IBinder {
	return p.remote
}

func (p *testProxyWithDescriptor) InterfaceDescriptor() string {
	return "test.IDescriptor"
}

func (p *testProxyWithDescriptor) DoWork(
	ctx context.Context,
) error {
	data := parcel.New()
	data.WriteInterfaceToken("test.IDescriptor")

	reply, err := p.remote.Transact(ctx, binder.FirstCallTransaction, 0, data)
	if err != nil {
		return err
	}
	defer reply.Recycle()

	return binder.ReadStatus(reply)
}

func TestSmokeTestAllMethods_SkipsBothInfrastructureMethods(t *testing.T) {
	mock := &MockBinder{}
	proxy := &testProxyWithDescriptor{remote: mock}

	result := SmokeTestAllMethods(t, proxy)

	assert.Equal(t, 1, result.Total, "only DoWork should be tested")
	assert.Equal(t, 0, result.Passed)
	assert.Equal(t, 0, result.Panicked)
	assert.Equal(t, 1, result.Failed, "DoWork should fail (mock returns SecurityException)")
}
