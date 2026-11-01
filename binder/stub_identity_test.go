package binder

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestStubNodeIdentitiesNeverReused prevents a late Binder node collision.
//
// Example: identities remain distinct after garbage collection.
func TestStubNodeIdentitiesNeverReused(t *testing.T) {
	seen := make(map[uintptr]bool)
	for i := 0; i < 10000; i++ {
		s := NewStubBinder(nil)
		id := s.BinderPtr()
		require.NotZero(t, id)
		require.False(t, seen[id], "reused node identity %x", id)
		seen[id] = true
		if i%100 == 0 {
			runtime.GC()
		}
	}
}
