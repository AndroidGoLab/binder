//go:build e2e

package e2e

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xaionaro-go/binder/binder"
	"github.com/xaionaro-go/binder/parcel"
)

// --- int[] return type ---

func TestType_IntArray(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "display")

	const descriptor = "android.hardware.display.IDisplayManager"

	// IDisplayManager::getDisplayIds(false) -> int[].
	codeGetDisplayIds := resolveCode(ctx, t, svc, descriptor, "getDisplayIds")
	data := parcel.New()
	data.WriteInterfaceToken(descriptor)
	data.WriteBool(false)

	reply, err := svc.Transact(ctx, codeGetDisplayIds, 0, data)
	requireOrSkip(t, err)
	requireOrSkip(t, binder.ReadStatus(reply))

	count, err := reply.ReadInt32()
	require.NoError(t, err, "failed to read array count")
	require.Greater(t, count, int32(0), "expected at least one display ID")

	ids := make([]int32, count)
	for i := int32(0); i < count; i++ {
		ids[i], err = reply.ReadInt32()
		require.NoError(t, err, "failed to read display ID at index %d", i)
	}

	t.Logf("display IDs (int[]): %v", ids)
	assert.Equal(t, int32(0), ids[0], "default display ID should be 0")
}

// --- long[] return type ---

func TestType_LongArray(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sf := getSurfaceFlingerAIDL(ctx, t, driver)

	// ISurfaceComposer::getPhysicalDisplayIds -> long[].
	reply := transactNoArg(ctx, t, sf, surfaceComposerDescriptor, "getPhysicalDisplayIds")

	count, err := reply.ReadInt32()
	require.NoError(t, err, "failed to read array count")
	require.GreaterOrEqual(t, count, int32(1), "expected at least one physical display")

	ids := make([]int64, count)
	for i := int32(0); i < count; i++ {
		ids[i], err = reply.ReadInt64()
		require.NoError(t, err, "failed to read display ID at index %d", i)
	}

	t.Logf("physical display IDs (long[]): %v", ids)
	for i, id := range ids {
		assert.NotZero(t, id, "display ID at index %d should be non-zero", i)
	}
}

// --- Nullable parcelable (null case) ---

func TestType_NullableParcelable(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "alarm")

	const descriptor = "android.app.IAlarmManager"

	// IAlarmManager::getNextAlarmClock -> nullable AlarmClockInfo.
	codeNextAlarm := resolveCode(ctx, t, svc, descriptor, "getNextAlarmClock")
	data := parcel.New()
	data.WriteInterfaceToken(descriptor)
	data.WriteInt32(0) // userId

	reply, err := svc.Transact(ctx, codeNextAlarm, 0, data)
	requireOrSkip(t, err)
	requireOrSkip(t, binder.ReadStatus(reply))

	flag, err := reply.ReadInt32()
	require.NoError(t, err, "failed to read nullable flag")

	if flag == 0 {
		t.Logf("getNextAlarmClock: null (no alarm set)")
	} else {
		t.Logf("getNextAlarmClock: present (flag=%d)", flag)
	}
}

// --- Enum (int-backed) return ---

func TestType_EnumValue(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "uimode")

	const descriptor = "android.app.IUiModeManager"

	// IUiModeManager::getCurrentModeType -> int32 (UI_MODE_TYPE enum).
	reply := transactNoArg(ctx, t, svc, descriptor, "getCurrentModeType")

	val, err := reply.ReadInt32()
	require.NoError(t, err, "failed to read enum value")
	t.Logf("getCurrentModeType: %d", val)

	// Known values: 0=UNDEFINED/NORMAL, 1=NORMAL, 2=CAR, 3=TV, 4=WATCH, 5=APPLIANCE, 6=DESK, 7=VR.
	// Android Configuration.UI_MODE_TYPE_NORMAL is 1, but some ROMs (e.g. GrapheneOS) return 0.
	assert.GreaterOrEqual(t, val, int32(0), "mode type should be a non-negative enum value")
}

// --- Large parcelable ---

func TestType_LargeParcelable(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "display")

	const descriptor = "android.hardware.display.IDisplayManager"

	// IDisplayManager::getDisplayInfo(0) -> DisplayInfo parcelable.
	codeGetDisplayInfo := resolveCode(ctx, t, svc, descriptor, "getDisplayInfo")
	data := parcel.New()
	data.WriteInterfaceToken(descriptor)
	data.WriteInt32(0) // display 0

	reply, err := svc.Transact(ctx, codeGetDisplayInfo, 0, data)
	requireOrSkip(t, err)
	requireOrSkip(t, binder.ReadStatus(reply))

	// Nullable parcelable: int32 flag.
	flag, err := reply.ReadInt32()
	require.NoError(t, err, "failed to read nullable flag")
	require.Equal(t, int32(1), flag, "expected non-null DisplayInfo")

	// AIDL parcelable envelope: stability + size.
	stability, err := reply.ReadInt32()
	require.NoError(t, err, "failed to read stability")
	t.Logf("DisplayInfo stability: %d", stability)

	parcelableSize, err := reply.ReadInt32()
	require.NoError(t, err, "failed to read parcelable size")
	t.Logf("DisplayInfo parcelable size: %d bytes", parcelableSize)

	assert.Greater(t, parcelableSize, int32(100),
		"DisplayInfo should be a large parcelable (confirmed 880 bytes on emulator)")
}

// --- Multiple String16 parameters ---

func TestType_MultipleStringParams(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "appops")

	const descriptor = "com.android.internal.app.IAppOpsService"

	// IAppOpsService::checkOperation(int op, int uid, String packageName) -> int32.
	codeCheckOp := resolveCode(ctx, t, svc, descriptor, "checkOperation")
	data := parcel.New()
	data.WriteInterfaceToken(descriptor)
	data.WriteInt32(24) // OP_SYSTEM_ALERT_WINDOW
	data.WriteInt32(0)  // uid 0 (root)
	data.WriteString16("com.android.shell")

	reply, err := svc.Transact(ctx, codeCheckOp, 0, data)
	requireOrSkip(t, err)
	requireOrSkip(t, binder.ReadStatus(reply))

	mode, err := reply.ReadInt32()
	require.NoError(t, err, "failed to read operation mode")
	t.Logf("checkOperation(OP_SYSTEM_ALERT_WINDOW, uid=0, com.android.shell): mode=%d", mode)

	// Valid modes: 0=ALLOWED, 1=IGNORED, 2=ERRORED, 3=DEFAULT, 4=FOREGROUND.
	assert.GreaterOrEqual(t, mode, int32(0), "mode should be >= 0")
	assert.LessOrEqual(t, mode, int32(4), "mode should be <= 4")
}
