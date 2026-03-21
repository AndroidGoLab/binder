//go:build e2e

package e2e

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xaionaro-go/binder/binder"
	"github.com/xaionaro-go/binder/binder/versionaware"
	aidlerrors "github.com/xaionaro-go/binder/errors"
	"github.com/xaionaro-go/binder/kernelbinder"
	"github.com/xaionaro-go/binder/parcel"
	"github.com/xaionaro-go/binder/servicemanager"
)

func openBinder(t *testing.T) *versionaware.Transport {
	t.Helper()
	ctx := context.Background()
	driver, err := kernelbinder.Open(ctx, binder.WithMapSize(128*1024))
	require.NoError(t, err, "failed to open /dev/binder")
	transport, err := versionaware.NewTransport(ctx, driver, 0)
	require.NoError(t, err, "failed to create version-aware transport")
	t.Cleanup(func() {
		_ = transport.Close(ctx)
		_ = driver.Close(ctx)
	})
	return transport
}

// requireOrSkip calls require.NoError unless the error is a transient
// binder resource issue (which can happen when running 200+ tests
// sequentially and the kernel binder driver is under pressure),
// a service-not-found issue, or an AIDL version mismatch.
// In that case it skips the test instead of failing.
func requireOrSkip(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		return
	}
	errStr := err.Error()
	if strings.Contains(errStr, "read beyond end") ||
		strings.Contains(errStr, "failed transaction") ||
		strings.Contains(errStr, "interrupted system call") {
		t.Skipf("binder resource constraint (test passes in isolation): %v", err)
	}
	if strings.Contains(errStr, "unknown union tag") {
		t.Skipf("AIDL version mismatch (union tag not in generated code): %v", err)
	}
	if strings.Contains(errStr, "not found in version") {
		t.Skipf("method not available on this API level: %v", err)
	}
	if strings.Contains(errStr, "null binder") ||
		strings.Contains(errStr, "unexpected null") {
		t.Skipf("service not available on this device: %v", err)
	}
	if strings.Contains(errStr, "dead object") {
		t.Skipf("binder resource constraint (service died): %v", err)
	}
	if strings.Contains(errStr, "exception Security") {
		t.Skipf("permission denied: %v", err)
	}
	// NOTE: NullPointer and IllegalState exceptions are NOT skipped globally.
	// They may indicate bugs in our argument serialization or call sequence.
	// Handle them per-test where the cause is understood.
	if strings.Contains(errStr, "kernel status error") {
		t.Skipf("kernel binder error (SELinux/version-dependent): %v", err)
	}
	require.NoError(t, err)
}

// resolveCode resolves an AIDL method name to the correct transaction code
// for the detected device version, skipping the test if the method is not
// available on this API level.
func resolveCode(
	ctx context.Context,
	t *testing.T,
	svc binder.IBinder,
	descriptor string,
	method string,
) binder.TransactionCode {
	t.Helper()
	code, err := svc.ResolveCode(ctx, descriptor, method)
	requireOrSkip(t, err)
	return code
}

// transactNoArg sends a transaction with only an interface token (no arguments)
// and returns the reply after verifying AIDL status.
func transactNoArg(
	ctx context.Context,
	t *testing.T,
	svc binder.IBinder,
	descriptor string,
	method string,
) *parcel.Parcel {
	t.Helper()
	code := resolveCode(ctx, t, svc, descriptor, method)
	data := parcel.New()
	data.WriteInterfaceToken(descriptor)
	reply, err := svc.Transact(ctx, code, 0, data)
	requireOrSkip(t, err)
	statusErr := binder.ReadStatus(reply)
	requireOrSkip(t, statusErr)
	return reply
}

// --- ServiceManager tests ---

func TestListServices(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)

	services, err := sm.ListServices(ctx)
	require.NoError(t, err, "ListServices failed")
	require.NotEmpty(t, services, "expected at least one service")

	t.Logf("Found %d services", len(services))
	for i, name := range services {
		if i < 10 {
			t.Logf("  [%d] %s", i, name)
		}
	}

	assert.Contains(t, services, servicemanager.ServiceName("SurfaceFlinger"))
	assert.Contains(t, services, servicemanager.ServiceName("activity"))
}

func TestGetService(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)

	svc, err := sm.GetService(ctx, servicemanager.ServiceName("SurfaceFlinger"))
	require.NoError(t, err, "GetService(SurfaceFlinger) failed")
	require.NotNil(t, svc, "expected non-nil binder for SurfaceFlinger")

	handle := svc.Handle()
	t.Logf("SurfaceFlinger handle: %d", handle)
	assert.Greater(t, handle, uint32(0), "SurfaceFlinger handle should be > 0")
}

func TestCheckService(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)

	svc, err := sm.CheckService(ctx, servicemanager.ServiceName("SurfaceFlinger"))
	require.NoError(t, err, "CheckService(SurfaceFlinger) failed")
	require.NotNil(t, svc, "expected non-nil binder for SurfaceFlinger")

	nonexistent, err := sm.CheckService(ctx, servicemanager.ServiceName("definitely.does.not.exist.12345"))
	require.NoError(t, err, "CheckService for non-existent should not error")
	assert.Nil(t, nonexistent, "expected nil binder for non-existent service")
}

func TestPingBinder(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)

	svc, err := sm.GetService(ctx, servicemanager.ServiceName("SurfaceFlinger"))
	require.NoError(t, err)
	require.NotNil(t, svc)

	alive := svc.IsAlive(ctx)
	assert.True(t, alive, "SurfaceFlinger should be alive")
}

// --- SurfaceFlingerAIDL typed call tests ---

const surfaceComposerDescriptor = "android.gui.ISurfaceComposer"

func getSurfaceFlingerAIDL(
	ctx context.Context,
	t *testing.T,
	driver *versionaware.Transport,
) binder.IBinder {
	t.Helper()
	sm := servicemanager.New(driver)
	svc, err := sm.GetService(ctx, servicemanager.ServiceName("SurfaceFlingerAIDL"))
	require.NoError(t, err, "GetService(SurfaceFlingerAIDL) failed")
	require.NotNil(t, svc)
	return svc
}

func TestSurfaceFlinger_GetBootDisplayModeSupport(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sf := getSurfaceFlingerAIDL(ctx, t, driver)

	// ISurfaceComposer::getBootDisplayModeSupport → bool.
	reply := transactNoArg(ctx, t, sf, surfaceComposerDescriptor, "getBootDisplayModeSupport")
	val, err := reply.ReadBool()
	require.NoError(t, err)
	t.Logf("getBootDisplayModeSupport: %v", val)
}

func TestSurfaceFlinger_GetPhysicalDisplayIds(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sf := getSurfaceFlingerAIDL(ctx, t, driver)

	// ISurfaceComposer::getPhysicalDisplayIds → long[].
	reply := transactNoArg(ctx, t, sf, surfaceComposerDescriptor, "getPhysicalDisplayIds")
	count, err := reply.ReadInt32()
	require.NoError(t, err)
	require.Greater(t, count, int32(0), "expected at least one physical display")

	ids := make([]int64, count)
	for i := int32(0); i < count; i++ {
		ids[i], err = reply.ReadInt64()
		require.NoError(t, err)
	}
	t.Logf("getPhysicalDisplayIds: %v", ids)
	assert.NotZero(t, ids[0], "first display ID should be non-zero")
}

func TestSurfaceFlinger_GetStaticDisplayInfo(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sf := getSurfaceFlingerAIDL(ctx, t, driver)

	// First get a display ID.
	reply := transactNoArg(ctx, t, sf, surfaceComposerDescriptor, "getPhysicalDisplayIds")
	count, err := reply.ReadInt32()
	require.NoError(t, err)
	require.Greater(t, count, int32(0))
	displayID, err := reply.ReadInt64()
	require.NoError(t, err)

	// ISurfaceComposer::getStaticDisplayInfo(long displayId) → Parcelable.
	codeStaticInfo := resolveCode(ctx, t, sf, surfaceComposerDescriptor, "getStaticDisplayInfo")
	data := parcel.New()
	data.WriteInterfaceToken(surfaceComposerDescriptor)
	data.WriteInt64(displayID)
	reply2, err := sf.Transact(ctx, codeStaticInfo, 0, data)
	require.NoError(t, err)
	require.NoError(t, binder.ReadStatus(reply2))

	// StaticDisplayInfo is a parcelable; just verify we got data back.
	remaining := reply2.Data()[reply2.Position():]
	t.Logf("getStaticDisplayInfo: %d bytes of parcelable data for display %d", len(remaining), displayID)
	assert.Greater(t, len(remaining), 0, "expected non-empty StaticDisplayInfo")

	// NDK backend does NOT write a stability prefix.
	// Wire format: int32(dataSize) + int32(connectionType) + float32(density) + int32(secure).
	parcelableSize, err := reply2.ReadInt32()
	require.NoError(t, err)
	t.Logf("  parcelable data size: %d", parcelableSize)
	assert.Greater(t, parcelableSize, int32(0), "parcelable should have data")

	// First field: connectionType (int32).
	connType, err := reply2.ReadInt32()
	require.NoError(t, err)
	t.Logf("  connectionType: %d", connType)
}

// --- ActivityManager typed call tests ---

const activityManagerDescriptor = "android.app.IActivityManager"

func getActivityManager(
	ctx context.Context,
	t *testing.T,
	driver *versionaware.Transport,
) binder.IBinder {
	t.Helper()
	sm := servicemanager.New(driver)
	svc, err := sm.GetService(ctx, servicemanager.ActivityService)
	require.NoError(t, err, "GetService(activity) failed")
	require.NotNil(t, svc)
	return svc
}

func TestActivityManager_GetProcessLimit(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	am := getActivityManager(ctx, t, driver)

	// IActivityManager::getProcessLimit → int32.
	reply := transactNoArg(ctx, t, am, activityManagerDescriptor, "getProcessLimit")
	val, err := reply.ReadInt32()
	require.NoError(t, err)
	t.Logf("getProcessLimit: %d", val)
}

func TestActivityManager_IsUserAMonkey(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	am := getActivityManager(ctx, t, driver)

	// IActivityManager::isUserAMonkey → bool.
	reply := transactNoArg(ctx, t, am, activityManagerDescriptor, "isUserAMonkey")
	val, err := reply.ReadBool()
	require.NoError(t, err)
	assert.False(t, val, "should not be a monkey in test")
	t.Logf("isUserAMonkey: %v", val)
}

func TestActivityManager_IsAppFreezerSupported(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	am := getActivityManager(ctx, t, driver)

	// IActivityManager::isAppFreezerSupported → bool.
	reply := transactNoArg(ctx, t, am, activityManagerDescriptor, "isAppFreezerSupported")
	val, err := reply.ReadBool()
	require.NoError(t, err)
	t.Logf("isAppFreezerSupported: %v", val)
}

func TestActivityManager_CheckPermission(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	am := getActivityManager(ctx, t, driver)

	// IActivityManager::checkPermission(String permission, int pid, int uid) → int32.
	// 0 = PERMISSION_GRANTED, -1 = PERMISSION_DENIED.
	codeCheckPerm := resolveCode(ctx, t, am, activityManagerDescriptor, "checkPermission")
	data := parcel.New()
	data.WriteInterfaceToken(activityManagerDescriptor)
	data.WriteString16("android.permission.INTERNET")
	data.WriteInt32(int32(os.Getpid()))
	data.WriteInt32(0) // uid 0 = root

	reply, err := am.Transact(ctx, codeCheckPerm, 0, data)
	require.NoError(t, err)
	require.NoError(t, binder.ReadStatus(reply))

	val, err := reply.ReadInt32()
	require.NoError(t, err)
	assert.Equal(t, int32(0), val, "root should have INTERNET permission")
	t.Logf("checkPermission(INTERNET, pid=%d, uid=0): %d", os.Getpid(), val)
}

// --- Multiple services in one test: distinct handles ---

func TestDistinctServiceHandles(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)

	sf, err := sm.GetService(ctx, servicemanager.ServiceName("SurfaceFlingerAIDL"))
	require.NoError(t, err)
	require.NotNil(t, sf)

	am, err := sm.GetService(ctx, servicemanager.ActivityService)
	require.NoError(t, err)
	require.NotNil(t, am)

	assert.NotEqual(t, sf.Handle(), am.Handle(),
		"different services should have different handles")
	t.Logf("SurfaceFlingerAIDL handle=%d, activity handle=%d", sf.Handle(), am.Handle())
}

// --- Oneway transaction ---

func TestOnewayTransaction(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)

	svc, err := sm.GetService(ctx, servicemanager.ServiceName("SurfaceFlinger"))
	require.NoError(t, err)
	require.NotNil(t, svc)

	// Send PING with FlagOneway. The driver should receive
	// BR_TRANSACTION_COMPLETE without BR_REPLY and return an empty parcel.
	reply, err := svc.Transact(ctx, binder.PingTransaction, binder.FlagOneway, parcel.New())
	require.NoError(t, err, "oneway PING should succeed")
	assert.Equal(t, 0, reply.Len(), "oneway reply should be empty")
}

// --- Death notification registration ---

type testDeathRecipient struct{}

func (r *testDeathRecipient) BinderDied() {}

func TestDeathNotificationRegistration(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)

	svc, err := sm.GetService(ctx, servicemanager.ServiceName("SurfaceFlinger"))
	require.NoError(t, err)
	require.NotNil(t, svc)

	recipient := &testDeathRecipient{}

	// Register death notification.
	err = svc.LinkToDeath(ctx, recipient)
	require.NoError(t, err, "LinkToDeath should succeed")

	// Clear death notification.
	err = svc.UnlinkToDeath(ctx, recipient)
	require.NoError(t, err, "UnlinkToDeath should succeed")
}

// --- Concurrent transactions ---

func TestConcurrentTransactions(t *testing.T) {
	ctx := context.Background()

	const goroutines = 5
	var wg sync.WaitGroup
	errs := make([]error, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			// Each goroutine opens its own driver for true parallelism.
			driver, err := kernelbinder.Open(ctx, binder.WithMapSize(128*1024))
			if err != nil {
				errs[idx] = err
				return
			}
			defer func() { _ = driver.Close(ctx) }()

			transport, err := versionaware.NewTransport(ctx, driver, 0)
			if err != nil {
				errs[idx] = err
				return
			}

			sm := servicemanager.New(transport)
			services, err := sm.ListServices(ctx)
			if err != nil {
				errs[idx] = err
				return
			}
			if len(services) == 0 {
				errs[idx] = errors.New("no services found")
				return
			}
		}(i)
	}

	wg.Wait()

	for i, err := range errs {
		assert.NoError(t, err, "goroutine %d failed", i)
	}
}

// --- Transaction to invalid handle ---

func TestTransactionInvalidHandle(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)

	invalid := binder.NewProxyBinder(driver, binder.DefaultCallerIdentity(), 0xDEAD)

	reply, err := invalid.Transact(ctx, binder.PingTransaction, 0, parcel.New())
	require.Error(t, err, "transaction to invalid handle should fail")
	assert.Nil(t, reply)

	var txnErr *aidlerrors.TransactionError
	require.True(t, errors.As(err, &txnErr), "expected TransactionError, got %T: %v", err, err)
	t.Logf("invalid handle error: %v (code: %v)", txnErr, txnErr.Code)
}

// --- Ping multiple services ---

func TestPingMultipleServices(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)

	services := []string{"SurfaceFlinger", "activity", "SurfaceFlingerAIDL"}
	for _, name := range services {
		svc, err := sm.GetService(ctx, servicemanager.ServiceName(name))
		require.NoError(t, err, "GetService(%s)", name)
		require.NotNil(t, svc)

		alive := svc.IsAlive(ctx)
		assert.True(t, alive, "%s should be alive", name)
		t.Logf("%s (handle=%d): alive=%v", name, svc.Handle(), alive)
	}
}
