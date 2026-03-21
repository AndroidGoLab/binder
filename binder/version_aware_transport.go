package binder

import "context"

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
