package versionaware

import (
	"context"
	"testing"

	"github.com/AndroidGoLab/binder/binder"
)

type unregisterTransport struct {
	binder.Transport
	cookie uintptr
}

func (t *unregisterTransport) UnregisterReceiver(_ context.Context, cookie uintptr) {
	t.cookie = cookie
}

func TestUnregisterReceiverForwards(t *testing.T) {
	inner := &unregisterTransport{}
	transport := &Transport{inner: inner}
	if err := transport.UnregisterReceiver(context.Background(), 42); err != nil || inner.cookie != 42 {
		t.Fatalf("cookie=%d error=%v", inner.cookie, err)
	}
}

func TestUnregisterReceiverRejectsUnsupportedTransport(t *testing.T) {
	transport := &Transport{inner: &captureTransport{}}
	if err := transport.UnregisterReceiver(context.Background(), 42); err == nil {
		t.Fatal("accepted unsupported transport")
	}
}

func TestCanUnregisterReceiverReflectsUnderlyingTransport(t *testing.T) {
	supported := &Transport{inner: &unregisterTransport{}}
	unsupported := &Transport{inner: &captureTransport{}}
	if !supported.CanUnregisterReceiver() || unsupported.CanUnregisterReceiver() {
		t.Fatal("incorrect unregister capability")
	}
}
