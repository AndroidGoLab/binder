package binder

import (
	"context"

	"github.com/AndroidGoLab/binder/parcel"
)

// ReceiverUnregisterer removes a receiver and reports cleanup errors.
//
// Example: a transport wrapper forwards errors from its inner transport.
type ReceiverUnregisterer interface {
	// UnregisterReceiver removes a callback and returns any cleanup error.
	//
	// Example: unregister a completed asynchronous request.
	UnregisterReceiver(context.Context, uintptr) error
}

// ReceiverUnregistererNoError removes a receiver without returning an error.
//
// Example: the kernel Binder driver provides this form.
type ReceiverUnregistererNoError interface {
	// UnregisterReceiver removes a callback.
	//
	// Example: unregister a kernel Binder receiver.
	UnregisterReceiver(context.Context, uintptr)
}

// ReceiverUnregisterCapability reports whether receiver removal is supported.
//
// Example: a wrapper reflects an unsupported inner transport.
type ReceiverUnregisterCapability interface {
	// CanUnregisterReceiver reports whether callback removal is available.
	//
	// Example: reject registration when cleanup is unavailable.
	CanUnregisterReceiver() bool
}

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
