package binder

import (
	"testing"

	"github.com/AndroidGoLab/binder/parcel"
)

type profileTestBinder struct {
	IBinder
	transport VersionAwareTransport
}

func (b *profileTestBinder) Transport() VersionAwareTransport { return b.transport }

type profileProviderBinder struct {
	profileTestBinder
	profile parcel.DisplayInfoWireProfile
}

func (b *profileProviderBinder) DisplayInfoWireProfile() parcel.DisplayInfoWireProfile {
	return b.profile
}

type profileProviderTransport struct {
	VersionAwareTransport
	profile parcel.DisplayInfoWireProfile
}

func (t *profileProviderTransport) DisplayInfoWireProfile() parcel.DisplayInfoWireProfile {
	return t.profile
}
func TestDisplayInfoWireProfileFromBinder(t *testing.T) {
	if got := DisplayInfoWireProfileFromBinder(nil); got != parcel.DisplayInfoWireProfileUnknown {
		t.Fatalf("nil = %v", got)
	}
	transport := &profileProviderTransport{profile: parcel.DisplayInfoWireProfileAndroid14Vsync}
	b := &profileProviderBinder{profileTestBinder: profileTestBinder{transport: transport}, profile: parcel.DisplayInfoWireProfileAndroid14Initial}
	if got := DisplayInfoWireProfileFromBinder(b); got != b.profile {
		t.Fatalf("binder profile = %v", got)
	}
	b.profile = parcel.DisplayInfoWireProfileUnknown
	if got := DisplayInfoWireProfileFromBinder(b); got != parcel.DisplayInfoWireProfileUnknown {
		t.Fatalf("binder unknown must remain authoritative, got %v", got)
	}
	if got := DisplayInfoWireProfileFromBinder(&profileTestBinder{transport: transport}); got != transport.profile {
		t.Fatalf("transport profile = %v", got)
	}
	if got := DisplayInfoWireProfileFromBinder(&profileTestBinder{}); got != parcel.DisplayInfoWireProfileUnknown {
		t.Fatalf("missing profile = %v", got)
	}
}
