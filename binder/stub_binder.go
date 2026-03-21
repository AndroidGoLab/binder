package binder

import (
	"context"
	"fmt"
	"sync"
	"unsafe"

	"github.com/facebookincubator/go-belt/tool/logger"
	"github.com/xaionaro-go/binder/parcel"
)

// StubBinder is a server-side IBinder that wraps a TransactionReceiver.
// When passed as a binder parameter in a proxy method, it is written
// as a local binder object (BINDER_TYPE_BINDER) instead of a handle
// reference (BINDER_TYPE_HANDLE).
//
// The cookie is assigned lazily the first time the stub is registered
// with a Transport (see RegisterWithTransport).
type StubBinder struct {
	Receiver  TransactionReceiver
	transport Transport

	mu     sync.Mutex
	cookie uintptr

	// weakRef is a heap-allocated anchor whose address is used as the
	// flat_binder_object.binder field (the kernel binder node identity).
	// In Android C++, this role is played by BBinder::getWeakRefs().
	// It must differ from cookie (which is the dispatch key) because
	// the kernel uses them independently. Keeping weakRef as a *uint64
	// prevents the GC from collecting it while the StubBinder is alive.
	weakRef *uint64
}

// NewStubBinder creates a StubBinder wrapping the given TransactionReceiver.
func NewStubBinder(
	receiver TransactionReceiver,
) *StubBinder {
	return &StubBinder{
		Receiver: receiver,
		weakRef:  new(uint64),
	}
}

// Transact is not supported on local stubs; calling it returns an error.
func (s *StubBinder) Transact(
	_ context.Context,
	_ TransactionCode,
	_ TransactionFlags,
	_ *parcel.Parcel,
) (*parcel.Parcel, error) {
	return nil, fmt.Errorf("StubBinder: Transact is not supported on local stubs")
}

// ResolveCode is not supported on local stubs.
func (s *StubBinder) ResolveCode(
	_ context.Context,
	_ string,
	_ string,
) (TransactionCode, error) {
	return 0, fmt.Errorf("StubBinder: ResolveCode is not supported on local stubs")
}

// LinkToDeath is a no-op for local stubs (they cannot die remotely).
func (s *StubBinder) LinkToDeath(
	_ context.Context,
	_ DeathRecipient,
) error {
	return nil
}

// UnlinkToDeath is a no-op for local stubs.
func (s *StubBinder) UnlinkToDeath(
	_ context.Context,
	_ DeathRecipient,
) error {
	return nil
}

// IsAlive always returns true for local stubs.
func (s *StubBinder) IsAlive(_ context.Context) bool {
	return true
}

// Handle returns 0 for local stubs. This is intentional: stubs are
// local objects and do not have a remote handle. Handle values are
// only meaningful for remote proxies (ProxyBinder). The value 0 is
// also used by ServiceManager's handle, but the two cannot be confused
// because a StubBinder is never used as a remote proxy.
func (s *StubBinder) Handle() uint32 {
	return 0
}

// Cookie returns the cookie assigned by RegisterWithTransport.
// Returns 0 if the stub has not been registered yet.
func (s *StubBinder) Cookie() uintptr {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cookie
}

// BinderPtr returns the stable address used as the binder node identity
// in the kernel (the flat_binder_object.binder field). This address is
// distinct from Cookie() -- the kernel uses BinderPtr to create/find the
// binder_node, and echoes Cookie back in BR_TRANSACTION for dispatch.
func (s *StubBinder) BinderPtr() uintptr {
	return uintptr(unsafe.Pointer(s.weakRef))
}

// Transport returns the VersionAwareTransport stored during
// RegisterWithTransport, or nil if the stub has not been registered
// or the stored transport does not implement VersionAwareTransport.
func (s *StubBinder) Transport() VersionAwareTransport {
	vat, _ := s.transport.(VersionAwareTransport)
	return vat
}

// Identity returns the default caller identity.
func (s *StubBinder) Identity() CallerIdentity {
	return DefaultCallerIdentity()
}

// RegisterWithTransport registers this stub's receiver with the given
// transport and stores the returned cookie. Subsequent calls are no-ops
// (the stub is only registered once). Returns the cookie.
func (s *StubBinder) RegisterWithTransport(
	ctx context.Context,
	t Transport,
) uintptr {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cookie != 0 {
		if s.transport != t {
			logger.Warnf(ctx, "StubBinder.RegisterWithTransport called with a different transport; ignoring (already registered)")
		}
		return s.cookie
	}

	s.cookie = t.RegisterReceiver(ctx, s.Receiver)
	s.transport = t
	return s.cookie
}

// Verify StubBinder implements IBinder.
var _ IBinder = (*StubBinder)(nil)
