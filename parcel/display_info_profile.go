package parcel

// DisplayInfoWireProfile identifies the platform-specific DisplayInfo layout.
type DisplayInfoWireProfile uint8

const (
	// DisplayInfoWireProfileUnknown means the remote layout has not been identified.
	DisplayInfoWireProfileUnknown DisplayInfoWireProfile = iota
	// DisplayInfoWireProfileAndroid10 identifies the Android 10 layout.
	DisplayInfoWireProfileAndroid10
	// DisplayInfoWireProfileAndroid14Initial identifies the initial Android 14 layout.
	DisplayInfoWireProfileAndroid14Initial
	// DisplayInfoWireProfileAndroid14Vsync identifies the Android 14 layout with VSync fields.
	DisplayInfoWireProfileAndroid14Vsync
)

// SetDisplayInfoWireProfile sets the remote DisplayInfo layout.
func (p *Parcel) SetDisplayInfoWireProfile(profile DisplayInfoWireProfile) {
	p.displayInfoWireProfile = profile
}

// DisplayInfoWireProfile returns the remote DisplayInfo layout, or unknown.
func (p *Parcel) DisplayInfoWireProfile() DisplayInfoWireProfile { return p.displayInfoWireProfile }
