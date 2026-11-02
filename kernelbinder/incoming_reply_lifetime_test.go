//go:build linux

package kernelbinder

import (
	"context"
	"errors"
	"testing"

	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/parcel"
	"github.com/stretchr/testify/require"
)

type replyLifetimeReceiver struct {
	reply *parcel.Parcel
	err   error
}

func (*replyLifetimeReceiver) Descriptor() string { return "test.reply" }
func (r *replyLifetimeReceiver) OnTransaction(context.Context, binder.TransactionCode, *parcel.Parcel) (*parcel.Parcel, error) {
	return r.reply, r.err
}

func TestIncomingTransactionRecyclesCallbackReply(t *testing.T) {
	for _, tc := range []struct {
		name  string
		flags binder.TransactionFlags
		err   error
	}{
		{name: "reply write failure"},
		{name: "callback error", err: errors.New("callback failed")},
		{name: "oneway", flags: binder.FlagOneway},
		{name: "oneway callback error", flags: binder.FlagOneway, err: errors.New("callback failed")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reply := parcel.New()
			reply.WriteInt32(42)
			receiver := &replyLifetimeReceiver{reply: reply, err: tc.err}
			d := &Driver{fd: -1, receivers: map[uintptr]*receiverEntry{1: {receiver: receiver}}}
			d.handleIncomingTransaction(context.Background(), &binderTransactionData{cookie: 1, code: uint32(binder.FirstCallTransaction), flags: uint32(tc.flags)})
			require.Empty(t, reply.Data(), "callback reply must be recycled")
		})
	}
}
