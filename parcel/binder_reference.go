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

// BinderReference preserves Binder identity and its Android stability level.
// Local writes require a registered owner and a serialization context; the
// parcel retains its registration lease until Recycle. A zero stability on a
// new non-null reference selects system stability on API 34.
type BinderReference struct {
	Kind       BinderReferenceKind
	Handle     uint32
	Pointer    uintptr
	Cookie     uintptr
	Stability  StabilityLevel
	LocalOwner RegisteredLocalBinder
}

func (r BinderReference) validate(api int) error {
	if api == 29 && r.Stability != 0 {
		return fmt.Errorf("parcel: legacy Binder has stability")
	}
	switch r.Kind {
	case BinderReferenceNull:
		if r.Handle != 0 || r.Pointer != 0 || r.Cookie != 0 || r.Stability != 0 || r.LocalOwner != nil {
			return fmt.Errorf("parcel: null Binder has identity or stability")
		}
	case BinderReferenceRemote:
		if r.Pointer != 0 || r.Cookie != 0 || r.LocalOwner != nil {
			return fmt.Errorf("parcel: remote Binder has local identity")
		}
	case BinderReferenceLocal:
		if r.LocalOwner == nil || r.Handle != 0 {
			return fmt.Errorf("parcel: local Binder requires registered owner")
		}
	default:
		return fmt.Errorf("parcel: unknown Binder reference kind %d", r.Kind)
	}
	if r.Kind != BinderReferenceNull && r.Stability != 0 && r.Stability != 3 && r.Stability != 12 && r.Stability != 63 {
		return fmt.Errorf("parcel: invalid Binder stability %d", r.Stability)
	}
	return nil
}

// WriteBinderReference writes a strong Binder using the parcel's target API.
func (p *Parcel) WriteBinderReference(r BinderReference) error {
	api := p.TargetAPILevel()
	if api != 29 && api != 34 {
		return fmt.Errorf("parcel: unsupported Binder API level %d", api)
	}
	if err := r.validate(api); err != nil {
		return err
	}
	if r.Kind == BinderReferenceLocal {
		context := p.BinderSerializationContext()
		if context == nil {
			return fmt.Errorf("parcel: local Binder requires serialization context")
		}
		identity, lease, err := context.AcquireLocalBinder(r.LocalOwner)
		if err != nil {
			if lease != nil {
				lease.Release()
			}
			return fmt.Errorf("parcel: acquire local Binder: %w", err)
		}
		if lease == nil {
			return fmt.Errorf("parcel: local Binder acquisition returned nil lease")
		}
		if identity.Pointer == 0 || identity.Cookie == 0 || (r.Pointer != 0 && r.Pointer != identity.Pointer) || (r.Cookie != 0 && r.Cookie != identity.Cookie) {
			lease.Release()
			return fmt.Errorf("parcel: invalid local Binder identity")
		}
		r.Pointer = identity.Pointer
		r.Cookie = identity.Cookie
		p.RetainBinderLease(lease)
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
		if r.Stability != 0 {
			binary.LittleEndian.PutUint32(p.data[len(p.data)-4:], uint32(r.Stability))
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
	offset := p.Position()
	b, err := p.read(flatBinderObjectSize)
	if err != nil {
		return r, err
	}
	kind := binderObjectType(binary.LittleEndian.Uint32(b))
	value := binary.LittleEndian.Uint64(b[8:])
	cookie := binary.LittleEndian.Uint64(b[16:])
	if api == 34 {
		level, e := p.ReadInt32()
		if e != nil {
			return r, fmt.Errorf("parcel: reading binder stability: %w", e)
		}
		r.Stability = StabilityLevel(level)
	}
	switch kind {
	case binderTypeBinder:
		if value == 0 && cookie == 0 {
			if r.Stability != 0 {
				return BinderReference{}, fmt.Errorf("parcel: null Binder has stability %d", r.Stability)
			}
			return r, nil
		}
		if value == 0 || cookie == 0 || uint64(uintptr(value)) != value || uint64(uintptr(cookie)) != cookie {
			return BinderReference{}, fmt.Errorf("parcel: invalid local Binder identity")
		}
		r.Kind = BinderReferenceLocal
		r.Pointer = uintptr(value)
		r.Cookie = uintptr(cookie)
	case binderTypeHandle:
		if value>>32 != 0 || cookie != 0 {
			return BinderReference{}, fmt.Errorf("parcel: invalid remote Binder identity")
		}
		r.Kind = BinderReferenceRemote
		r.Handle = uint32(value)
	default:
		return BinderReference{}, fmt.Errorf("parcel: unexpected Binder type %#x", kind)
	}
	found := false
	for _, off := range p.objects {
		if off == uint64(offset) {
			found = true
			break
		}
	}
	if !found {
		return BinderReference{}, fmt.Errorf("parcel: Binder object at offset %d missing from objects", offset)
	}
	if api == 34 && r.Stability != 3 && r.Stability != 12 && r.Stability != 63 {
		return BinderReference{}, fmt.Errorf("parcel: invalid Binder stability %d", r.Stability)
	}
	return r, nil
}
