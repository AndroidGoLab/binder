package binder

import "os"

// CallerIdentity holds the caller's identity, used to auto-fill
// identity parameters in AIDL proxy method calls.
type CallerIdentity struct {
	PackageName    string
	AttributionTag string
	UserID         int32
	PID            int32
	UID            int32
}

// DefaultCallerIdentity returns the identity for a shell-user process.
// The package name defaults to "com.android.shell"; callers should override
// PackageName for non-shell contexts (e.g., app or system service processes).
func DefaultCallerIdentity() CallerIdentity {
	return CallerIdentity{
		PackageName:    "com.android.shell",
		AttributionTag: "",
		UserID:         0,
		PID:            int32(os.Getpid()),
		UID:            int32(os.Getuid()),
	}
}
