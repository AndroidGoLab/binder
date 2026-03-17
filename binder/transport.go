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

// VersionAwareTransport extends Transport with version-aware
// transaction code resolution. Implemented by versionaware.Transport.
type VersionAwareTransport interface {
	Transport

	// ResolveCode maps an AIDL interface descriptor and method name
	// to the correct TransactionCode for the target device.
	// Returns an error if the method does not exist on the device's
	// API level/revision.
	ResolveCode(
		ctx context.Context,
		descriptor string,
		method string,
	) (TransactionCode, error)
}
