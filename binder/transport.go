package binder

import (
	"context"

	"github.com/xaionaro-go/binder/parcel"
)

// Transport is the low-level interface to the Binder kernel driver.
// It is implemented by kernelbinder.Driver.
type Transport interface {
	Transact(
		ctx context.Context,
		handle uint32,
		code TransactionCode,
		flags TransactionFlags,
		data *parcel.Parcel,
	) (_reply *parcel.Parcel, _err error)

	AcquireHandle(ctx context.Context, handle uint32) (_err error)
	ReleaseHandle(ctx context.Context, handle uint32) (_err error)

	RegisterReceiver(
		ctx context.Context,
		receiver TransactionReceiver,
	) uintptr

	RequestDeathNotification(
		ctx context.Context,
		handle uint32,
		recipient DeathRecipient,
	) (_err error)

	ClearDeathNotification(
		ctx context.Context,
		handle uint32,
		recipient DeathRecipient,
	) (_err error)

	Close(ctx context.Context) (_err error)
}
