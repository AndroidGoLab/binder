//go:build linux

package kernelbinder

import (
	"context"
	"runtime"
	"testing"
)

func TestReceiverCookiesNeverReused(t *testing.T) {
	d := &Driver{receivers: make(map[uintptr]*receiverEntry)}
	d.readLoopOnce.Do(func() {})
	seen := make(map[uintptr]bool)
	for i := 0; i < 10000; i++ {
		cookie := d.RegisterReceiver(context.Background(), nil)
		if cookie == 0 || seen[cookie] {
			t.Fatalf("reused cookie %x", cookie)
		}
		seen[cookie] = true
		d.UnregisterReceiver(context.Background(), cookie)
		if d.lookupReceiver(cookie) != nil {
			t.Fatal("unregistered receiver remains live")
		}
		if i%100 == 0 {
			runtime.GC()
		}
	}
}
