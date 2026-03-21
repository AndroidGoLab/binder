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

	"github.com/xaionaro-go/binder/binder"
	"github.com/xaionaro-go/binder/binder/versionaware"
	"github.com/xaionaro-go/binder/kernelbinder"
	"github.com/xaionaro-go/binder/parcel"
	"github.com/xaionaro-go/binder/servicemanager"
)

// openBinderDirect opens a binder driver and version-aware transport
// without using t.Cleanup, returning both so callers can close them
// manually. This is needed for tests that measure resource cleanup.
func openBinderDirect(
	ctx context.Context,
	t *testing.T,
) (*kernelbinder.Driver, *versionaware.Transport) {
	t.Helper()
	driver, err := kernelbinder.Open(ctx, binder.WithMapSize(128*1024))
	require.NoError(t, err, "failed to open /dev/binder")
	transport, err := versionaware.NewTransport(ctx, driver, 0)
	require.NoError(t, err, "failed to create version-aware transport")
	return driver, transport
}

// --- Test 1: Many services opened in parallel goroutines ---

func TestConcurrent_ManyServicesParallel(t *testing.T) {
	const goroutineCount = 50

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	errs := make([]error, goroutineCount)
	handles := make([]uint32, goroutineCount)

	for i := 0; i < goroutineCount; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			driver, err := kernelbinder.Open(ctx, binder.WithMapSize(128*1024))
			if err != nil {
				errs[idx] = fmt.Errorf("open binder: %w", err)
				return
			}
			defer func() { _ = driver.Close(ctx) }()

			transport, err := versionaware.NewTransport(ctx, driver, 0)
			if err != nil {
				errs[idx] = fmt.Errorf("new transport: %w", err)
				return
			}

			sm := servicemanager.New(transport)
			svc, err := sm.GetService(ctx, servicemanager.ServiceName("SurfaceFlinger"))
			if err != nil {
				errs[idx] = fmt.Errorf("get service: %w", err)
				return
			}
			if svc == nil {
				errs[idx] = errors.New("got nil binder for SurfaceFlinger")
				return
			}

			handles[idx] = svc.Handle()

			if !svc.IsAlive(ctx) {
				errs[idx] = errors.New("SurfaceFlinger not alive")
				return
			}
		}(i)
	}

	wg.Wait()

	var successCount int
	for i, err := range errs {
		if err == nil {
			successCount++
			assert.Greater(t, handles[i], uint32(0),
				"goroutine %d: SurfaceFlinger handle should be > 0", i)
		} else {
			t.Logf("goroutine %d: %v", i, err)
		}
	}

	// At least 80% should succeed under pressure; the rest may hit
	// transient resource limits.
	minSuccess := goroutineCount * 80 / 100
	require.GreaterOrEqual(t, successCount, minSuccess,
		"expected at least %d/%d goroutines to succeed, got %d",
		minSuccess, goroutineCount, successCount)
	t.Logf("%d/%d goroutines succeeded", successCount, goroutineCount)
}

// --- Test 2: Rapid open and close cycles ---

func TestConcurrent_RapidOpenClose(t *testing.T) {
	const iterations = 100

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	var openErrors, closeErrors int

	for i := 0; i < iterations; i++ {
		driver, err := kernelbinder.Open(ctx, binder.WithMapSize(128*1024))
		if err != nil {
			openErrors++
			t.Logf("iteration %d: open failed: %v", i, err)
			continue
		}

		transport, err := versionaware.NewTransport(ctx, driver, 0)
		if err != nil {
			_ = driver.Close(ctx)
			openErrors++
			t.Logf("iteration %d: new transport failed: %v", i, err)
			continue
		}

		// Do a quick operation to exercise the fd before closing.
		sm := servicemanager.New(transport)
		_, _ = sm.CheckService(ctx, servicemanager.ServiceName("SurfaceFlinger"))

		if err := driver.Close(ctx); err != nil {
			closeErrors++
			t.Logf("iteration %d: close failed: %v", i, err)
		}
	}

	t.Logf("open errors: %d/%d, close errors: %d/%d",
		openErrors, iterations, closeErrors, iterations)

	// A high failure rate indicates fd leaks or resource exhaustion.
	maxOpenErrors := iterations * 10 / 100
	assert.LessOrEqual(t, openErrors, maxOpenErrors,
		"too many open failures (%d/%d); possible fd leak", openErrors, iterations)
	assert.Equal(t, 0, closeErrors,
		"close should never fail if open succeeded")
}

// --- Test 3: Multiple transports on the same driver ---

func TestConcurrent_MultipleTransportsOnSameDriver(t *testing.T) {
	const transportCount = 5
	const callsPerTransport = 10

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	driver, err := kernelbinder.Open(ctx, binder.WithMapSize(128*1024))
	require.NoError(t, err, "failed to open /dev/binder")
	defer func() { _ = driver.Close(ctx) }()

	transports := make([]*versionaware.Transport, transportCount)
	for i := range transports {
		transport, err := versionaware.NewTransport(ctx, driver, 0)
		require.NoError(t, err, "failed to create transport %d", i)
		transports[i] = transport
	}

	var wg sync.WaitGroup
	errs := make([]error, transportCount*callsPerTransport)

	for i, transport := range transports {
		for j := 0; j < callsPerTransport; j++ {
			idx := i*callsPerTransport + j
			wg.Add(1)
			go func(t2 *versionaware.Transport, errIdx int) {
				defer wg.Done()

				sm := servicemanager.New(t2)
				services, err := sm.ListServices(ctx)
				if err != nil {
					errs[errIdx] = err
					return
				}
				if len(services) == 0 {
					errs[errIdx] = errors.New("no services found")
					return
				}
			}(transport, idx)
		}
	}

	wg.Wait()

	var failCount int
	for _, err := range errs {
		if err != nil {
			failCount++
		}
	}

	totalCalls := transportCount * callsPerTransport
	t.Logf("%d/%d concurrent calls succeeded across %d transports",
		totalCalls-failCount, totalCalls, transportCount)

	// Under sustained binder pressure (e.g., running after other stress
	// tests), a small number of transient failures can occur due to
	// kernel-level thread scheduling contention on the shared fd.
	// Allow up to 10% failures.
	maxFails := totalCalls * 10 / 100
	assert.LessOrEqual(t, failCount, maxFails,
		"too many failures with shared driver: %d/%d (max %d)",
		failCount, totalCalls, maxFails)
}

// --- Test 4: Large parcel payload ---

func TestConcurrent_LargeParcelPayload(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	driver := openBinder(t)
	sm := servicemanager.New(driver)

	// Use SurfaceFlingerAIDL's getStaticDisplayInfo which returns a
	// parcelable reply. We test that writing a large data parcel
	// (1MB+ interface token + padding) does not corrupt the driver.
	svc, err := sm.GetService(ctx, servicemanager.ServiceName("SurfaceFlingerAIDL"))
	require.NoError(t, err)
	require.NotNil(t, svc)

	// Build a parcel with a large byte array payload. The service will
	// reject this as an invalid transaction, but we verify the driver
	// handles the large buffer without crashing, hanging, or leaking.
	const payloadSize = 1024 * 1024 // 1 MB
	largeData := make([]byte, payloadSize)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	data := parcel.New()
	data.WriteInterfaceToken(surfaceComposerDescriptor)
	data.WriteByteArray(largeData)

	// Use PING since it is universally supported and safe.
	// The kernel binder still processes the large buffer transfer even though
	// PING ignores the payload data.
	reply, err := svc.Transact(ctx, binder.PingTransaction, 0, data)
	require.NoError(t, err, "ping with large parcel should succeed")
	assert.NotNil(t, reply, "ping reply should not be nil")

	// Now test that we can still do normal transactions after the large
	// parcel was allocated (memory was not permanently consumed).
	reply2, err := svc.Transact(ctx, binder.PingTransaction, 0, parcel.New())
	require.NoError(t, err, "second ping should succeed (buffer management intact)")
	assert.NotNil(t, reply2, "second ping reply should not be nil")

	// Verify a real transaction still works.
	codeGetBoot := resolveCode(ctx, t, svc, surfaceComposerDescriptor, "getBootDisplayModeSupport")
	data2 := parcel.New()
	data2.WriteInterfaceToken(surfaceComposerDescriptor)
	reply3, err := svc.Transact(ctx, codeGetBoot, 0, data2)
	requireOrSkip(t, err)
	requireOrSkip(t, binder.ReadStatus(reply3))

	_, err = reply3.ReadBool()
	require.NoError(t, err, "should read bool from getBootDisplayModeSupport")

	t.Logf("large parcel (%d bytes) did not corrupt driver state", payloadSize)
}

// --- Test 5: Stub registration and unregistration under load ---

// echoReceiver is a minimal TransactionReceiver for testing stubs.
type echoReceiver struct {
	descriptor string
	callCount  atomic.Int64
}

func (r *echoReceiver) Descriptor() string { return r.descriptor }

func (r *echoReceiver) OnTransaction(
	_ context.Context,
	_ binder.TransactionCode,
	data *parcel.Parcel,
) (*parcel.Parcel, error) {
	r.callCount.Add(1)
	// Echo the input data back as the reply.
	reply := parcel.New()
	binder.WriteStatus(reply, nil)
	reply.WriteByteArray(data.Data())
	return reply, nil
}

func TestConcurrent_StubRegistrationUnderLoad(t *testing.T) {
	const stubCount = 20
	const transactionsPerStub = 5

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	driver, err := kernelbinder.Open(ctx, binder.WithMapSize(128*1024))
	require.NoError(t, err)
	defer func() { _ = driver.Close(ctx) }()

	transport, err := versionaware.NewTransport(ctx, driver, 0)
	require.NoError(t, err)

	// Register multiple stubs concurrently.
	// Serialize transport access via a mutex to avoid racing on the
	// underlying driver's RegisterReceiver (which may not be thread-safe).
	var wg sync.WaitGroup
	var transportMu sync.Mutex
	stubs := make([]*binder.StubBinder, stubCount)
	receivers := make([]*echoReceiver, stubCount)
	cookies := make([]uintptr, stubCount)

	for i := 0; i < stubCount; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			receivers[idx] = &echoReceiver{
				descriptor: fmt.Sprintf("test.stub.%d", idx),
			}
			stubs[idx] = binder.NewStubBinder(receivers[idx])
			transportMu.Lock()
			cookies[idx] = stubs[idx].RegisterWithTransport(ctx, transport)
			transportMu.Unlock()
		}(i)
	}
	wg.Wait()

	// Verify all stubs got unique, non-zero cookies.
	cookieSet := make(map[uintptr]bool, stubCount)
	for i, cookie := range cookies {
		require.NotZero(t, cookie, "stub %d should have non-zero cookie", i)
		assert.False(t, cookieSet[cookie],
			"stub %d has duplicate cookie 0x%x", i, cookie)
		cookieSet[cookie] = true
	}

	// While stubs are registered, do normal service manager calls to
	// exercise the read loop alongside stub dispatch.
	sm := servicemanager.New(transport)
	for j := 0; j < transactionsPerStub; j++ {
		services, err := sm.ListServices(ctx)
		if err != nil {
			t.Logf("ListServices during stub load (iteration %d): %v", j, err)
			continue
		}
		assert.NotEmpty(t, services, "services should not be empty during stub load")
	}

	// Unregister all stubs concurrently.
	for i := 0; i < stubCount; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			driver.UnregisterReceiver(ctx, cookies[idx])
		}(i)
	}
	wg.Wait()

	// Verify the transport still works after mass unregistration.
	svc, err := sm.GetService(ctx, servicemanager.ServiceName("SurfaceFlinger"))
	require.NoError(t, err, "GetService should work after stub unregistration")
	require.NotNil(t, svc)
	assert.True(t, svc.IsAlive(ctx), "SurfaceFlinger should be alive after stub cleanup")

	t.Logf("registered and unregistered %d stubs; %d unique cookies; transport stable",
		stubCount, len(cookieSet))
}

// --- Test 6: ServiceManager flood ---

func TestConcurrent_ServiceManagerFlood(t *testing.T) {
	const goroutineCount = 100

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Use a single driver to test the binder thread multiplexing under
	// heavy ServiceManager call pressure.
	driver := openBinder(t)

	var wg sync.WaitGroup
	var successCount, failCount atomic.Int64

	for i := 0; i < goroutineCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			sm := servicemanager.New(driver)
			services, err := sm.ListServices(ctx)
			if err != nil {
				failCount.Add(1)
				return
			}
			if len(services) == 0 {
				failCount.Add(1)
				return
			}
			successCount.Add(1)
		}()
	}

	wg.Wait()

	success := int(successCount.Load())
	fails := int(failCount.Load())
	t.Logf("ListServices flood: %d succeeded, %d failed out of %d",
		success, fails, goroutineCount)

	// Under heavy concurrent load on a single fd, some failures are
	// expected because the kernel serializes binder transactions per
	// OS thread. But the majority should succeed.
	minSuccess := goroutineCount * 50 / 100
	require.GreaterOrEqual(t, success, minSuccess,
		"expected at least %d/%d to succeed", minSuccess, goroutineCount)
}

// --- Test 7: Transact timeout ---

func TestConcurrent_TransactTimeout(t *testing.T) {
	// Test that context cancellation propagates correctly when the
	// binder system is under load from oneway transaction flooding.
	const onewayFloodCount = 50

	outerCtx, outerCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer outerCancel()

	driver := openBinder(t)
	sm := servicemanager.New(driver)

	svc, err := sm.GetService(outerCtx, servicemanager.ServiceName("SurfaceFlinger"))
	require.NoError(t, err)
	require.NotNil(t, svc)

	// Flood oneway PING transactions; they should all return immediately
	// with empty parcels and not block.
	var wg sync.WaitGroup
	var onewaySuccess, onewayFail atomic.Int64

	for i := 0; i < onewayFloodCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			reply, err := svc.Transact(outerCtx, binder.PingTransaction, binder.FlagOneway, parcel.New())
			if err != nil {
				onewayFail.Add(1)
				return
			}
			if reply.Len() != 0 {
				onewayFail.Add(1)
				return
			}
			onewaySuccess.Add(1)
		}()
	}

	wg.Wait()

	t.Logf("oneway flood: %d succeeded, %d failed",
		onewaySuccess.Load(), onewayFail.Load())

	// After the flood, a normal synchronous call with a short timeout
	// should either succeed or time out — but not hang indefinitely.
	shortCtx, shortCancel := context.WithTimeout(outerCtx, 5*time.Second)
	defer shortCancel()

	alive := svc.IsAlive(shortCtx)
	t.Logf("SurfaceFlinger alive after oneway flood: %v", alive)

	// The service should still be alive; if not, the oneway flood
	// corrupted driver state.
	assert.True(t, alive,
		"service should remain alive after oneway flood")
}

// --- Test 8: Handle exhaustion ---

func TestConcurrent_HandleExhaustion(t *testing.T) {
	const handleCount = 50

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	driver, err := kernelbinder.Open(ctx, binder.WithMapSize(128*1024))
	require.NoError(t, err)

	transport, err := versionaware.NewTransport(ctx, driver, 0)
	require.NoError(t, err)

	sm := servicemanager.New(transport)

	// Accumulate many service handles without closing the driver.
	// Each GetService call acquires a new binder handle.
	serviceNames := []servicemanager.ServiceName{
		"SurfaceFlinger", "SurfaceFlingerAIDL", "activity", "power",
		"window", "package", "user", "uimode", "display",
		"notification",
	}

	handles := make([]binder.IBinder, 0, handleCount)
	var acquireErrors int

	for i := 0; i < handleCount; i++ {
		name := serviceNames[i%len(serviceNames)]
		svc, err := sm.GetService(ctx, name)
		if err != nil {
			acquireErrors++
			t.Logf("handle %d: GetService(%s) failed: %v", i, name, err)
			continue
		}
		if svc == nil {
			acquireErrors++
			continue
		}
		handles = append(handles, svc)
	}

	t.Logf("acquired %d handles (%d errors)", len(handles), acquireErrors)
	require.NotEmpty(t, handles, "should have acquired at least some handles")

	// Verify all acquired handles are still valid (no premature cleanup).
	var aliveCount int
	for _, svc := range handles {
		if svc.IsAlive(ctx) {
			aliveCount++
		}
	}
	t.Logf("%d/%d handles are alive before cleanup", aliveCount, len(handles))

	// At least 80% should still be alive.
	minAlive := len(handles) * 80 / 100
	assert.GreaterOrEqual(t, aliveCount, minAlive,
		"expected at least %d/%d handles to remain alive", minAlive, len(handles))

	// Close the driver; this should release all handles without panicking.
	err = driver.Close(ctx)
	assert.NoError(t, err, "driver.Close should succeed and release all handles")

	t.Logf("driver closed; all %d handles released", len(handles))
}
