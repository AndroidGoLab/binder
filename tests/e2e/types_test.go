//go:build e2e

package e2e

import (
	"context"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xaionaro-go/aidl/binder"
	"github.com/xaionaro-go/aidl/parcel"
)

// --- int[] return type ---

func TestType_IntArray(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "display")

	const descriptor = "android.hardware.display.IDisplayManager"

	// IDisplayManager::getDisplayIds(false) (code 2) -> int[].
	data := parcel.New()
	data.WriteInterfaceToken(descriptor)
	data.WriteBool(false)

	reply, err := svc.Transact(ctx, 2, 0, data)
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

// --- int[] from vibrator_manager ---

func TestType_IntArray_Vibrator(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "vibrator_manager")

	const descriptor = "android.os.IVibratorManagerService"

	// IVibratorManagerService::getVibratorIds (code 1) -> int[].
	reply := transactNoArg(ctx, t, svc, descriptor, 1)

	count, err := reply.ReadInt32()
	require.NoError(t, err, "failed to read array count")
	require.GreaterOrEqual(t, count, int32(1), "expected at least one vibrator ID")

	ids := make([]int32, count)
	for i := int32(0); i < count; i++ {
		ids[i], err = reply.ReadInt32()
		require.NoError(t, err, "failed to read vibrator ID at index %d", i)
	}

	t.Logf("vibrator IDs (int[]): %v", ids)
}

// --- long[] return type ---

func TestType_LongArray(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sf := getSurfaceFlingerAIDL(ctx, t, driver)

	// ISurfaceComposer::getPhysicalDisplayIds (code 6) -> long[].
	reply := transactNoArg(ctx, t, sf, surfaceComposerDescriptor, 6)

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

// --- String16 return value ---

func TestType_String16Return(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "country_detector")

	const descriptor = "android.location.ICountryDetector"

	// ICountryDetector::detectCountry (code 1) -> Country parcelable.
	reply := transactNoArg(ctx, t, svc, descriptor, 1)

	// Reply is a nullable parcelable: int32 flag (1=present, 0=null).
	flag, err := reply.ReadInt32()
	require.NoError(t, err, "failed to read nullable flag")
	require.Equal(t, int32(1), flag, "expected non-null Country parcelable")

	country, err := reply.ReadString16()
	require.NoError(t, err, "failed to read country code string")
	assert.NotEmpty(t, country, "country code should be non-empty")

	source, err := reply.ReadInt32()
	require.NoError(t, err, "failed to read source field")

	timestamp, err := reply.ReadInt64()
	require.NoError(t, err, "failed to read timestamp field")
	assert.Greater(t, timestamp, int64(0), "timestamp should be positive")

	t.Logf("country=%q, source=%d, timestamp=%d", country, source, timestamp)
}

// --- Nullable parcelable (null case) ---

func TestType_NullableParcelable(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "alarm")

	const descriptor = "android.app.IAlarmManager"

	// IAlarmManager::getNextAlarmClock (code 7) -> nullable AlarmClockInfo.
	data := parcel.New()
	data.WriteInterfaceToken(descriptor)
	data.WriteInt32(0) // userId

	reply, err := svc.Transact(ctx, 7, 0, data)
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

// --- Nested parcelable with List<Parcelable> and float32 fields ---

func TestType_NestedParcelable(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sf := getSurfaceFlingerAIDL(ctx, t, driver)

	const knownDisplayID = int64(4619827259835644672)

	// ISurfaceComposer::getDynamicDisplayInfoFromId (code 13) -> DynamicDisplayInfo.
	data := parcel.New()
	data.WriteInterfaceToken(surfaceComposerDescriptor)
	data.WriteInt64(knownDisplayID)

	reply, err := sf.Transact(ctx, 13, 0, data)
	requireOrSkip(t, err)
	requireOrSkip(t, binder.ReadStatus(reply))

	// DynamicDisplayInfo parcelable envelope.
	stability, err := reply.ReadInt32()
	require.NoError(t, err, "failed to read stability")
	t.Logf("DynamicDisplayInfo stability: %d", stability)

	size, err := reply.ReadInt32()
	require.NoError(t, err, "failed to read parcelable size")
	t.Logf("DynamicDisplayInfo size: %d", size)

	// First field: List<DisplayMode> (supportedDisplayModes).
	modeCount, err := reply.ReadInt32()
	require.NoError(t, err, "failed to read DisplayMode list count")
	t.Logf("DisplayMode count: %d", modeCount)
	require.Greater(t, modeCount, int32(0), "expected at least one DisplayMode")

	// Read the first DisplayMode parcelable.
	modeStability, err := reply.ReadInt32()
	require.NoError(t, err, "failed to read DisplayMode stability")
	t.Logf("  DisplayMode[0] stability: %d", modeStability)

	modeSize, err := reply.ReadInt32()
	require.NoError(t, err, "failed to read DisplayMode size")
	t.Logf("  DisplayMode[0] size: %d", modeSize)

	// Record position at start of DisplayMode fields.
	modeStart := reply.Position()

	// offset 0: int32 id.
	modeID, err := reply.ReadInt32()
	require.NoError(t, err)
	t.Logf("  DisplayMode[0].id: %d", modeID)

	// offset 4: int32 (unknown).
	_, err = reply.ReadInt32()
	require.NoError(t, err)

	// offset 8: int32 (unknown).
	_, err = reply.ReadInt32()
	require.NoError(t, err)

	// offset 12: int32 width.
	width, err := reply.ReadInt32()
	require.NoError(t, err)
	t.Logf("  DisplayMode[0].width: %d", width)

	// offset 16: int32 height.
	height, err := reply.ReadInt32()
	require.NoError(t, err)
	t.Logf("  DisplayMode[0].height: %d", height)

	// offset 20: float32 xDpi.
	xDpi, err := reply.ReadFloat32()
	require.NoError(t, err)
	t.Logf("  DisplayMode[0].xDpi: %f", xDpi)
	assert.Greater(t, xDpi, float32(0), "xDpi should be positive")
	assert.False(t, math.IsNaN(float64(xDpi)), "xDpi should not be NaN")

	// offset 24: float32 yDpi.
	yDpi, err := reply.ReadFloat32()
	require.NoError(t, err)
	t.Logf("  DisplayMode[0].yDpi: %f", yDpi)
	assert.Greater(t, yDpi, float32(0), "yDpi should be positive")
	assert.False(t, math.IsNaN(float64(yDpi)), "yDpi should not be NaN")

	// Skip to offset 32: float32 refreshRate.
	reply.SetPosition(modeStart + 32)
	refreshRate, err := reply.ReadFloat32()
	require.NoError(t, err)
	t.Logf("  DisplayMode[0].refreshRate: %f", refreshRate)
	assert.Greater(t, refreshRate, float32(0), "refreshRate should be positive")
}

// --- Enum (int-backed) return ---

func TestType_EnumValue(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "uimode")

	const descriptor = "android.app.IUiModeManager"

	// IUiModeManager::getCurrentModeType (code 7) -> int32 (UI_MODE_TYPE enum).
	reply := transactNoArg(ctx, t, svc, descriptor, 7)

	val, err := reply.ReadInt32()
	require.NoError(t, err, "failed to read enum value")
	t.Logf("getCurrentModeType: %d", val)

	// Known values: 1=NORMAL, 2=CAR, 3=TV, 4=WATCH, 5=APPLIANCE, 6=DESK, 7=VR.
	assert.GreaterOrEqual(t, val, int32(1), "mode type should be a valid enum value (>= 1)")
}

// --- Large parcelable ---

func TestType_LargeParcelable(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "display")

	const descriptor = "android.hardware.display.IDisplayManager"

	// IDisplayManager::getDisplayInfo(0) (code 1) -> DisplayInfo parcelable.
	data := parcel.New()
	data.WriteInterfaceToken(descriptor)
	data.WriteInt32(0) // display 0

	reply, err := svc.Transact(ctx, 1, 0, data)
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

	// IAppOpsService::checkOperation(int op, int uid, String packageName) (code 1) -> int32.
	data := parcel.New()
	data.WriteInterfaceToken(descriptor)
	data.WriteInt32(24) // OP_SYSTEM_ALERT_WINDOW
	data.WriteInt32(0)  // uid 0 (root)
	data.WriteString16("com.android.systemui")

	reply, err := svc.Transact(ctx, 1, 0, data)
	requireOrSkip(t, err)
	requireOrSkip(t, binder.ReadStatus(reply))

	mode, err := reply.ReadInt32()
	require.NoError(t, err, "failed to read operation mode")
	t.Logf("checkOperation(OP_SYSTEM_ALERT_WINDOW, uid=0, com.android.systemui): mode=%d", mode)

	// Valid modes: 0=ALLOWED, 1=IGNORED, 2=ERRORED, 3=DEFAULT, 4=FOREGROUND.
	assert.GreaterOrEqual(t, mode, int32(0), "mode should be >= 0")
	assert.LessOrEqual(t, mode, int32(4), "mode should be <= 4")
}
