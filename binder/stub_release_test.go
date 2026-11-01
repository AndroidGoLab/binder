package binder

import (
	"context"
	"errors"
	"testing"
)

type releaseTransport struct {
	Transport
	registrations, releases int
	releaseErr              error
}

func (t *releaseTransport) RegisterReceiver(context.Context, TransactionReceiver) uintptr {
	t.registrations++
	return 42
}
func (t *releaseTransport) UnregisterReceiver(context.Context, uintptr) error {
	t.releases++
	return t.releaseErr
}
func (t *releaseTransport) CanUnregisterReceiver() bool { return true }

func TestStubReleaseIsTerminalAndIdempotent(t *testing.T) {
	tr := &releaseTransport{}
	stub := NewStubBinder(&testReceiver{})
	cookie := stub.RegisterWithTransport(context.Background(), tr)
	if err := stub.Release(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := stub.Release(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := stub.RegisterWithTransport(context.Background(), tr); got != cookie || tr.registrations != 1 || tr.releases != 1 {
		t.Fatalf("cookie=%d registrations=%d releases=%d", got, tr.registrations, tr.releases)
	}
}
func TestStubReleaseReportsUnregisterError(t *testing.T) {
	want := errors.New("unregister failed")
	tr := &releaseTransport{releaseErr: want}
	stub := NewStubBinder(&testReceiver{})
	stub.RegisterWithTransport(context.Background(), tr)
	if err := stub.Release(context.Background()); !errors.Is(err, want) {
		t.Fatalf("got %v", err)
	}
	if err := stub.Release(context.Background()); !errors.Is(err, want) || tr.releases != 1 {
		t.Fatalf("second release: %v, calls %d", err, tr.releases)
	}
}

type voidReleaseTransport struct {
	Transport
	releases int
}

func (*voidReleaseTransport) RegisterReceiver(context.Context, TransactionReceiver) uintptr {
	return 42
}
func (t *voidReleaseTransport) UnregisterReceiver(context.Context, uintptr) { t.releases++ }
func TestStubReleaseSupportsDriverStyleUnregistration(t *testing.T) {
	tr := &voidReleaseTransport{}
	stub := NewStubBinder(&testReceiver{})
	stub.RegisterWithTransport(context.Background(), tr)
	if err := stub.Release(context.Background()); err != nil || tr.releases != 1 {
		t.Fatalf("error %v, releases %d", err, tr.releases)
	}
}
