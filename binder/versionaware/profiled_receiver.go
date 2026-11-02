package versionaware

import (
	"context"

	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/parcel"
)

// profiledReceiver supplies the transport's API level to callbacks.
type profiledReceiver struct {
	receiver  binder.TransactionReceiver
	transport *Transport
}

// Descriptor forwards the callback's interface descriptor.
func (r *profiledReceiver) Descriptor() string { return r.receiver.Descriptor() }

// OnTransaction attaches the target profile to the request and reply.
func (r *profiledReceiver) OnTransaction(
	ctx context.Context,
	code binder.TransactionCode,
	data *parcel.Parcel,
) (*parcel.Parcel, error) {
	level := r.transport.APILevel()
	data.SetTargetAPILevel(level)
	reply, err := r.receiver.OnTransaction(ctx, code, data)
	if reply != nil {
		reply.SetTargetAPILevel(level)
	}
	return reply, err
}
