//go:build linux

package kernelbinder

import (
	"context"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestReceiverCookiesNeverReused prevents dispatch to a later listener.
//
// Example: a removed callback's cookie is never assigned again.
func TestReceiverCookiesNeverReused(t *testing.T) {
	d := &Driver{receivers: make(map[uintptr]*receiverEntry)}
	d.readLoopOnce.Do(func() {})
	seen := make(map[uintptr]bool)
	for i := 0; i < 10000; i++ {
		cookie := d.RegisterReceiver(context.Background(), nil)
		require.NotZero(t, cookie)
		require.False(t, seen[cookie], "reused cookie %x", cookie)
		seen[cookie] = true
		d.UnregisterReceiver(context.Background(), cookie)
		require.Nil(t, d.lookupReceiver(cookie))
		if i%100 == 0 {
			runtime.GC()
		}
	}
}
