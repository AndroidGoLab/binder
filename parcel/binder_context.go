package parcel

import "sync"

// RegisteredLocalBinder identifies a local Binder registered with a transport.
type RegisteredLocalBinder interface {
	BinderPtr() uintptr
	Cookie() uintptr
}

// LocalBinderIdentity is the wire identity protected by a BinderLease.
type LocalBinderIdentity struct {
	Pointer uintptr
	Cookie  uintptr
}

// BinderLease keeps a local Binder registration alive until Release.
type BinderLease interface{ Release() }

// BinderSerializationContext acquires a local Binder for a target transport.
type BinderSerializationContext interface {
	AcquireLocalBinder(owner RegisteredLocalBinder) (LocalBinderIdentity, BinderLease, error)
}

type sharedBinderLease struct {
	mu    sync.Mutex
	refs  int
	lease BinderLease
}

func (l *sharedBinderLease) retain() {
	l.mu.Lock()
	l.refs++
	l.mu.Unlock()
}

func (l *sharedBinderLease) release() {
	l.mu.Lock()
	l.refs--
	if l.refs != 0 {
		l.mu.Unlock()
		return
	}
	lease := l.lease
	l.mu.Unlock()
	lease.Release()
}

// SetBinderSerializationContext associates local Binder writes with a transport.
func (p *Parcel) SetBinderSerializationContext(ctx BinderSerializationContext) { p.binderContext = ctx }

// BinderSerializationContext returns the context for local Binder writes.
func (p *Parcel) BinderSerializationContext() BinderSerializationContext { return p.binderContext }

// RetainBinderLease transfers a lease to this parcel until it is recycled.
func (p *Parcel) RetainBinderLease(lease BinderLease) {
	p.binderLeases = append(p.binderLeases, &sharedBinderLease{refs: 1, lease: lease})
}
