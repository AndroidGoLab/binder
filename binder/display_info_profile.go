package binder

import "github.com/AndroidGoLab/binder/parcel"

// DisplayInfoWireProfileProvider identifies a peer's verified DisplayInfo layout.
type DisplayInfoWireProfileProvider interface {
	DisplayInfoWireProfile() parcel.DisplayInfoWireProfile
}

// DisplayInfoWireProfileFromBinder obtains a peer's profile, or unknown when unavailable.
func DisplayInfoWireProfileFromBinder(b IBinder) parcel.DisplayInfoWireProfile {
	if b == nil {
		return parcel.DisplayInfoWireProfileUnknown
	}
	if provider, ok := b.(DisplayInfoWireProfileProvider); ok {
		return provider.DisplayInfoWireProfile()
	}
	if provider, ok := b.Transport().(DisplayInfoWireProfileProvider); ok {
		return provider.DisplayInfoWireProfile()
	}
	return parcel.DisplayInfoWireProfileUnknown
}
