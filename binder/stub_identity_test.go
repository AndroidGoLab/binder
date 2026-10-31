package binder

import (
	"runtime"
	"testing"
)

func TestStubNodeIdentitiesNeverReused(t *testing.T) {
	seen := make(map[uintptr]bool)
	for i := 0; i < 10000; i++ {
		s := NewStubBinder(nil)
		id := s.BinderPtr()
		if id == 0 || seen[id] {
			t.Fatalf("reused node identity %x", id)
		}
		seen[id] = true
		if i%100 == 0 {
			runtime.GC()
		}
	}
}
