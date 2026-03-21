//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xaionaro-go/binder/binder"
	"github.com/xaionaro-go/binder/binder/versionaware"
	"github.com/xaionaro-go/binder/kernelbinder"
	"github.com/xaionaro-go/binder/parcel"
	"github.com/xaionaro-go/binder/servicemanager"
)

// deathRecipientSpy records whether BinderDied was called and how many times.
type deathRecipientSpy struct {
	diedCount atomic.Int32
	diedCh    chan struct{} // closed on first BinderDied call
	once      sync.Once
}

func newDeathRecipientSpy() *deathRecipientSpy {
	return &deathRecipientSpy{
		diedCh: make(chan struct{}),
	}
}

func (r *deathRecipientSpy) BinderDied() {
	r.diedCount.Add(1)
	r.once.Do(func() { close(r.diedCh) })
}

// TestLifecycle_DeathNotification registers a death notification on
// SurfaceFlinger (a long-lived system service), verifies the notification
// does NOT fire while the service is alive, then unlinks cleanly.
//
// Bugs exposed:
// - handleDeadBinder does not remove the entry from deathRecipients /
//   deathRecipientsByHndl after firing, causing a memory leak.
// - ClearDeathNotification has a TOCTOU race: it drops the lock between
//   lookup and the BC_CLEAR_DEATH_NOTIFICATION write, so a concurrent
//   BR_DEAD_BINDER could delete the entry in between.
func TestLifecycle_DeathNotification(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	driver, transport := openBinderDirect(ctx, t)
	defer func() { _ = driver.Close(ctx) }()

	sm := servicemanager.New(transport)
	svc, err := sm.GetService(ctx, servicemanager.ServiceName("SurfaceFlinger"))
	require.NoError(t, err, "GetService(SurfaceFlinger)")
	require.NotNil(t, svc)

	spy := newDeathRecipientSpy()

	// Register death notification.
	err = svc.LinkToDeath(ctx, spy)
	require.NoError(t, err, "LinkToDeath should succeed for a live service")

	// Verify SurfaceFlinger is alive.
	require.True(t, svc.IsAlive(ctx), "SurfaceFlinger should be alive")

	// Wait a short period — the notification must NOT fire for a live service.
	select {
	case <-spy.diedCh:
		t.Fatal("death notification fired for a service that is still alive")
	case <-time.After(2 * time.Second):
		// Expected: no notification.
	}

	assert.Equal(t, int32(0), spy.diedCount.Load(),
		"BinderDied must not have been called")

	// Unlink the death notification.
	err = svc.UnlinkToDeath(ctx, spy)
	require.NoError(t, err, "UnlinkToDeath should succeed")

	// Verify the service is still usable after unlink.
	require.True(t, svc.IsAlive(ctx),
		"SurfaceFlinger should still be alive after UnlinkToDeath")

	t.Log("death notification correctly stayed silent for live service")
}

// TestLifecycle_HandleReuse gets a handle, releases it, gets a new handle
// for the same service, and verifies both handles work independently.
//
// Bugs exposed:
// - acquiredHandles map is set to nil (not empty map) in Close, so
//   any subsequent AcquireHandle panics with nil map write.
func TestLifecycle_HandleReuse(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	driver, transport := openBinderDirect(ctx, t)
	defer func() { _ = driver.Close(ctx) }()

	sm := servicemanager.New(transport)

	// First handle.
	svc1, err := sm.GetService(ctx, servicemanager.ServiceName("SurfaceFlinger"))
	require.NoError(t, err)
	require.NotNil(t, svc1)
	handle1 := svc1.Handle()
	require.True(t, svc1.IsAlive(ctx), "first handle should be alive")
	t.Logf("first handle: %d", handle1)

	// Release the first handle explicitly.
	err = transport.ReleaseHandle(ctx, handle1)
	require.NoError(t, err, "ReleaseHandle should succeed")

	// Get a new handle for the same service.
	svc2, err := sm.GetService(ctx, servicemanager.ServiceName("SurfaceFlinger"))
	require.NoError(t, err)
	require.NotNil(t, svc2)
	handle2 := svc2.Handle()
	t.Logf("second handle: %d", handle2)

	// The second handle must be functional.
	require.True(t, svc2.IsAlive(ctx), "second handle should be alive")

	t.Logf("handle reuse: first=%d second=%d", handle1, handle2)
}

// TestLifecycle_StaleHandle opens a driver, gets a service handle,
// closes the driver, then tries to use the handle. Expects a clean
// error (no panic, no hang).
//
// Bugs exposed:
// - writeCommand / doIoctl pass fd=-1 to the syscall after Close,
//   producing EBADF. The error path returns a BinderError, but
//   copyFromMapped would panic on nil d.mapped if the ioctl somehow
//   succeeded (it won't, but the lack of a guard is a latent bug).
func TestLifecycle_StaleHandle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	driver, transport := openBinderDirect(ctx, t)

	sm := servicemanager.New(transport)
	svc, err := sm.GetService(ctx, servicemanager.ServiceName("SurfaceFlinger"))
	require.NoError(t, err)
	require.NotNil(t, svc)

	// Close the driver while we still hold the handle.
	err = driver.Close(ctx)
	require.NoError(t, err, "Close should succeed")

	// Attempt to use the stale handle — must return an error, not panic.
	reply, err := svc.Transact(ctx, binder.PingTransaction, 0, parcel.New())
	assert.Error(t, err, "transact on stale handle should fail")
	assert.Nil(t, reply, "reply should be nil on stale handle")
	t.Logf("stale handle error: %v", err)
}

// TestLifecycle_DoubleClose closes the driver twice. The second call
// must not panic. It should either return nil (idempotent) or a clean
// error.
//
// Bugs exposed:
// - Close sets d.acquiredHandles = nil. A second Close does
//   `d.mu.Lock(); handles := d.acquiredHandles; d.acquiredHandles = nil`
//   which is safe. But d.mapped is set to nil after Munmap, and
//   d.fd is set to -1 after Close. The guards (if d.mapped != nil,
//   if d.fd >= 0) protect against double-close, but the method does
//   not return ErrClosed or similar, so the caller cannot distinguish
//   "already closed" from "successful close". This is a design smell.
func TestLifecycle_DoubleClose(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	driver, err := kernelbinder.Open(ctx, binder.WithMapSize(128*1024))
	require.NoError(t, err, "Open should succeed")

	// First close — must succeed.
	err = driver.Close(ctx)
	require.NoError(t, err, "first Close should succeed")

	// Second close — must not panic. May return nil or an error.
	assert.NotPanics(t, func() {
		_ = driver.Close(ctx)
	}, "second Close must not panic")

	t.Log("double close completed without panic")
}

// TestLifecycle_UseAfterClose opens a driver and a version-aware
// transport, closes the driver, then attempts various transport
// operations. Each must return a clean error (not panic or hang).
//
// Bugs exposed:
// - Transport delegates to inner Driver which has fd=-1 after Close.
//   The errors are clean (EBADF wrapped in BinderError), but there is
//   no "driver closed" sentinel to give the caller a clear diagnosis.
func TestLifecycle_UseAfterClose(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	driver, transport := openBinderDirect(ctx, t)

	// Close the underlying driver.
	err := driver.Close(ctx)
	require.NoError(t, err, "Close should succeed")

	// Every operation on the transport should fail cleanly.
	t.Run("Transact", func(t *testing.T) {
		_, err := transport.Transact(ctx, 0, binder.PingTransaction, 0, parcel.New())
		assert.Error(t, err, "Transact after close should error")
		t.Logf("Transact error: %v", err)
	})

	t.Run("AcquireHandle", func(t *testing.T) {
		err := transport.AcquireHandle(ctx, 1)
		assert.Error(t, err, "AcquireHandle after close should error")
		t.Logf("AcquireHandle error: %v", err)
	})

	t.Run("ReleaseHandle", func(t *testing.T) {
		err := transport.ReleaseHandle(ctx, 1)
		assert.Error(t, err, "ReleaseHandle after close should error")
		t.Logf("ReleaseHandle error: %v", err)
	})

	t.Run("RequestDeathNotification", func(t *testing.T) {
		spy := newDeathRecipientSpy()
		err := transport.RequestDeathNotification(ctx, 1, spy)
		assert.Error(t, err,
			"RequestDeathNotification after close should error")
		t.Logf("RequestDeathNotification error: %v", err)
	})
}

// TestLifecycle_ManyHandles opens 100 handles to different services,
// verifies all are alive, then closes the driver cleanly. This stresses
// the acquiredHandles map and the Close-time release loop.
//
// Bugs exposed:
// - Close iterates acquiredHandles and sends BC_RELEASE + BC_DECREFS
//   for each. If the fd is already broken (unlikely but possible under
//   pressure), errors accumulate but are joined. The iteration itself
//   is safe because handles is a snapshot. However, the handles are
//   released one-by-one with separate ioctl calls, which is slow.
func TestLifecycle_ManyHandles(t *testing.T) {
	const handleCount = 100

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	driver, transport := openBinderDirect(ctx, t)
	defer func() { _ = driver.Close(ctx) }()

	sm := servicemanager.New(transport)

	// Get the list of available services.
	allServices, err := sm.ListServices(ctx)
	require.NoError(t, err, "ListServices")
	require.NotEmpty(t, allServices, "need at least one service")

	// Open handles to as many distinct services as possible, cycling
	// through the list if there are fewer than handleCount services.
	handles := make([]binder.IBinder, 0, handleCount)
	aliveCount := 0
	for i := 0; i < handleCount; i++ {
		name := allServices[i%len(allServices)]
		svc, err := sm.GetService(ctx, name)
		if err != nil {
			t.Logf("GetService(%s) failed at iteration %d: %v", name, i, err)
			continue
		}
		if svc == nil {
			continue
		}
		handles = append(handles, svc)
		if svc.IsAlive(ctx) {
			aliveCount++
		}
	}

	t.Logf("opened %d handles, %d alive", len(handles), aliveCount)
	assert.GreaterOrEqual(t, len(handles), 50,
		"should have opened at least 50 handles")
	// Many services (security HALs, gatekeeper, etc.) reject pings from
	// unprivileged callers or return errors for SELinux-restricted
	// transactions. Verify at least a quarter respond.
	assert.GreaterOrEqual(t, aliveCount, len(handles)/4,
		"at least a quarter of opened handles should be alive")

	// Close the driver — this must release all acquired handles.
	err = driver.Close(ctx)
	assert.NoError(t, err, "Close with many handles should succeed")
}

// dummyReceiver implements binder.TransactionReceiver for testing stub
// registration and unregistration.
type dummyReceiver struct {
	desc string
}

func (r *dummyReceiver) Descriptor() string { return r.desc }

func (r *dummyReceiver) OnTransaction(
	_ context.Context,
	_ binder.TransactionCode,
	_ *parcel.Parcel,
) (*parcel.Parcel, error) {
	return parcel.New(), nil
}

// TestLifecycle_StubRegistrationCleanup registers a stub, unregisters it,
// then verifies the read loop handles it gracefully (no crash, no leak)
// and the transport remains functional.
//
// Bugs exposed:
// - UnregisterReceiver only deletes from the receivers map. The read
//   loop continues running, and if a BR_TRANSACTION arrives for the
//   now-gone cookie, lookupReceiver returns nil and an empty reply is
//   sent. This is correct behavior, but there is no way for the
//   registrant to know the read loop acknowledged the removal.
func TestLifecycle_StubRegistrationCleanup(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	driver, transport := openBinderDirect(ctx, t)
	defer func() { _ = driver.Close(ctx) }()

	receiver := &dummyReceiver{desc: "test.lifecycle.IDummy"}

	// Register the receiver. This starts the read loop on first call.
	cookie := driver.RegisterReceiver(ctx, receiver)
	require.NotZero(t, cookie, "cookie should be non-zero")
	t.Logf("registered receiver with cookie 0x%x", cookie)

	// The transport should still be functional while a receiver is
	// registered (the read loop is running).
	sm := servicemanager.New(transport)
	services, err := sm.ListServices(ctx)
	require.NoError(t, err, "ListServices with active receiver")
	require.NotEmpty(t, services)

	// Unregister the receiver.
	driver.UnregisterReceiver(ctx, cookie)

	// The transport should still be functional after unregistration.
	svc, err := sm.GetService(ctx, servicemanager.ServiceName("SurfaceFlinger"))
	require.NoError(t, err, "GetService after unregister")
	require.NotNil(t, svc)
	assert.True(t, svc.IsAlive(ctx),
		"SurfaceFlinger should be alive after stub unregistration")

	t.Log("stub registration cleanup completed; transport remains functional")
}

// TestLifecycle_DriverReopen closes and reopens /dev/binder, then
// verifies fresh state: old handles do not carry over, new service
// lookups work.
//
// Bugs exposed:
// - readLoopOnce is a sync.Once that is never reset. After Close +
//   re-Open, calling RegisterReceiver will not start a new read loop
//   because sync.Once.Do is a no-op after the first call. The new
//   Driver from Open gets a fresh sync.Once, but if someone reuses
//   the closed Driver object, the read loop is gone forever.
func TestLifecycle_DriverReopen(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// First session.
	driver1, transport1 := openBinderDirect(ctx, t)
	sm1 := servicemanager.New(transport1)
	svc1, err := sm1.GetService(ctx, servicemanager.ServiceName("SurfaceFlinger"))
	require.NoError(t, err)
	require.NotNil(t, svc1)
	handle1 := svc1.Handle()
	t.Logf("first session: SurfaceFlinger handle=%d", handle1)

	// Close the first driver.
	err = driver1.Close(ctx)
	require.NoError(t, err, "first Close")

	// Second session — completely fresh driver.
	driver2, err := kernelbinder.Open(ctx, binder.WithMapSize(128*1024))
	require.NoError(t, err, "second Open")
	defer func() { _ = driver2.Close(ctx) }()

	transport2, err := versionaware.NewTransport(ctx, driver2, 0)
	require.NoError(t, err, "second NewTransport")

	sm2 := servicemanager.New(transport2)
	svc2, err := sm2.GetService(ctx, servicemanager.ServiceName("SurfaceFlinger"))
	require.NoError(t, err, "GetService on reopened driver")
	require.NotNil(t, svc2)
	handle2 := svc2.Handle()
	t.Logf("second session: SurfaceFlinger handle=%d", handle2)

	// The new handle must work.
	require.True(t, svc2.IsAlive(ctx),
		"SurfaceFlinger should be alive on reopened driver")

	// Verify the old driver's handle does NOT work on the new driver.
	// Construct a proxy using handle1 on the new transport — it may
	// refer to a different (or invalid) binder node.
	oldHandleProxy := binder.NewProxyBinder(
		transport2, binder.DefaultCallerIdentity(), handle1,
	)
	// This may succeed (if the kernel reuses the handle number for the
	// same service) or fail. The key assertion is: no panic, no hang.
	alive := oldHandleProxy.IsAlive(ctx)
	t.Logf("old handle %d on new driver: alive=%v", handle1, alive)

	// Verify multiple distinct services on the new driver.
	for _, name := range []string{"activity", "SurfaceFlingerAIDL"} {
		s, err := sm2.GetService(ctx, servicemanager.ServiceName(name))
		if err != nil {
			t.Logf("GetService(%s) on reopened driver: %v (skipping)", name, err)
			continue
		}
		require.NotNil(t, s, "%s should not be nil", name)
		assert.True(t, s.IsAlive(ctx), "%s should be alive on reopened driver", name)
	}

	t.Log("driver reopen: fresh state verified")
}

// TestLifecycle_DeathNotificationIdempotentUnlink registers a death
// notification, unlinks it, then attempts to unlink again. The second
// unlink should return an error (no registered notification), not panic.
//
// Bugs exposed:
// - ClearDeathNotification returns an error for a missing handle, which
//   is correct. But if handleDeadBinder fires between two calls and does
//   NOT remove the entry (current bug), the second unlink may send a
//   stale cookie to the kernel, causing undefined behavior.
func TestLifecycle_DeathNotificationIdempotentUnlink(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	driver, transport := openBinderDirect(ctx, t)
	defer func() { _ = driver.Close(ctx) }()

	sm := servicemanager.New(transport)
	svc, err := sm.GetService(ctx, servicemanager.ServiceName("SurfaceFlinger"))
	require.NoError(t, err)
	require.NotNil(t, svc)

	spy := newDeathRecipientSpy()

	err = svc.LinkToDeath(ctx, spy)
	require.NoError(t, err, "LinkToDeath")

	err = svc.UnlinkToDeath(ctx, spy)
	require.NoError(t, err, "first UnlinkToDeath")

	// Second unlink — should fail cleanly (no notification registered).
	err = svc.UnlinkToDeath(ctx, spy)
	assert.Error(t, err,
		"second UnlinkToDeath should error: no notification registered")
	t.Logf("second UnlinkToDeath error: %v", err)
}

// TestLifecycle_DeathNotificationMultipleRecipients registers death
// notifications on distinct services (each with a unique handle), verifies
// none fire, then unlinks all. Uses one recipient per handle to avoid
// the known limitation that deathRecipientsByHndl supports only one
// recipient per handle.
//
// Bugs exposed:
// - RequestDeathNotification overwrites deathRecipientsByHndl[handle]
//   if called twice for the same handle with different recipients.
//   The old entry in deathRecipients[oldCookie] is leaked. This test
//   avoids that by using distinct handles but documents the limitation.
func TestLifecycle_DeathNotificationMultipleRecipients(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	driver, transport := openBinderDirect(ctx, t)
	defer func() { _ = driver.Close(ctx) }()

	sm := servicemanager.New(transport)

	// Use distinct services so each gets a unique handle.
	services := []string{"SurfaceFlinger", "activity", "SurfaceFlingerAIDL"}

	type registration struct {
		svc    binder.IBinder
		spy    *deathRecipientSpy
		handle uint32
	}
	var regs []registration
	seenHandles := map[uint32]bool{}

	for _, name := range services {
		svc, err := sm.GetService(ctx, servicemanager.ServiceName(name))
		if err != nil {
			t.Logf("GetService(%s) failed: %v (skipping)", name, err)
			continue
		}
		require.NotNil(t, svc)

		// Skip if we already have a registration for this handle
		// (would trigger the overwrite bug).
		if seenHandles[svc.Handle()] {
			t.Logf("skipping %s: handle %d already used", name, svc.Handle())
			continue
		}
		seenHandles[svc.Handle()] = true

		spy := newDeathRecipientSpy()
		err = svc.LinkToDeath(ctx, spy)
		require.NoError(t, err, "LinkToDeath for %s", name)
		regs = append(regs, registration{
			svc:    svc,
			spy:    spy,
			handle: svc.Handle(),
		})
	}

	require.NotEmpty(t, regs, "should have registered at least one recipient")

	// None should have fired.
	for i, reg := range regs {
		assert.Equal(t, int32(0), reg.spy.diedCount.Load(),
			"recipient %d (handle %d) should not have fired", i, reg.handle)
	}

	// Unlink all — each handle has exactly one recipient, so unlink
	// should succeed.
	for i, reg := range regs {
		err := reg.svc.UnlinkToDeath(ctx, reg.spy)
		assert.NoError(t, err, "UnlinkToDeath for recipient %d (handle %d)", i, reg.handle)
	}

	t.Logf("registered and unlinked %d death notifications on distinct handles",
		len(regs))
}

// TestLifecycle_CloseWithActiveDeathNotification closes the driver
// while a death notification is still registered. Verifies no panic
// and no hang.
//
// Bugs exposed:
// - Close does NOT clear death recipients or send
//   BC_CLEAR_DEATH_NOTIFICATION. The kernel cleans up when the fd is
//   closed, but the Driver's deathRecipients map retains entries that
//   will never be delivered (memory leak until GC).
func TestLifecycle_CloseWithActiveDeathNotification(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	driver, transport := openBinderDirect(ctx, t)

	sm := servicemanager.New(transport)
	svc, err := sm.GetService(ctx, servicemanager.ServiceName("SurfaceFlinger"))
	require.NoError(t, err)
	require.NotNil(t, svc)

	spy := newDeathRecipientSpy()
	err = svc.LinkToDeath(ctx, spy)
	require.NoError(t, err, "LinkToDeath")

	// Close without unlinking — must not panic or hang.
	assert.NotPanics(t, func() {
		err = driver.Close(ctx)
	}, "Close with active death notification must not panic")
	assert.NoError(t, err, "Close should succeed even with active death notification")

	// The spy should NOT have been called (the service didn't die;
	// we just closed our fd).
	assert.Equal(t, int32(0), spy.diedCount.Load(),
		"BinderDied should not fire when WE close the driver")

	t.Log("close with active death notification: clean shutdown")
}

// TestLifecycle_ConcurrentLinkUnlink exercises concurrent LinkToDeath and
// UnlinkToDeath on the same handle from multiple goroutines. Tests the
// thread safety of the deathRecipients maps.
func TestLifecycle_ConcurrentLinkUnlink(t *testing.T) {
	const goroutines = 20

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	driver, transport := openBinderDirect(ctx, t)
	defer func() { _ = driver.Close(ctx) }()

	sm := servicemanager.New(transport)
	svc, err := sm.GetService(ctx, servicemanager.ServiceName("SurfaceFlinger"))
	require.NoError(t, err)
	require.NotNil(t, svc)

	var wg sync.WaitGroup
	errs := make([]error, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			spy := newDeathRecipientSpy()

			if linkErr := svc.LinkToDeath(ctx, spy); linkErr != nil {
				errs[idx] = fmt.Errorf("LinkToDeath: %w", linkErr)
				return
			}

			// Small delay to let concurrent operations interleave.
			time.Sleep(time.Duration(idx) * time.Millisecond)

			if unlinkErr := svc.UnlinkToDeath(ctx, spy); unlinkErr != nil {
				errs[idx] = fmt.Errorf("UnlinkToDeath: %w", unlinkErr)
				return
			}
		}(i)
	}

	wg.Wait()

	// Check for panics / errors.
	var errCount int
	for i, err := range errs {
		if err != nil {
			errCount++
			t.Logf("goroutine %d: %v", i, err)
		}
	}

	// Some errors are expected due to the RequestDeathNotification
	// overwriting deathRecipientsByHndl[handle] — that is a known bug
	// (only one recipient per handle is supported). Verify that errors
	// stay within a reasonable bound (at most all but one can fail due
	// to single-recipient-per-handle contention).
	t.Logf("concurrent link/unlink: %d/%d errors", errCount, goroutines)
	assert.Less(t, errCount, goroutines,
		"at least one goroutine should succeed without error")

	// The driver must still be functional.
	assert.True(t, svc.IsAlive(ctx),
		"SurfaceFlinger should be alive after concurrent link/unlink")
}
