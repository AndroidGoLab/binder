package binder

import (
	"context"

	"github.com/facebookincubator/go-belt/tool/logger"
	"github.com/xaionaro-go/binder/parcel"
)

// ProxyBinder is a client-side handle to a remote Binder object.
// It delegates all operations to the underlying VersionAwareTransport.
type ProxyBinder struct {
	transport VersionAwareTransport
	identity  CallerIdentity
	handle    uint32
}

// NewProxyBinder creates a new ProxyBinder for the given transport, identity, and handle.
// The transport must implement VersionAwareTransport (use
// versionaware.NewTransport to wrap a raw kernelbinder.Driver).
func NewProxyBinder(
	transport VersionAwareTransport,
	identity CallerIdentity,
	handle uint32,
) *ProxyBinder {
	return &ProxyBinder{
		transport: transport,
		identity:  identity,
		handle:    handle,
	}
}

// Transact sends a transaction to the remote Binder object.
func (b *ProxyBinder) Transact(
	ctx context.Context,
	code TransactionCode,
	flags TransactionFlags,
	data *parcel.Parcel,
) (_reply *parcel.Parcel, _err error) {
	logger.Tracef(ctx, "Transact(handle=%d, code=%d, flags=%d)", b.handle, code, flags)
	defer func() { logger.Tracef(ctx, "/Transact: %v", _err) }()

	return b.transport.Transact(ctx, b.handle, code, flags, data)
}

// ResolveCode delegates to the underlying Transport.
func (b *ProxyBinder) ResolveCode(
	descriptor string,
	method string,
) (TransactionCode, error) {
	return b.transport.ResolveCode(descriptor, method)
}

// LinkToDeath registers a DeathRecipient for this Binder object.
func (b *ProxyBinder) LinkToDeath(
	ctx context.Context,
	recipient DeathRecipient,
) (_err error) {
	return b.transport.RequestDeathNotification(ctx, b.handle, recipient)
}

// UnlinkToDeath unregisters a DeathRecipient for this Binder object.
func (b *ProxyBinder) UnlinkToDeath(
	ctx context.Context,
	recipient DeathRecipient,
) (_err error) {
	return b.transport.ClearDeathNotification(ctx, b.handle, recipient)
}

// IsAlive checks whether the remote Binder object is still alive via a ping transaction.
func (b *ProxyBinder) IsAlive(ctx context.Context) bool {
	reply, err := b.transport.Transact(ctx, b.handle, PingTransaction, 0, parcel.New())
	if err != nil {
		return false
	}

	reply.Recycle()
	return true
}

// Handle returns the kernel-level handle for this Binder object.
func (b *ProxyBinder) Handle() uint32 {
	return b.handle
}

// Transport returns the underlying VersionAwareTransport used by this ProxyBinder.
func (b *ProxyBinder) Transport() VersionAwareTransport {
	return b.transport
}

// Identity returns the caller identity associated with this ProxyBinder.
func (b *ProxyBinder) Identity() CallerIdentity {
	return b.identity
}

// Verify ProxyBinder implements IBinder.
var _ IBinder = (*ProxyBinder)(nil)
