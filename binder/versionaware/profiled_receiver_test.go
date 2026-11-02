package versionaware

import (
	"context"
	"testing"

	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/parcel"
)

type profileCaptureTransport struct {
	binder.Transport
	receiver binder.TransactionReceiver
}

func (t *profileCaptureTransport) RegisterReceiver(_ context.Context, receiver binder.TransactionReceiver) uintptr {
	t.receiver = receiver
	return 42
}

type profileCaptureReceiver struct {
	level   int
	context parcel.BinderSerializationContext
	profile parcel.DisplayInfoWireProfile
}

func (*profileCaptureReceiver) Descriptor() string { return "example.Callback" }
func (r *profileCaptureReceiver) OnTransaction(_ context.Context, _ binder.TransactionCode, data *parcel.Parcel) (*parcel.Parcel, error) {
	r.level = data.TargetAPILevel()
	r.context = data.BinderSerializationContext()
	r.profile = data.DisplayInfoWireProfile()
	data.SetTargetAPILevel(0)
	data.SetDisplayInfoWireProfile(parcel.DisplayInfoWireProfileUnknown)
	return parcel.New(), nil
}
func TestRegisterReceiverProfilesCallbacks(t *testing.T) {
	inner := &profileCaptureTransport{}
	tr := &Transport{inner: inner, apiLevel: 29}
	receiver := &profileCaptureReceiver{}
	if cookie := tr.RegisterReceiver(context.Background(), receiver); cookie != 42 {
		t.Fatalf("cookie = %d", cookie)
	}
	if got := inner.receiver.Descriptor(); got != receiver.Descriptor() {
		t.Fatalf("descriptor = %q", got)
	}
	data := parcel.New()
	data.SetDisplayInfoWireProfile(parcel.DisplayInfoWireProfileAndroid14Initial)
	reply, err := inner.receiver.OnTransaction(context.Background(), 1, data)
	if err != nil || receiver.level != 29 || receiver.context == nil || receiver.profile != parcel.DisplayInfoWireProfileAndroid14Initial || reply.TargetAPILevel() != 29 || reply.BinderSerializationContext() == nil || reply.DisplayInfoWireProfile() != parcel.DisplayInfoWireProfileAndroid14Initial {
		t.Fatalf("callback profile = %d, reply = %v, err = %v", receiver.level, reply.TargetAPILevel(), err)
	}
	other := &profileCaptureTransport{}
	(&Transport{inner: other, apiLevel: 34}).RegisterReceiver(context.Background(), receiver)
	reply, err = other.receiver.OnTransaction(context.Background(), 1, parcel.New())
	if err != nil || receiver.level != 34 || reply.TargetAPILevel() != 34 {
		t.Fatalf("second callback profile = %d, reply = %v, err = %v", receiver.level, reply.TargetAPILevel(), err)
	}
}
