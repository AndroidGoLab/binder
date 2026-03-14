package versionaware

import (
	"context"
	"sync"

	"github.com/xaionaro-go/aidl/binder"
	"github.com/xaionaro-go/aidl/parcel"
)

// Transport wraps a binder.Transport and adds version-aware
// transaction code resolution via ResolveCode.
type Transport struct {
	inner binder.Transport

	mu       sync.Mutex
	apiLevel int
	table    VersionTable
	detected bool
}

// NewTransport creates a version-aware Transport wrapping inner.
// If targetAPI > 0, uses that API level's table directly.
// If targetAPI == 0, detects the device's API level on first use.
func NewTransport(
	inner binder.Transport,
	targetAPI int,
) *Transport {
	t := &Transport{
		inner: inner,
	}
	if targetAPI > 0 {
		t.apiLevel = targetAPI
		t.table = tableForAPI(targetAPI)
		t.detected = true
	}
	return t
}

// ResolveCode resolves an AIDL method name to the correct transaction code
// for the target device's API level.
func (t *Transport) ResolveCode(
	descriptor string,
	method string,
) binder.TransactionCode {
	t.ensureDetected()
	return t.table.Resolve(descriptor, method)
}

func (t *Transport) ensureDetected() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.detected {
		return
	}
	t.detected = true
	// API level detection will be implemented in Phase 2.
	// For now, use the default (build-time) table.
	t.apiLevel = DefaultAPILevel
	t.table = tableForAPI(t.apiLevel)
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
