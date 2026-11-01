//go:build linux

package kernelbinder

import (
	"context"
	"runtime"
	"sync"
	"testing"

	"github.com/AndroidGoLab/binder/binder"
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

// TestConcurrentRawAndStubIdentitiesStayDistinct covers concurrent allocation.
//
// Example: local stubs and raw listeners register on the same driver.
func TestConcurrentRawAndStubIdentitiesStayDistinct(t *testing.T) {
	// workers is the number of concurrent identity producers.
	//
	// Example: multiple services register listeners simultaneously.
	const workers = 16
	// perWorker is the number of identity pairs per producer.
	//
	// Example: exercise repeated allocation under contention.
	const perWorker = 100
	d := &Driver{receivers: make(map[uintptr]*receiverEntry)}
	d.readLoopOnce.Do(func() {})
	identities := make(chan uintptr, workers*perWorker*2)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		// Allocate both identity kinds concurrently to exercise the shared source.
		//
		// Example: independent services register callbacks at the same time.
		go func() {
			defer wg.Done()
			for range perWorker {
				identities <- binder.NewStubBinder(nil).BinderPtr()
				cookie := d.RegisterReceiver(context.Background(), nil)
				identities <- cookie
				d.UnregisterReceiver(context.Background(), cookie)
			}
		}()
	}
	wg.Wait()
	close(identities)
	seen := make(map[uintptr]bool)
	for id := range identities {
		require.NotZero(t, id)
		require.False(t, seen[id], "reused local Binder identity %x", id)
		seen[id] = true
	}
}

// TestRawReceiverAndStubNodeIdentitiesDoNotCollide exercises both node users.
//
// Example: a raw HIDL listener and a local stub share one Binder driver.
func TestRawReceiverAndStubNodeIdentitiesDoNotCollide(t *testing.T) {
	d := &Driver{receivers: make(map[uintptr]*receiverEntry)}
	d.readLoopOnce.Do(func() {})
	seen := make(map[uintptr]bool)
	for i := 0; i < 1000; i++ {
		stub := binder.NewStubBinder(nil)
		node := stub.BinderPtr()
		require.NotZero(t, node)
		require.False(t, seen[node])
		seen[node] = true
		require.Equal(t, node, stub.BinderPtr())
		cookie := d.RegisterReceiver(context.Background(), nil)
		require.NotZero(t, cookie)
		require.False(t, seen[cookie], "raw cookie collides with a stub node")
		seen[cookie] = true
		d.UnregisterReceiver(context.Background(), cookie)
		if i%100 == 0 {
			runtime.GC()
		}
	}
}
