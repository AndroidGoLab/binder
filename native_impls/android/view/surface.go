package view

import (
	"fmt"

	"github.com/AndroidGoLab/binder/parcel"
)

const surfaceProducerKind uint32 = 0x62717565

// Surface is Android's native Surface parcelable. Local Binder references must
// remain registered with their transport for the lifetime of their use.
type Surface struct {
	Name             string
	IsSingleBuffered bool
	Producer         parcel.BinderReference
	Control          parcel.BinderReference
}

var _ parcel.Parcelable = (*Surface)(nil)

// MarshalParcel writes the API-specific native Surface representation.
// On error, p may contain partial data and Binder leases. Discard and recycle
// it before releasing local Binder owners.
func (s *Surface) MarshalParcel(p *parcel.Parcel) error {
	api := p.TargetAPILevel()
	if api != 29 && api != 34 {
		return fmt.Errorf("Surface: unsupported API level %d", api)
	}
	if api == 29 && (s.Control.Kind != parcel.BinderReferenceNull || s.Control.Handle != 0 || s.Control.Pointer != 0 || s.Control.Cookie != 0 || s.Control.Stability != 0 || s.Control.LocalOwner != nil) {
		return fmt.Errorf("Surface: control Binder is unsupported on API 29")
	}
	p.WriteString16(s.Name)
	p.WriteBool(s.IsSingleBuffered)
	p.WriteUint32(surfaceProducerKind)
	if err := p.WriteBinderReference(s.Producer); err != nil {
		return err
	}
	if api == 34 {
		return p.WriteBinderReference(s.Control)
	}
	return nil
}

// UnmarshalParcel reads the API-specific native Surface representation.
func (s *Surface) UnmarshalParcel(p *parcel.Parcel) error {
	api := p.TargetAPILevel()
	if api != 29 && api != 34 {
		return fmt.Errorf("Surface: unsupported API level %d", api)
	}
	var v Surface
	var err error
	if v.Name, err = p.ReadString16(); err != nil {
		return fmt.Errorf("Surface: name: %w", err)
	}
	if v.IsSingleBuffered, err = p.ReadBool(); err != nil {
		return fmt.Errorf("Surface: single buffered: %w", err)
	}
	kind, err := p.ReadUint32()
	if err != nil {
		return fmt.Errorf("Surface: producer kind: %w", err)
	}
	if kind != surfaceProducerKind {
		return fmt.Errorf("Surface: unknown producer kind %#x", kind)
	}
	if v.Producer, err = p.ReadBinderReference(); err != nil {
		return fmt.Errorf("Surface: producer: %w", err)
	}
	if api == 34 {
		if v.Control, err = p.ReadBinderReference(); err != nil {
			return fmt.Errorf("Surface: control: %w", err)
		}
	}
	*s = v
	return nil
}
