package binder

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"

	"github.com/AndroidGoLab/binder/parcel"
	"github.com/facebookincubator/go-belt/tool/logger"
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

	mu              sync.Mutex
	cookie          uintptr
	released        bool
	releaseErr      error
	leases          int
	drained         chan struct{}
	unregistering   bool
	unregisterDone  chan struct{}
	releaseComplete bool

	// nodeID is an opaque, process-unique kernel Binder node identity.
	nodeID uintptr
}

// Release prevents new leases and waits for existing leases before unregistering.
// Cancellation leaves the registration closed to new leases and permits retry.
//
// Example: release a callback after its request finishes.
func (s *StubBinder) Release(ctx context.Context) error {
	for {
		s.mu.Lock()
		s.released = true
		if s.releaseComplete {
			err := s.releaseErr
			s.mu.Unlock()
			return err
		}
		if s.leases != 0 || s.unregistering {
			wait := s.drained
			if s.unregistering {
				wait = s.unregisterDone
			}
			s.mu.Unlock()
			select {
			case <-wait:
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		if s.cookie == 0 {
			s.releaseComplete = true
			s.mu.Unlock()
			return nil
		}
		s.unregistering = true
		s.unregisterDone = make(chan struct{})
		transport, cookie := s.transport, s.cookie
		s.mu.Unlock()
		var err error
		switch unregister := transport.(type) {
		case ReceiverUnregisterer:
			err = unregister.UnregisterReceiver(ctx, cookie)
		case ReceiverUnregistererNoError:
			unregister.UnregisterReceiver(ctx, cookie)
		default:
			err = fmt.Errorf("binder: transport cannot unregister receiver")
		}
		s.mu.Lock()
		s.unregistering = false
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			s.releaseComplete = true
			s.releaseErr = err
		}
		close(s.unregisterDone)
		s.mu.Unlock()
		return err
	}
}

// serializationLease pins a registration until its idempotent release.
//
// Example: a parcel retains the lease through a transaction.
type serializationLease struct {
	stub *StubBinder
	once sync.Once
}

// Release drops this lease's registration pin once.
//
// Example: recycling a parcel releases its local Binder pins.
func (l *serializationLease) Release() {
	// Drop the pin at most once.
	//
	// Example: repeated parcel cleanup cannot decrement the count twice.
	l.once.Do(func() {
		l.stub.mu.Lock()
		defer l.stub.mu.Unlock()
		l.stub.leases--
		if l.stub.leases == 0 {
			close(l.stub.drained)
		}
	})
}

// acquireSerializationLease pins a live registration on the requested transport.
//
// Example: prevent unregistering a stub while its parcel is in flight.
func (s *StubBinder) acquireSerializationLease(t Transport) (*serializationLease, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.released || s.cookie == 0 || !sameBinderTransport(s.transport, t) {
		return nil, fmt.Errorf("binder: local binder is not live on this transport")
	}
	if s.leases == 0 {
		s.drained = make(chan struct{})
	}
	s.leases++
	return &serializationLease{stub: s}, nil
}

// BinderTransportUnwrapper identifies wrappers that share their underlying Binder namespace.
//
// Example: a version-aware transport exposes the driver it wraps.
type BinderTransportUnwrapper interface {
	// UnderlyingBinderTransport returns the transport owning this namespace.
	//
	// Example: return the wrapped driver.
	UnderlyingBinderTransport() Transport
}

// sameBinderTransport compares the canonical owners of two Binder namespaces.
//
// Example: a driver and its version-aware wrapper share an identity.
func sameBinderTransport(a, b Transport) bool {
	// canonical resolves wrappers to a comparable namespace owner.
	//
	// Example: resolve a version-aware transport to its driver.
	canonical := func(t Transport) Transport {
		// Bound traversal also rejects malformed cyclic wrappers.
		for i := 0; i < 64; i++ {
			if t == nil {
				return nil
			}
			wrapper, ok := t.(BinderTransportUnwrapper)
			if !ok {
				if !reflect.ValueOf(t).Comparable() {
					return nil
				}
				return t
			}
			t = wrapper.UnderlyingBinderTransport()
		}
		return nil
	}
	a, b = canonical(a), canonical(b)
	return a != nil && b != nil && a == b
}

// nextLocalBinderIdentity assigns process-unique opaque Binder identities.
//
// Example: a new stub cannot reuse a released node's identity.
var nextLocalBinderIdentity atomic.Uintptr

// NewLocalBinderIdentity reserves a process-unique nonzero Binder identity.
//
// Example: reserve a receiver cookie that may also be used as a node key.
func NewLocalBinderIdentity() uintptr {
	for {
		old := nextLocalBinderIdentity.Load()
		if old == ^uintptr(0) {
			panic("binder: exhausted local binder node identities")
		}
		if nextLocalBinderIdentity.CompareAndSwap(old, old+1) {
			return old + 1
		}
	}
}

// NewStubBinder creates a StubBinder wrapping the given TransactionReceiver.
func NewStubBinder(
	receiver TransactionReceiver,
) *StubBinder {
	return &StubBinder{
		Receiver: receiver,
		nodeID:   NewLocalBinderIdentity(),
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

// BinderPtr returns the opaque identity used for the kernel Binder node.
//
// Example: write this identity into a local Binder object.
func (s *StubBinder) BinderPtr() uintptr {
	return s.nodeID
}

// Transport returns the VersionAwareTransport stored during
// RegisterWithTransport, or nil if the stub has not been registered
// or the stored transport does not implement VersionAwareTransport.
//
// Example: retrieve the transport associated with a registered stub.
func (s *StubBinder) Transport() VersionAwareTransport {
	s.mu.Lock()
	defer s.mu.Unlock()
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
//
// Example: register a callback before passing its stub to a proxy.
func (s *StubBinder) RegisterWithTransport(
	ctx context.Context,
	t Transport,
) uintptr {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.released {
		return s.cookie
	}
	if s.cookie != 0 {
		if !sameBinderTransport(s.transport, t) {
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
