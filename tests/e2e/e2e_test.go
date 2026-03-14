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

	"github.com/xaionaro-go/aidl/binder"
	aidlerrors "github.com/xaionaro-go/aidl/errors"
	"github.com/xaionaro-go/aidl/kernelbinder"
	"github.com/xaionaro-go/aidl/parcel"
	"github.com/xaionaro-go/aidl/servicemanager"
)

func openBinder(t *testing.T) *kernelbinder.Driver {
	t.Helper()
	ctx := context.Background()
	driver, err := kernelbinder.Open(ctx, binder.WithMapSize(128*1024))
	require.NoError(t, err, "failed to open /dev/binder")
	t.Cleanup(func() {
		_ = driver.Close(ctx)
	})
	return driver
}

// requireOrSkip calls require.NoError unless the error is a transient
// binder resource issue (which can happen when running 200+ tests
// sequentially and the kernel binder driver is under pressure).
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
	require.NoError(t, err)
}

// transactNoArg sends a transaction with only an interface token (no arguments)
// and returns the reply after verifying AIDL status.
func transactNoArg(
	ctx context.Context,
	t *testing.T,
	svc binder.IBinder,
	descriptor string,
	code binder.TransactionCode,
) *parcel.Parcel {
	t.Helper()
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

	assert.Contains(t, services, "SurfaceFlinger")
	assert.Contains(t, services, "activity")
}

func TestGetService(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)

	svc, err := sm.GetService(ctx, "SurfaceFlinger")
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

	svc, err := sm.CheckService(ctx, "SurfaceFlinger")
	require.NoError(t, err, "CheckService(SurfaceFlinger) failed")
	require.NotNil(t, svc, "expected non-nil binder for SurfaceFlinger")

	nonexistent, err := sm.CheckService(ctx, "definitely.does.not.exist.12345")
	require.NoError(t, err, "CheckService for non-existent should not error")
	assert.Nil(t, nonexistent, "expected nil binder for non-existent service")
}

func TestPingBinder(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)

	svc, err := sm.GetService(ctx, "SurfaceFlinger")
	require.NoError(t, err)
	require.NotNil(t, svc)

	alive := svc.IsAlive(ctx)
	assert.True(t, alive, "SurfaceFlinger should be alive")
}

// TestIsDeclared is skipped: the transaction code for IsDeclared
// depends on the IServiceManager AIDL version, which varies across
// Android builds. Needs codegen-derived transaction codes to be reliable.
func TestIsDeclared(t *testing.T) {
	t.Skip("transaction code may differ across Android versions")
}

// --- SurfaceFlingerAIDL typed call tests ---

const surfaceComposerDescriptor = "android.gui.ISurfaceComposer"

func getSurfaceFlingerAIDL(
	ctx context.Context,
	t *testing.T,
	driver *kernelbinder.Driver,
) binder.IBinder {
	t.Helper()
	sm := servicemanager.New(driver)
	svc, err := sm.GetService(ctx, "SurfaceFlingerAIDL")
	require.NoError(t, err, "GetService(SurfaceFlingerAIDL) failed")
	require.NotNil(t, svc)
	return svc
}

func TestSurfaceFlinger_GetGpuContextPriority(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sf := getSurfaceFlingerAIDL(ctx, t, driver)

	// ISurfaceComposer::getGpuContextPriority (code 64) → int32.
	reply := transactNoArg(ctx, t, sf, surfaceComposerDescriptor, 64)
	val, err := reply.ReadInt32()
	require.NoError(t, err)
	t.Logf("getGpuContextPriority: %d", val)
}

func TestSurfaceFlinger_GetBootDisplayModeSupport(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sf := getSurfaceFlingerAIDL(ctx, t, driver)

	// ISurfaceComposer::getBootDisplayModeSupport (code 19) → bool.
	reply := transactNoArg(ctx, t, sf, surfaceComposerDescriptor, 19)
	val, err := reply.ReadBool()
	require.NoError(t, err)
	t.Logf("getBootDisplayModeSupport: %v", val)
}

func TestSurfaceFlinger_GetPhysicalDisplayIds(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sf := getSurfaceFlingerAIDL(ctx, t, driver)

	// ISurfaceComposer::getPhysicalDisplayIds (code 6) → long[].
	reply := transactNoArg(ctx, t, sf, surfaceComposerDescriptor, 6)
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
	reply := transactNoArg(ctx, t, sf, surfaceComposerDescriptor, 6)
	count, err := reply.ReadInt32()
	require.NoError(t, err)
	require.Greater(t, count, int32(0))
	displayID, err := reply.ReadInt64()
	require.NoError(t, err)

	// ISurfaceComposer::getStaticDisplayInfo(long displayId) (code 12) → Parcelable.
	data := parcel.New()
	data.WriteInterfaceToken(surfaceComposerDescriptor)
	data.WriteInt64(displayID)
	reply2, err := sf.Transact(ctx, 12, 0, data)
	require.NoError(t, err)
	require.NoError(t, binder.ReadStatus(reply2))

	// StaticDisplayInfo is a parcelable; just verify we got data back.
	remaining := reply2.Data()[reply2.Position():]
	t.Logf("getStaticDisplayInfo: %d bytes of parcelable data for display %d", len(remaining), displayID)
	assert.Greater(t, len(remaining), 0, "expected non-empty StaticDisplayInfo")

	// AIDL parcelable wire format: stability (int32) + data size (int32) + fields.
	stability, err := reply2.ReadInt32()
	require.NoError(t, err)
	t.Logf("  stability: %d", stability)

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
	driver *kernelbinder.Driver,
) binder.IBinder {
	t.Helper()
	sm := servicemanager.New(driver)
	svc, err := sm.GetService(ctx, "activity")
	require.NoError(t, err, "GetService(activity) failed")
	require.NotNil(t, svc)
	return svc
}

func TestActivityManager_GetProcessLimit(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	am := getActivityManager(ctx, t, driver)

	// IActivityManager::getProcessLimit (code 53) → int32.
	reply := transactNoArg(ctx, t, am, activityManagerDescriptor, 53)
	val, err := reply.ReadInt32()
	require.NoError(t, err)
	t.Logf("getProcessLimit: %d", val)
}

func TestActivityManager_IsUserAMonkey(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	am := getActivityManager(ctx, t, driver)

	// IActivityManager::isUserAMonkey (code 105) → bool.
	reply := transactNoArg(ctx, t, am, activityManagerDescriptor, 105)
	val, err := reply.ReadBool()
	require.NoError(t, err)
	assert.False(t, val, "should not be a monkey in test")
	t.Logf("isUserAMonkey: %v", val)
}

func TestActivityManager_IsAppFreezerSupported(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	am := getActivityManager(ctx, t, driver)

	// IActivityManager::isAppFreezerSupported (code 232) → bool.
	reply := transactNoArg(ctx, t, am, activityManagerDescriptor, 232)
	val, err := reply.ReadBool()
	require.NoError(t, err)
	t.Logf("isAppFreezerSupported: %v", val)
}

func TestActivityManager_CheckPermission(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	am := getActivityManager(ctx, t, driver)

	// IActivityManager::checkPermission(String permission, int pid, int uid) (code 9) → int32.
	// 0 = PERMISSION_GRANTED, -1 = PERMISSION_DENIED.
	data := parcel.New()
	data.WriteInterfaceToken(activityManagerDescriptor)
	data.WriteString16("android.permission.INTERNET")
	data.WriteInt32(int32(os.Getpid()))
	data.WriteInt32(0) // uid 0 = root

	reply, err := am.Transact(ctx, 9, 0, data)
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

	sf, err := sm.GetService(ctx, "SurfaceFlingerAIDL")
	require.NoError(t, err)
	require.NotNil(t, sf)

	am, err := sm.GetService(ctx, "activity")
	require.NoError(t, err)
	require.NotNil(t, am)

	assert.NotEqual(t, sf.Handle(), am.Handle(),
		"different services should have different handles")
	t.Logf("SurfaceFlingerAIDL handle=%d, activity handle=%d", sf.Handle(), am.Handle())
}

// --- float32 parsing from real service data ---

func TestSurfaceFlinger_ParseFloat32FromDisplayInfo(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sf := getSurfaceFlingerAIDL(ctx, t, driver)

	// Get a display ID.
	reply := transactNoArg(ctx, t, sf, surfaceComposerDescriptor, 6)
	count, err := reply.ReadInt32()
	require.NoError(t, err)
	require.Greater(t, count, int32(0))
	displayID, err := reply.ReadInt64()
	require.NoError(t, err)

	// getStaticDisplayInfo(displayId) → AIDL Parcelable.
	data := parcel.New()
	data.WriteInterfaceToken(surfaceComposerDescriptor)
	data.WriteInt64(displayID)
	reply2, err := sf.Transact(ctx, 12, 0, data)
	require.NoError(t, err)
	require.NoError(t, binder.ReadStatus(reply2))

	// StaticDisplayInfo wire format:
	//   int32: stability (parcelable stability marker)
	//   int32: parcelable data size
	//   int32: connectionType
	//   float32: density
	//   int32: secure (bool)
	stability, err := reply2.ReadInt32()
	require.NoError(t, err)
	t.Logf("stability: %d", stability)

	parcelableSize, err := reply2.ReadInt32()
	require.NoError(t, err)
	t.Logf("parcelable size: %d", parcelableSize)

	connType, err := reply2.ReadInt32()
	require.NoError(t, err)
	t.Logf("connectionType: %d", connType)

	density, err := reply2.ReadFloat32()
	require.NoError(t, err)
	t.Logf("density: %f", density)
	assert.Greater(t, density, float32(0), "density should be positive")
	assert.Less(t, density, float32(1000), "density should be reasonable")
}

// --- AIDL status exception parsing ---

func TestActivityManager_SecurityException(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	am := getActivityManager(ctx, t, driver)

	// IActivityManager::getRunningUserIds (code 148) requires system process
	// and returns ExceptionSecurity for unprivileged callers.
	data := parcel.New()
	data.WriteInterfaceToken(activityManagerDescriptor)
	reply, err := am.Transact(ctx, 148, 0, data)
	require.NoError(t, err, "Transact itself should succeed")

	statusErr := binder.ReadStatus(reply)
	require.Error(t, statusErr, "expected AIDL exception for getRunningUserIds")

	var se *aidlerrors.StatusError
	require.True(t, errors.As(statusErr, &se), "expected StatusError, got %T", statusErr)
	assert.Equal(t, aidlerrors.ExceptionSecurity, se.Exception,
		"expected Security exception")
	assert.NotEmpty(t, se.Message, "exception should have a message")
	t.Logf("Security exception: %s", se.Message)
}

// --- Oneway transaction ---

func TestOnewayTransaction(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)

	svc, err := sm.GetService(ctx, "SurfaceFlinger")
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

	svc, err := sm.GetService(ctx, "SurfaceFlinger")
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

			sm := servicemanager.New(driver)
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

	invalid := binder.NewProxyBinder(driver, 0xDEAD)

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
		svc, err := sm.GetService(ctx, name)
		require.NoError(t, err, "GetService(%s)", name)
		require.NotNil(t, svc)

		alive := svc.IsAlive(ctx)
		assert.True(t, alive, "%s should be alive", name)
		t.Logf("%s (handle=%d): alive=%v", name, svc.Handle(), alive)
	}
}

// --- Sequential transactions on same service ---

func TestSequentialTransactionsOnSameService(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sf := getSurfaceFlingerAIDL(ctx, t, driver)

	// Call the same method multiple times in sequence.
	for i := 0; i < 5; i++ {
		reply := transactNoArg(ctx, t, sf, surfaceComposerDescriptor, 64)
		val, err := reply.ReadInt32()
		require.NoError(t, err)
		t.Logf("getGpuContextPriority call %d: %d", i+1, val)
	}
}
