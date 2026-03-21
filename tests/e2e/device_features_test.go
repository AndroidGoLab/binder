//go:build e2e

package e2e

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	genApp "github.com/xaionaro-go/binder/android/app"
	genDisplay "github.com/xaionaro-go/binder/android/hardware/display"
	genOs "github.com/xaionaro-go/binder/android/os"
	"github.com/xaionaro-go/binder/binder"
	"github.com/xaionaro-go/binder/parcel"
	"github.com/xaionaro-go/binder/servicemanager"
)

// --- audio: android.media.IAudioService ---
// No typed proxy exists for IAudioService. Use raw transact.

func TestDeviceFeature_Audio(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "audio")

	// isMasterMute() — no parameters, returns bool
	code := resolveCode(ctx, t, svc, "android.media.IAudioService", "isMasterMute")
	data := parcel.New()
	data.WriteInterfaceToken("android.media.IAudioService")

	reply, err := svc.Transact(ctx, code, 0, data)
	requireOrSkip(t, err)
	requireOrSkip(t, binder.ReadStatus(reply))

	muted, err := reply.ReadBool()
	requireOrSkip(t, err)
	t.Logf("isMasterMute: %v", muted)
}

// --- notification: android.app.INotificationManager ---

func TestDeviceFeature_Notifications(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "notification")

	proxy := genApp.NewNotificationManagerProxy(svc)
	mode, err := proxy.GetZenMode(ctx)
	requireOrSkip(t, err)
	t.Logf("GetZenMode: %d", mode)
}

// --- display: android.hardware.display.IDisplayManager ---

func TestDeviceFeature_Display(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "display")

	proxy := genDisplay.NewDisplayManagerProxy(svc)
	ids, err := proxy.GetDisplayIds(ctx, false)
	requireOrSkip(t, err)
	require.NotEmpty(t, ids, "device should have at least one display")
	t.Logf("GetDisplayIds: %v", ids)
}

// --- wifi: android.net.wifi.IWifiManager ---
// No typed proxy and not in version tables. Verify service is
// reachable and alive. The IWifiManager methods are Java-only
// and not in the AIDL spec, so we can't resolve transaction codes.

func TestDeviceFeature_WiFi(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)

	svc, err := sm.CheckService(ctx, "wifi")
	requireOrSkip(t, err)
	if svc == nil {
		t.Skip("wifi service not registered")
	}
	require.True(t, svc.IsAlive(ctx), "wifi service should be alive")
	t.Logf("wifi service: alive, handle present")
}

// --- phone: com.android.internal.telephony.ITelephony ---

func TestDeviceFeature_Telephony(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "phone")

	code := resolveCode(ctx, t, svc, "com.android.internal.telephony.ITelephony", "getNetworkCountryIsoForPhone")
	data := parcel.New()
	data.WriteInterfaceToken("com.android.internal.telephony.ITelephony")
	data.WriteInt32(0) // phoneId = 0

	reply, err := svc.Transact(ctx, code, 0, data)
	requireOrSkip(t, err)
	requireOrSkip(t, binder.ReadStatus(reply))

	iso, err := reply.ReadString16()
	requireOrSkip(t, err)
	// SIM-less devices may return an empty country ISO code.
	if iso != "" {
		assert.Len(t, iso, 2, "country ISO code should be 2 characters")
	}
	t.Logf("getNetworkCountryIsoForPhone(0): %q", iso)
}

// --- power: android.os.IPowerManager ---

func TestDeviceFeature_Power(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "power")

	proxy := genOs.NewPowerManagerProxy(svc)

	powerSave, err := proxy.IsPowerSaveMode(ctx)
	requireOrSkip(t, err)
	t.Logf("IsPowerSaveMode: %v", powerSave)

	interactive, err := proxy.IsInteractive(ctx)
	requireOrSkip(t, err)
	t.Logf("IsInteractive: %v", interactive)
}
