//go:build e2e

package e2e

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xaionaro-go/binder/binder"
	aidlerrors "github.com/xaionaro-go/binder/errors"
	"github.com/xaionaro-go/binder/parcel"
	"github.com/xaionaro-go/binder/servicemanager"
)

func TestServiceBreadth_PingManyServices(t *testing.T) {
	ctx := context.Background()

	serviceNames := []string{
		"SurfaceFlinger", "SurfaceFlingerAIDL", "activity", "power", "window",
		"package", "user", "uimode", "display", "notification", "clipboard",
		"alarm", "connectivity", "appops", "vibrator_manager", "country_detector",
		"input", "gpu", "font", "overlay", "audio", "media_session",
		"location", "account", "bluetooth_manager", "phone", "usb",
		"statusbar", "wallpaper", "dreams", "trust", "role",
		"jobscheduler", "content", "search", "game", "mount",
		"deviceidle", "thermalservice", "batteryproperties", "processinfo",
		"storagestats", "webviewupdate", "credential", "media_router", "midi",
	}

	var successCount int32

	for _, name := range serviceNames {
		name := name
		t.Run(name, func(t *testing.T) {
			driver := openBinder(t)
			sm := servicemanager.New(driver)

			svc, err := sm.GetService(ctx, servicemanager.ServiceName(name))
			if err != nil {
				t.Logf("GetService(%s) failed: %v", name, err)
				return
			}
			if svc == nil {
				t.Logf("GetService(%s) returned nil", name)
				return
			}

			alive := svc.IsAlive(ctx)
			t.Logf("%s: handle=%d alive=%v", name, svc.Handle(), alive)
			if !alive {
				t.Logf("%s: not alive", name)
				return
			}

			atomic.AddInt32(&successCount, 1)
		})
	}

	finalCount := atomic.LoadInt32(&successCount)
	assert.GreaterOrEqual(t, finalCount, int32(30),
		"expected at least 30 services to be reachable, got %d", finalCount)
	t.Logf("total services pinged successfully: %d/%d", finalCount, len(serviceNames))
}

func TestServiceBreadth_TransactAcrossCategories(t *testing.T) {
	ctx := context.Background()

	type serviceSpec struct {
		name       string
		descriptor string
		method     string
		writeArgs  func(data *parcel.Parcel)
		readResult func(t *testing.T, reply *parcel.Parcel)
	}

	specs := []serviceSpec{
		{
			name:       "power",
			descriptor: "android.os.IPowerManager",
			method:     "isPowerSaveMode",
			readResult: func(t *testing.T, reply *parcel.Parcel) {
				t.Helper()
				val, err := reply.ReadBool()
				require.NoError(t, err)
				t.Logf("power.isPowerSaveMode: %v", val)
			},
		},
		{
			name:       "window",
			descriptor: "android.view.IWindowManager",
			method:     "isKeyguardLocked",
			readResult: func(t *testing.T, reply *parcel.Parcel) {
				t.Helper()
				val, err := reply.ReadBool()
				require.NoError(t, err)
				t.Logf("window.isKeyguardLocked: %v", val)
			},
		},
		{
			name:       "uimode",
			descriptor: "android.app.IUiModeManager",
			method:     "getCurrentModeType",
			readResult: func(t *testing.T, reply *parcel.Parcel) {
				t.Helper()
				val, err := reply.ReadInt32()
				require.NoError(t, err)
				t.Logf("uimode.getCurrentModeType: %d", val)
			},
		},
		{
			name:       "display",
			descriptor: "android.hardware.display.IDisplayManager",
			method:     "getDisplayIds",
			writeArgs: func(data *parcel.Parcel) {
				data.WriteBool(false)
			},
			readResult: func(t *testing.T, reply *parcel.Parcel) {
				t.Helper()
				count, err := reply.ReadInt32()
				require.NoError(t, err)
				require.GreaterOrEqual(t, count, int32(0))
				ids := make([]int32, count)
				for i := int32(0); i < count; i++ {
					ids[i], err = reply.ReadInt32()
					require.NoError(t, err)
				}
				t.Logf("display.getDisplayIds: count=%d ids=%v", count, ids)
			},
		},
		{
			name:       "notification",
			descriptor: "android.app.INotificationManager",
			method:     "getZenMode",
			readResult: func(t *testing.T, reply *parcel.Parcel) {
				t.Helper()
				val, err := reply.ReadInt32()
				require.NoError(t, err)
				t.Logf("notification.getZenMode: %d", val)
			},
		},
		{
			name:       "clipboard",
			descriptor: "android.content.IClipboard",
			method:     "hasClipboardText",
			writeArgs: func(data *parcel.Parcel) {
				data.WriteString16("com.android.shell")
				data.WriteInt32(0)
				data.WriteInt32(0)
			},
			readResult: func(t *testing.T, reply *parcel.Parcel) {
				t.Helper()
				val, err := reply.ReadBool()
				require.NoError(t, err)
				t.Logf("clipboard.hasClipboardText: %v", val)
			},
		},
		{
			name:       "connectivity",
			descriptor: "android.net.IConnectivityManager",
			method:     "isNetworkSupported",
			writeArgs: func(data *parcel.Parcel) {
				data.WriteInt32(1)
			},
			readResult: func(t *testing.T, reply *parcel.Parcel) {
				t.Helper()
				val, err := reply.ReadBool()
				require.NoError(t, err)
				t.Logf("connectivity.isNetworkSupported: %v", val)
			},
		},
		{
			name:       "appops",
			descriptor: "com.android.internal.app.IAppOpsService",
			method:     "checkOperation",
			writeArgs: func(data *parcel.Parcel) {
				data.WriteInt32(24)
				data.WriteInt32(0)
				data.WriteString16("com.android.systemui")
			},
			readResult: func(t *testing.T, reply *parcel.Parcel) {
				t.Helper()
				val, err := reply.ReadInt32()
				require.NoError(t, err)
				t.Logf("appops.checkOperation: %d", val)
			},
		},
		{
			name:       "vibrator_manager",
			descriptor: "android.os.IVibratorManagerService",
			method:     "getVibratorIds",
			readResult: func(t *testing.T, reply *parcel.Parcel) {
				t.Helper()
				count, err := reply.ReadInt32()
				require.NoError(t, err)
				require.GreaterOrEqual(t, count, int32(0))
				ids := make([]int32, count)
				for i := int32(0); i < count; i++ {
					ids[i], err = reply.ReadInt32()
					require.NoError(t, err)
				}
				t.Logf("vibrator_manager.getVibratorIds: count=%d ids=%v", count, ids)
			},
		},
		{
			name:       "user",
			descriptor: "android.os.IUserManager",
			method:     "getCredentialOwnerProfile",
			writeArgs: func(data *parcel.Parcel) {
				data.WriteInt32(0)
			},
			readResult: func(t *testing.T, reply *parcel.Parcel) {
				t.Helper()
				val, err := reply.ReadInt32()
				require.NoError(t, err)
				t.Logf("user.getCredentialOwnerProfile: %d", val)
			},
		},
	}

	for _, spec := range specs {
		spec := spec
		t.Run(spec.name, func(t *testing.T) {
			driver := openBinder(t)
			sm := servicemanager.New(driver)

			svc, err := sm.GetService(ctx, servicemanager.ServiceName(spec.name))
			require.NoError(t, err, "GetService(%s) failed", spec.name)
			require.NotNil(t, svc, "GetService(%s) returned nil", spec.name)

			code, err := svc.ResolveCode(ctx, spec.descriptor, spec.method)
			requireOrSkip(t, err)

			data := parcel.New()
			data.WriteInterfaceToken(spec.descriptor)
			if spec.writeArgs != nil {
				spec.writeArgs(data)
			}

			reply, err := svc.Transact(ctx, code, 0, data)
			require.NoError(t, err, "Transact failed for %s method %s", spec.name, spec.method)
			if statusErr := binder.ReadStatus(reply); statusErr != nil {
				var se *aidlerrors.StatusError
				if errors.As(statusErr, &se) && se.Exception == aidlerrors.ExceptionSecurity {
					t.Skipf("%s method %s: %v", spec.name, spec.method, se)
				}
				require.NoError(t, statusErr, "AIDL status error for %s method %s", spec.name, spec.method)
			}

			spec.readResult(t, reply)
		})
	}
}
