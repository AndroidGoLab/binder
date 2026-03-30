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

	// ResolveMethodSignature returns the parameter type descriptor
	// list (DEX format) for the given method on the target device.
	// Returns nil (not error) if the signature is unknown or cannot
	// be extracted. The proxy uses this to adapt parameter marshaling
	// when the device's method signature differs from the compiled one.
	ResolveMethodSignature(
		ctx context.Context,
		descriptor string,
		method string,
	) []string

	// APILevel returns the detected Android API level of the device
	// (e.g., 35 for Android 15, 36 for Android 16). Returns 0 if
	// the API level is unknown.
	APILevel() int
}
