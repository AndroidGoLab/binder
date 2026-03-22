package binder

import (
	"context"

	"github.com/AndroidGoLab/binder/parcel"
)

// TransactionReceiver processes incoming binder transactions.
// Implemented by generated Stub types that dispatch transaction
// codes to typed Go interface methods.
type TransactionReceiver interface {
	// Descriptor returns the AIDL interface descriptor (e.g.
	// "android.frameworks.cameraservice.device.ICameraDeviceCallback").
	// The binder driver queries this via INTERFACE_TRANSACTION to
	// verify the remote binder's class.
	Descriptor() string

	OnTransaction(
		ctx context.Context,
		code TransactionCode,
		data *parcel.Parcel,
	) (*parcel.Parcel, error)
}
