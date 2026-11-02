package parcel

import "testing"

func TestTargetAPILevelLifecycle(t *testing.T) {
	a, b := New(), New()
	if a.TargetAPILevel() != 0 || b.TargetAPILevel() != 0 {
		t.Fatal("new parcel must have unknown API")
	}
	a.SetTargetAPILevel(29)
	b.SetTargetAPILevel(36)
	a.WriteInterfaceToken("test.Interface")
	clone := a.Android10Request()
	if clone == a || clone.TargetAPILevel() != 29 || b.TargetAPILevel() != 36 {
		t.Fatal("target API was not preserved independently")
	}
	if err := a.Recycle(); err != nil {
		t.Fatal(err)
	}
	if a.TargetAPILevel() != 0 || clone.TargetAPILevel() != 29 {
		t.Fatal("recycle did not reset only the recycled parcel")
	}
}
