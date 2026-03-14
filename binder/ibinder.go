package binder

import (
	"context"

	"github.com/xaionaro-go/aidl/parcel"
)

// IBinder is the high-level interface to a remote Binder object.
type IBinder interface {
	Transact(
		ctx context.Context,
		code TransactionCode,
		flags TransactionFlags,
		data *parcel.Parcel,
	) (_reply *parcel.Parcel, _err error)

	LinkToDeath(ctx context.Context, recipient DeathRecipient) (_err error)
	UnlinkToDeath(ctx context.Context, recipient DeathRecipient) (_err error)
	IsAlive(ctx context.Context) bool
	Handle() uint32
	Transport() Transport
}
