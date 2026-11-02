package parcel

import "testing"

func TestDisplayInfoWireProfileLifecycle(t *testing.T) {
	first, second := New(), New()
	if first.DisplayInfoWireProfile() != DisplayInfoWireProfileUnknown {
		t.Fatal("new parcel must have unknown profile")
	}
	first.SetDisplayInfoWireProfile(DisplayInfoWireProfileAndroid14Initial)
	second.SetDisplayInfoWireProfile(DisplayInfoWireProfileAndroid14Vsync)
	first.WriteInterfaceToken("test.Interface")
	clone := first.Android10Request()
	if clone == first || clone.DisplayInfoWireProfile() != DisplayInfoWireProfileAndroid14Initial || second.DisplayInfoWireProfile() != DisplayInfoWireProfileAndroid14Vsync {
		t.Fatal("profiles not preserved independently")
	}
	if err := first.Recycle(); err != nil {
		t.Fatal(err)
	}
	if first.DisplayInfoWireProfile() != DisplayInfoWireProfileUnknown || clone.DisplayInfoWireProfile() != DisplayInfoWireProfileAndroid14Initial {
		t.Fatal("recycle did not reset only the source")
	}
	if err := clone.Recycle(); err != nil {
		t.Fatal(err)
	}
}
