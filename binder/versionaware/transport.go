package versionaware

import (
	"context"

	"github.com/xaionaro-go/aidl/binder"
	"github.com/xaionaro-go/aidl/parcel"
)

// Transport wraps a binder.Transport and adds version-aware
// transaction code resolution via ResolveCode.
type Transport struct {
	inner    binder.Transport
	apiLevel int
	table    VersionTable
}

// DetectAPILevel returns the Android API level of the running device.
// Call this BEFORE opening /dev/binder, because the detection may fork
// a child process (getprop), and forking after mmap-ing the binder
// driver corrupts its state.
func DetectAPILevel() int {
	return detectAPILevel()
}

// NewTransport creates a version-aware Transport wrapping inner.
// If targetAPI > 0, uses that API level's table directly.
// If targetAPI == 0, auto-detects via DetectAPILevel().
//
// IMPORTANT: DetectAPILevel() may fork a child process (getprop).
// Forking after mmap-ing /dev/binder corrupts the driver state.
// Callers MUST either:
//   - Call DetectAPILevel() BEFORE opening the binder driver and pass the result, or
//   - Ensure the binder fd has not been opened yet when NewTransport is called with targetAPI=0.
func NewTransport(
	inner binder.Transport,
	targetAPI int,
) *Transport {
	if targetAPI <= 0 {
		targetAPI = detectAPILevel()
	}
	if targetAPI <= 0 {
		targetAPI = DefaultAPILevel
	}
	return &Transport{
		inner:    inner,
		apiLevel: targetAPI,
		table:    tableForAPI(targetAPI),
	}
}

// ResolveCode resolves an AIDL method name to the correct transaction code
// for the target device's API level.
func (t *Transport) ResolveCode(
	descriptor string,
	method string,
) binder.TransactionCode {
	return t.table.Resolve(descriptor, method)
}

// tableForAPI returns the VersionTable for the given API level.
// Falls back to the closest known API level.
func tableForAPI(apiLevel int) VersionTable {
	if table, ok := Tables[apiLevel]; ok {
		return table
	}
	// Fall back to default.
	if table, ok := Tables[DefaultAPILevel]; ok {
		return table
	}
	return nil
}

// DefaultAPILevel is the API level that the compiled proxy code was
// generated against. Set by generated code (codes_gen.go).
var DefaultAPILevel int

// Tables holds multi-version transaction code tables.
// Populated by generated code (codes_gen.go).
var Tables = MultiVersionTable{}

// --- Delegate all Transport methods to inner ---

func (t *Transport) Transact(
	ctx context.Context,
	handle uint32,
	code binder.TransactionCode,
	flags binder.TransactionFlags,
	data *parcel.Parcel,
) (*parcel.Parcel, error) {
	return t.inner.Transact(ctx, handle, code, flags, data)
}

func (t *Transport) AcquireHandle(
	ctx context.Context,
	handle uint32,
) error {
	return t.inner.AcquireHandle(ctx, handle)
}

func (t *Transport) ReleaseHandle(
	ctx context.Context,
	handle uint32,
) error {
	return t.inner.ReleaseHandle(ctx, handle)
}

func (t *Transport) RequestDeathNotification(
	ctx context.Context,
	handle uint32,
	recipient binder.DeathRecipient,
) error {
	return t.inner.RequestDeathNotification(ctx, handle, recipient)
}

func (t *Transport) ClearDeathNotification(
	ctx context.Context,
	handle uint32,
	recipient binder.DeathRecipient,
) error {
	return t.inner.ClearDeathNotification(ctx, handle, recipient)
}

func (t *Transport) Close(ctx context.Context) error {
	return t.inner.Close(ctx)
}

// Verify Transport implements binder.Transport and binder.CodeResolver.
var _ binder.Transport = (*Transport)(nil)
var _ binder.CodeResolver = (*Transport)(nil)
