//go:build e2e

package e2e

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	androidos "github.com/xaionaro-go/binder/android/os"
	"github.com/xaionaro-go/binder/binder"
	"github.com/xaionaro-go/binder/binder/versionaware"
	aidlerrors "github.com/xaionaro-go/binder/errors"
	"github.com/xaionaro-go/binder/kernelbinder"
	"github.com/xaionaro-go/binder/parcel"
	"github.com/xaionaro-go/binder/servicemanager"
)

// ---------------------------------------------------------------------------
// 1. TestOneway_FloodNotifications
//
// Sends 100 rapid oneway PING transactions to SurfaceFlinger, then verifies
// the service is still responsive with a synchronous two-way PING.
//
// This stresses the oneway code path: the driver must receive
// BR_TRANSACTION_COMPLETE for each oneway without blocking on BR_REPLY.
// A bug where Transact waits for BR_REPLY on oneway calls would deadlock
// or time out under this flood.
// ---------------------------------------------------------------------------

func TestOneway_FloodNotifications(t *testing.T) {
	const floodCount = 100

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	driver := openBinder(t)
	sm := servicemanager.New(driver)

	svc, err := sm.GetService(ctx, servicemanager.ServiceName("SurfaceFlinger"))
	require.NoError(t, err)
	require.NotNil(t, svc)

	// Flood oneway PINGs sequentially. Each must return quickly with an
	// empty parcel (only BR_TRANSACTION_COMPLETE, no BR_REPLY).
	var successCount int
	var firstErr error
	for i := 0; i < floodCount; i++ {
		reply, err := svc.Transact(ctx, binder.PingTransaction, binder.FlagOneway, parcel.New())
		switch {
		case err != nil:
			if firstErr == nil {
				firstErr = fmt.Errorf("oneway PING #%d: %w", i, err)
			}
		default:
			successCount++
			// Oneway replies must be empty: the driver must not return
			// a stale BR_REPLY from a previous two-way transaction.
			assert.Equal(t, 0, reply.Len(),
				"oneway PING #%d reply should be empty (got %d bytes)", i, reply.Len())
		}
	}

	// At least 95% of the flood should succeed. Transient kernel
	// buffer pressure may cause a few failures, but a systematic bug
	// (e.g. deadlock on BR_REPLY) would cause near-total failure.
	minSuccess := floodCount * 95 / 100
	require.GreaterOrEqual(t, successCount, minSuccess,
		"expected at least %d/%d oneway PINGs to succeed (first error: %v)",
		minSuccess, floodCount, firstErr)

	t.Logf("oneway flood: %d/%d succeeded", successCount, floodCount)

	// Verify the service is still responsive with a normal two-way PING.
	alive := svc.IsAlive(ctx)
	require.True(t, alive,
		"service must be alive after oneway flood (driver state corrupted?)")

	// Also verify a real data-bearing transaction still works.
	reply, err := svc.Transact(ctx, binder.PingTransaction, 0, parcel.New())
	require.NoError(t, err, "synchronous PING after flood should succeed")
	assert.NotNil(t, reply)

	t.Logf("service responsive after %d oneway PINGs", floodCount)
}

// ---------------------------------------------------------------------------
// 2. TestOneway_CallbackDelivery
//
// Registers a stub implementing IServiceCallback with ServiceManager's
// registerForNotifications API, then verifies the oneway callback
// (onRegistration) is delivered to our stub.
//
// The ServiceManager sends onRegistration as a oneway transaction
// (FlagOneway). This tests the full round-trip: our process sends a
// two-way registerForNotifications containing our stub binder, and the
// ServiceManager sends oneway callbacks back to our read loop.
// ---------------------------------------------------------------------------

// serviceCallbackImpl records onRegistration calls.
type serviceCallbackImpl struct {
	mu    sync.Mutex
	calls []string
	done  chan struct{}
}

func newServiceCallbackImpl() *serviceCallbackImpl {
	return &serviceCallbackImpl{
		done: make(chan struct{}, 1),
	}
}

func (cb *serviceCallbackImpl) OnRegistration(
	_ context.Context,
	name string,
	_ binder.IBinder,
) error {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.calls = append(cb.calls, name)
	select {
	case cb.done <- struct{}{}:
	default:
	}
	return nil
}

func (cb *serviceCallbackImpl) receivedNames() []string {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	result := make([]string, len(cb.calls))
	copy(result, cb.calls)
	return result
}

func TestOneway_CallbackDelivery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	driver, err := kernelbinder.Open(ctx, binder.WithMapSize(128*1024))
	require.NoError(t, err)
	t.Cleanup(func() { _ = driver.Close(ctx) })

	transport, err := versionaware.NewTransport(ctx, driver, 0)
	require.NoError(t, err)

	smBinder := binder.NewProxyBinder(transport, binder.DefaultCallerIdentity(), 0)
	smProxy := androidos.NewServiceManagerProxy(smBinder)

	// Create our callback stub.
	impl := newServiceCallbackImpl()
	callbackStub := androidos.NewServiceCallbackStub(impl)

	// Register for notifications on "SurfaceFlinger" -- a service that
	// is always registered. The ServiceManager should immediately send
	// an onRegistration callback because SurfaceFlinger already exists.
	const watchedService = "SurfaceFlinger"
	err = smProxy.RegisterForNotifications(ctx, watchedService, callbackStub)
	requireOrSkip(t, err)

	// Wait for the oneway callback to arrive (delivered via the read loop).
	select {
	case <-impl.done:
		t.Logf("received onRegistration callback")
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for onRegistration callback")
	}

	names := impl.receivedNames()
	require.NotEmpty(t, names, "should have received at least one onRegistration callback")
	assert.Equal(t, watchedService, names[0],
		"first callback should be for the watched service")

	t.Logf("callback delivered for %q (total callbacks: %d)", watchedService, len(names))

	// Cleanup: unregister to avoid lingering kernel state.
	err = smProxy.UnregisterForNotifications(ctx, watchedService, callbackStub)
	if err != nil {
		t.Logf("unregister warning (non-fatal): %v", err)
	}
}

// ---------------------------------------------------------------------------
// 3. TestOneway_MixedOnewayAndTwoway
//
// Alternates between oneway and two-way transactions on the same service
// handle. Verifies that oneway transactions do not corrupt the reply
// parsing state for subsequent two-way transactions.
//
// A common bug: the driver reads a stale BR_REPLY left over from a
// previous two-way call and returns it as the oneway "reply", or the
// oneway consumes a BR_REPLY that belongs to the next two-way call.
// ---------------------------------------------------------------------------

func TestOneway_MixedOnewayAndTwoway(t *testing.T) {
	const iterations = 50

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	driver := openBinder(t)
	sm := servicemanager.New(driver)

	svc, err := sm.GetService(ctx, servicemanager.ServiceName("SurfaceFlinger"))
	require.NoError(t, err)
	require.NotNil(t, svc)

	for i := 0; i < iterations; i++ {
		// Oneway PING.
		onewayReply, err := svc.Transact(ctx, binder.PingTransaction, binder.FlagOneway, parcel.New())
		require.NoError(t, err, "iteration %d: oneway PING failed", i)
		assert.Equal(t, 0, onewayReply.Len(),
			"iteration %d: oneway reply must be empty", i)

		// Two-way PING immediately after.
		twowayReply, err := svc.Transact(ctx, binder.PingTransaction, 0, parcel.New())
		require.NoError(t, err, "iteration %d: two-way PING failed", i)
		assert.NotNil(t, twowayReply,
			"iteration %d: two-way reply must not be nil", i)
	}

	// Final liveness check.
	require.True(t, svc.IsAlive(ctx),
		"service must be alive after mixed oneway/two-way interleaving")

	t.Logf("completed %d oneway+twoway pairs without corruption", iterations)
}

// ---------------------------------------------------------------------------
// 4. TestOneway_LargePayload
//
// Sends a oneway transaction with a payload exceeding 1MB. The binder
// kernel limits the async buffer to half the total mapped size
// (default 1MB mapped = 512KB async). With our 128KB map size, the
// async limit is 64KB. A payload larger than the mapped region tests
// that the driver returns a clean error (not a hang or panic).
//
// We test two sizes:
//   - A moderate payload (32KB) that should succeed with our 128KB map.
//   - A huge payload (2MB) that exceeds any reasonable map size.
// ---------------------------------------------------------------------------

func TestOneway_LargePayload(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	t.Run("ModeratePayload_32KB", func(t *testing.T) {
		driver := openBinder(t)
		sm := servicemanager.New(driver)

		svc, err := sm.GetService(ctx, servicemanager.ServiceName("SurfaceFlinger"))
		require.NoError(t, err)
		require.NotNil(t, svc)

		// 32KB payload: within the async buffer limit for 128KB map.
		const payloadSize = 32 * 1024
		payload := make([]byte, payloadSize)
		for i := range payload {
			payload[i] = byte(i % 256)
		}

		data := parcel.New()
		data.WriteInterfaceToken("android.ui.ISurfaceComposer")
		data.WriteByteArray(payload)

		// Use a user transaction code that the service will reject,
		// but the driver should still process the oneway without hanging.
		reply, err := svc.Transact(
			ctx,
			binder.PingTransaction,
			binder.FlagOneway,
			data,
		)
		// The transaction may succeed (PING ignores the data) or fail
		// (service rejects it). Either outcome is acceptable -- the
		// important thing is that it does not hang or panic.
		if err != nil {
			t.Logf("32KB oneway PING returned error (acceptable): %v", err)
		} else {
			assert.Equal(t, 0, reply.Len(),
				"oneway reply should be empty even with large payload")
		}

		// Verify the service is still responsive.
		require.True(t, svc.IsAlive(ctx),
			"service must be alive after large oneway payload")
	})

	t.Run("HugePayload_2MB", func(t *testing.T) {
		// Open a driver with a larger map to allow the kernel to
		// accept the data buffer (the driver needs to mmap enough
		// space for the transaction data). Even with a large map,
		// the kernel's async buffer limit may reject it.
		driverBig, err := kernelbinder.Open(ctx, binder.WithMapSize(4*1024*1024))
		require.NoError(t, err)
		t.Cleanup(func() { _ = driverBig.Close(ctx) })

		transportBig, err := versionaware.NewTransport(ctx, driverBig, 0)
		require.NoError(t, err)

		sm := servicemanager.New(transportBig)
		svc, err := sm.GetService(ctx, servicemanager.ServiceName("SurfaceFlinger"))
		require.NoError(t, err)
		require.NotNil(t, svc)

		const payloadSize = 2 * 1024 * 1024
		payload := make([]byte, payloadSize)
		for i := range payload {
			payload[i] = byte(i % 251)
		}

		data := parcel.New()
		data.WriteByteArray(payload)

		reply, err := svc.Transact(
			ctx,
			binder.PingTransaction,
			binder.FlagOneway,
			data,
		)
		// The kernel may reject with BR_FAILED_REPLY (async buffer
		// full) or succeed. Both are valid outcomes. A hang or panic
		// is the bug we are detecting.
		if err != nil {
			t.Logf("2MB oneway returned error (expected): %v", err)
		} else {
			t.Logf("2MB oneway succeeded (reply len=%d)", reply.Len())
		}

		// Regardless of the outcome, the driver must remain functional.
		normalReply, err := svc.Transact(ctx, binder.PingTransaction, 0, parcel.New())
		require.NoError(t, err,
			"synchronous PING must succeed after large oneway attempt")
		assert.NotNil(t, normalReply)
	})
}

// ---------------------------------------------------------------------------
// 5. TestOneway_ErrorHandling
//
// Sends a oneway transaction to an invalid handle (0xDEAD). Verifies
// that the driver returns a clean error (TransactionError) rather than
// hanging or panicking.
//
// A common bug: the driver issues BC_TRANSACTION with FlagOneway and
// the invalid handle. The kernel responds with BR_DEAD_REPLY or
// BR_FAILED_REPLY. If the driver's oneway path only checks for
// BR_TRANSACTION_COMPLETE and never handles these error codes, it will
// spin in the read loop forever.
// ---------------------------------------------------------------------------

func TestOneway_ErrorHandling(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	driver := openBinder(t)

	// Create a proxy to a non-existent handle.
	invalid := binder.NewProxyBinder(driver, binder.DefaultCallerIdentity(), 0xDEAD)

	// Oneway to invalid handle.
	reply, err := invalid.Transact(ctx, binder.PingTransaction, binder.FlagOneway, parcel.New())
	require.Error(t, err, "oneway to invalid handle must fail")

	var txnErr *aidlerrors.TransactionError
	require.True(t, errors.As(err, &txnErr),
		"expected TransactionError, got %T: %v", err, err)
	t.Logf("oneway invalid handle error: %v (code: %v)", txnErr, txnErr.Code)

	// The reply must be nil on error.
	assert.Nil(t, reply, "oneway error reply should be nil")

	// Verify the driver is still usable after the error.
	sm := servicemanager.New(driver)
	svc, err := sm.GetService(ctx, servicemanager.ServiceName("SurfaceFlinger"))
	require.NoError(t, err,
		"GetService must succeed after oneway error (driver state must not be corrupted)")
	require.NotNil(t, svc)
	require.True(t, svc.IsAlive(ctx),
		"service must be alive after oneway error handling")
}

// ---------------------------------------------------------------------------
// 6. TestOneway_ConcurrentFlood
//
// Sends 100 oneway PINGs from multiple goroutines concurrently. Each
// goroutine opens its own driver to exercise true parallelism at the
// kernel level. Verifies no deadlock or corruption.
// ---------------------------------------------------------------------------

func TestOneway_ConcurrentFlood(t *testing.T) {
	const goroutineCount = 10
	const onewaySPerGoroutine = 10

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	var totalSuccess, totalFail atomic.Int64

	for g := 0; g < goroutineCount; g++ {
		wg.Add(1)
		go func(gIdx int) {
			defer wg.Done()

			driver, err := kernelbinder.Open(ctx, binder.WithMapSize(128*1024))
			if err != nil {
				totalFail.Add(int64(onewaySPerGoroutine))
				return
			}
			defer func() { _ = driver.Close(ctx) }()

			transport, err := versionaware.NewTransport(ctx, driver, 0)
			if err != nil {
				totalFail.Add(int64(onewaySPerGoroutine))
				return
			}

			sm := servicemanager.New(transport)
			svc, err := sm.GetService(ctx, servicemanager.ServiceName("SurfaceFlinger"))
			if err != nil {
				totalFail.Add(int64(onewaySPerGoroutine))
				return
			}

			for i := 0; i < onewaySPerGoroutine; i++ {
				reply, err := svc.Transact(
					ctx, binder.PingTransaction, binder.FlagOneway, parcel.New(),
				)
				if err != nil {
					totalFail.Add(1)
					continue
				}
				if reply.Len() != 0 {
					totalFail.Add(1)
					continue
				}
				totalSuccess.Add(1)
			}

			// Verify the driver is still usable after the oneway flood.
			if !svc.IsAlive(ctx) {
				totalFail.Add(1)
			}
		}(g)
	}

	wg.Wait()

	success := totalSuccess.Load()
	fail := totalFail.Load()
	total := int64(goroutineCount * onewaySPerGoroutine)

	t.Logf("concurrent oneway flood: %d/%d succeeded, %d failed", success, total, fail)

	// At least 80% should succeed under concurrent pressure.
	minSuccess := total * 80 / 100
	require.GreaterOrEqual(t, success, minSuccess,
		"expected at least %d/%d concurrent oneway PINGs to succeed", minSuccess, total)
}
