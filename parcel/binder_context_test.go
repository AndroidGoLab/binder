package parcel

import (
	"errors"
	"testing"
)

type contextLifecycleLease struct{ releases int }

func (l *contextLifecycleLease) Release() { l.releases++ }

type contextLifecycleContext struct{}

func (*contextLifecycleContext) AcquireLocalBinder(RegisteredLocalBinder) (LocalBinderIdentity, BinderLease, error) {
	return LocalBinderIdentity{}, nil, errors.New("unused")
}

func TestBinderContextCloneLeaseLifecycle(t *testing.T) {
	p := New()
	context := &contextLifecycleContext{}
	p.SetBinderSerializationContext(context)
	lease := &contextLifecycleLease{}
	p.RetainBinderLease(lease)
	p.WriteInterfaceToken("test.Interface")
	clone := p.Android10Request()
	second := p.Android10Request()
	if clone == p || clone.BinderSerializationContext() != context {
		t.Fatal("clone must retain context")
	}
	if err := p.Recycle(); err != nil {
		t.Fatal(err)
	}
	if p.BinderSerializationContext() != nil || lease.releases != 0 {
		t.Fatal("source recycled while clone still owns lease")
	}
	if err := clone.Recycle(); err != nil {
		t.Fatal(err)
	}
	if err := clone.Recycle(); err != nil {
		t.Fatal(err)
	}
	if lease.releases != 0 {
		t.Fatal("second clone still owns lease")
	}
	if err := second.Recycle(); err != nil {
		t.Fatal(err)
	}
	if lease.releases != 1 {
		t.Fatalf("release count = %d, want 1", lease.releases)
	}
}
