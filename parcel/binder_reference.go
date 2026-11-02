package parcel

import (
	"encoding/binary"
	"fmt"
)

// BinderReferenceKind identifies the representation of a strong Binder reference.
type BinderReferenceKind uint8

const (
	BinderReferenceNull BinderReferenceKind = iota
	BinderReferenceRemote
	BinderReferenceLocal
)

// BinderReference preserves a handle or a local Binder's pointer and dispatch cookie.
// Local references must already be registered with the transport. The caller owns
// that registration and must keep it alive while the reference is in use.
type BinderReference struct {
	Kind    BinderReferenceKind
	Handle  uint32
	Pointer uintptr
	Cookie  uintptr
}

func (r BinderReference) validate() error {
	switch r.Kind {
	case BinderReferenceNull:
		if r.Handle != 0 || r.Pointer != 0 || r.Cookie != 0 {
			return fmt.Errorf("parcel: null Binder has identity")
		}
	case BinderReferenceRemote:
		if r.Pointer != 0 || r.Cookie != 0 {
			return fmt.Errorf("parcel: remote Binder has local identity")
		}
	case BinderReferenceLocal:
		if r.Handle != 0 || r.Pointer == 0 || r.Cookie == 0 {
			return fmt.Errorf("parcel: local Binder requires pointer and cookie")
		}
	default:
		return fmt.Errorf("parcel: unknown Binder reference kind %d", r.Kind)
	}
	return nil
}

// WriteBinderReference writes a strong Binder using the parcel's target API.
func (p *Parcel) WriteBinderReference(r BinderReference) error {
	api := p.TargetAPILevel()
	if api != 29 && api != 34 {
		return fmt.Errorf("parcel: unsupported Binder API level %d", api)
	}
	if err := r.validate(); err != nil {
		return err
	}
	if api == 29 {
		switch r.Kind {
		case BinderReferenceNull:
			p.WriteNullStrongBinderLegacy()
		case BinderReferenceRemote:
			p.WriteStrongBinderLegacy(r.Handle)
		case BinderReferenceLocal:
			p.WriteLocalBinderLegacy(r.Pointer, r.Cookie)
		}
	} else {
		switch r.Kind {
		case BinderReferenceNull:
			p.WriteNullStrongBinder()
		case BinderReferenceRemote:
			p.WriteStrongBinder(r.Handle)
		case BinderReferenceLocal:
			p.WriteLocalBinder(r.Pointer, r.Cookie)
		}
	}
	return nil
}

// ReadBinderReference reads a strong Binder without truncating local identities.
func (p *Parcel) ReadBinderReference() (BinderReference, error) {
	var r BinderReference
	api := p.TargetAPILevel()
	if api != 29 && api != 34 {
		return r, fmt.Errorf("parcel: unsupported Binder API level %d", api)
	}
	b, err := p.read(flatBinderObjectSize)
	if err != nil {
		return r, err
	}
	kind := binderObjectType(binary.LittleEndian.Uint32(b))
	value := binary.LittleEndian.Uint64(b[8:])
	cookie := binary.LittleEndian.Uint64(b[16:])
	if api == 34 {
		if _, err = p.ReadInt32(); err != nil {
			return r, fmt.Errorf("parcel: reading binder stability: %w", err)
		}
	}
	switch kind {
	case binderTypeBinder:
		if value == 0 && cookie == 0 {
			return r, nil
		}
		if value == 0 || cookie == 0 || uint64(uintptr(value)) != value || uint64(uintptr(cookie)) != cookie {
			return r, fmt.Errorf("parcel: invalid local Binder identity")
		}
		r = BinderReference{Kind: BinderReferenceLocal, Pointer: uintptr(value), Cookie: uintptr(cookie)}
	case binderTypeHandle:
		if value>>32 != 0 || cookie != 0 {
			return r, fmt.Errorf("parcel: invalid remote Binder identity")
		}
		r = BinderReference{Kind: BinderReferenceRemote, Handle: uint32(value)}
	default:
		return r, fmt.Errorf("parcel: unexpected Binder type %#x", kind)
	}
	return r, nil
}
