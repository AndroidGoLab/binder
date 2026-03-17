package binder

import (
	"context"

	"github.com/xaionaro-go/binder/parcel"
)

// IBinder is the high-level interface to a remote Binder object.
type IBinder interface {
	Transact(
		ctx context.Context,
		code TransactionCode,
		flags TransactionFlags,
		data *parcel.Parcel,
	) (_reply *parcel.Parcel, _err error)

	// ResolveCode maps an AIDL interface descriptor and method name
	// to the correct TransactionCode for the target device.
	// Returns an error if the method cannot be resolved (unsupported
	// device version or unknown method).
	ResolveCode(
		ctx context.Context,
		descriptor string,
		method string,
	) (TransactionCode, error)

	LinkToDeath(ctx context.Context, recipient DeathRecipient) (_err error)
	UnlinkToDeath(ctx context.Context, recipient DeathRecipient) (_err error)
	IsAlive(ctx context.Context) bool
	Handle() uint32
	Cookie() uintptr
	Transport() VersionAwareTransport
	Identity() CallerIdentity
}
