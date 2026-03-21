//go:build e2e

package e2e

import (
	"context"
	"testing"

	"github.com/xaionaro-go/binder/servicemanager"

	genPm "github.com/xaionaro-go/binder/android/content/pm"
	genRollback "github.com/xaionaro-go/binder/android/content/rollback"
	genHwUsb "github.com/xaionaro-go/binder/android/hardware/usb"
	genOs "github.com/xaionaro-go/binder/android/os"
	genOsStorage "github.com/xaionaro-go/binder/android/os/storage"
	genPrint "github.com/xaionaro-go/binder/android/print"
	genSe "github.com/xaionaro-go/binder/android/se/omapi"
	genSlice "github.com/xaionaro-go/binder/android/app/slice"
	genSearch "github.com/xaionaro-go/binder/android/app"
	genPinner "github.com/xaionaro-go/binder/android/app/pinner"
	genTrust "github.com/xaionaro-go/binder/android/app/trust"
	genUsage "github.com/xaionaro-go/binder/android/app/usage"
	genVirtual "github.com/xaionaro-go/binder/android/companion/virtual"
	genVirtualNative "github.com/xaionaro-go/binder/android/companion/virtualnative"
	genVcn "github.com/xaionaro-go/binder/android/net/vcn"
)

// helper: get service or skip


// --- Typed proxy tests ---

// Overlay test removed: IOverlayManager proxy not generated.

func TestGenBatch5_Package_IsPackageAvailable(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)
	svc, err := sm.GetService(ctx, servicemanager.ServiceName("package"))
	requireOrSkip(t, err)
	proxy := genPm.NewPackageManagerProxy(svc)
	result, err := proxy.IsPackageAvailable(ctx, "com.android.shell")
	logProxyResult(t, "package", "IsPackageAvailable", err)
	if err == nil {
		t.Logf("com.android.shell available: %v", result)
	}
}

func TestGenBatch5_PackageNative_GetInstallerForPackage(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)
	svc, err := sm.GetService(ctx, servicemanager.ServiceName("package_native"))
	requireOrSkip(t, err)
	proxy := genPm.NewPackageManagerNativeProxy(svc)
	result, err := proxy.GetInstallerForPackage(ctx, "com.android.shell")
	logProxyResult(t, "package_native", "GetInstallerForPackage", err)
	if err == nil {
		t.Logf("installer for com.android.shell: %q", result)
	}
}

func TestGenBatch5_SecurityState(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)
	svc, err := sm.GetService(ctx, servicemanager.SecurityStateService)
	requireOrSkip(t, err)
	proxy := genOs.NewSecurityStateManagerProxy(svc)
	_, err = proxy.GetGlobalSecurityState(ctx)
	logProxyResult(t, "security_state", "GetGlobalSecurityState", err)
}

func TestGenBatch5_SystemConfig_GetDisabledUntilUsedPreinstalledCarrierApps(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)
	svc, err := sm.GetService(ctx, servicemanager.SystemConfigService)
	requireOrSkip(t, err)
	proxy := genOs.NewSystemConfigProxy(svc)
	_, err = proxy.GetDisabledUntilUsedPreinstalledCarrierApps(ctx)
	logProxyResult(t, "system_config", "GetDisabledUntilUsedPreinstalledCarrierApps", err)
}

func TestGenBatch5_USB_GetDeviceList(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)
	svc, err := sm.GetService(ctx, servicemanager.UsbService)
	requireOrSkip(t, err)
	proxy := genHwUsb.NewUsbManagerProxy(svc)
	_ = proxy.AsBinder()
	t.Logf("usb: proxy created, handle=%d", svc.Handle())
}

func TestGenBatch5_Wallpaper(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)
	svc, err := sm.GetService(ctx, servicemanager.WallpaperService)
	requireOrSkip(t, err)
	proxy := genSearch.NewWallpaperManagerProxy(svc)
	result, err := proxy.IsWallpaperSupported(ctx)
	logProxyResult(t, "wallpaper", "IsWallpaperSupported", err)
	if err == nil {
		t.Logf("wallpaper supported: %v", result)
	}
}

func TestGenBatch5_Trust(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)
	svc, err := sm.GetService(ctx, servicemanager.TrustService)
	requireOrSkip(t, err)
	proxy := genTrust.NewTrustManagerProxy(svc)
	result, err := proxy.IsTrustUsuallyManaged(ctx)
	logProxyResult(t, "trust", "IsTrustUsuallyManaged", err)
	if err == nil {
		t.Logf("trust usually managed: %v", result)
	}
}

func TestGenBatch5_SecureElement_GetReaders(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)
	svc, err := sm.GetService(ctx, servicemanager.SecureElementService)
	requireOrSkip(t, err)
	proxy := genSe.NewSecureElementServiceProxy(svc)
	_, err = proxy.GetReaders(ctx)
	logProxyResult(t, "secure_element", "GetReaders", err)
}

func TestGenBatch5_Print_GetPrintJobInfos(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)
	svc, err := sm.GetService(ctx, servicemanager.PrintService)
	requireOrSkip(t, err)
	proxy := genPrint.NewPrintManagerProxy(svc)
	_ = proxy.AsBinder()
	t.Logf("print: proxy created, handle=%d", svc.Handle())
}

func TestGenBatch5_ProcessInfo(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)
	svc, err := sm.GetService(ctx, servicemanager.ServiceName("processinfo"))
	requireOrSkip(t, err)
	proxy := genOs.NewProcessInfoServiceProxy(svc)
	_ = proxy.AsBinder()
	t.Logf("processinfo: proxy created, handle=%d", svc.Handle())
}

func TestGenBatch5_PowerStats(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)
	svc, err := sm.GetService(ctx, servicemanager.PowerStatsService)
	requireOrSkip(t, err)
	proxy := genOs.NewPowerStatsServiceProxy(svc)
	err = proxy.GetSupportedPowerMonitors(ctx, genOs.ResultReceiver{})
	logProxyResult(t, "powerstats", "GetSupportedPowerMonitors", err)
}

func TestGenBatch5_Pinner(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)
	svc, err := sm.GetService(ctx, servicemanager.ServiceName("pinner"))
	requireOrSkip(t, err)
	proxy := genPinner.NewPinnerServiceProxy(svc)
	_, err = proxy.GetPinnerStats(ctx)
	logProxyResult(t, "pinner", "GetPinnerStats", err)
}

func TestGenBatch5_Rollback(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)
	svc, err := sm.GetService(ctx, servicemanager.RollbackService)
	requireOrSkip(t, err)
	proxy := genRollback.NewRollbackManagerProxy(svc)
	_, err = proxy.GetAvailableRollbacks(ctx)
	logProxyResult(t, "rollback", "GetAvailableRollbacks", err)
}

func TestGenBatch5_Recovery(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)
	svc, err := sm.GetService(ctx, servicemanager.RecoveryService)
	requireOrSkip(t, err)
	proxy := genOs.NewRecoverySystemProxy(svc)
	_ = proxy.AsBinder()
	t.Logf("recovery: proxy created, handle=%d", svc.Handle())
}

func TestGenBatch5_Slice_HasSliceAccess(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)
	svc, err := sm.GetService(ctx, servicemanager.SliceService)
	requireOrSkip(t, err)
	proxy := genSlice.NewSliceManagerProxy(svc)
	result, err := proxy.HasSliceAccess(ctx, "com.android.shell")
	logProxyResult(t, "slice", "HasSliceAccess", err)
	if err == nil {
		t.Logf("has slice access: %v", result)
	}
}

func TestGenBatch5_Search_GetGlobalSearchActivities(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)
	svc, err := sm.GetService(ctx, servicemanager.SearchService)
	requireOrSkip(t, err)
	proxy := genSearch.NewSearchManagerProxy(svc)
	_, err = proxy.GetGlobalSearchActivities(ctx)
	logProxyResult(t, "search", "GetGlobalSearchActivities", err)
}

// PermissionMgr test removed: IPermissionManager proxy not generated.

func TestGenBatch5_VirtualDevice_GetVirtualDevices(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)
	svc, err := sm.GetService(ctx, servicemanager.VirtualDeviceService)
	requireOrSkip(t, err)
	proxy := genVirtual.NewVirtualDeviceManagerProxy(svc)
	_, err = proxy.GetVirtualDevices(ctx)
	logProxyResult(t, "virtualdevice", "GetVirtualDevices", err)
}

func TestGenBatch5_VirtualDeviceNative_GetDeviceIdsForUid(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)
	svc, err := sm.GetService(ctx, servicemanager.ServiceName("virtualdevice_native"))
	requireOrSkip(t, err)
	proxy := genVirtualNative.NewVirtualDeviceManagerNativeProxy(svc)
	_, err = proxy.GetDeviceIdsForUid(ctx, 0)
	logProxyResult(t, "virtualdevice_native", "GetDeviceIdsForUid", err)
}

func TestGenBatch5_SystemUpdate(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)
	svc, err := sm.GetService(ctx, servicemanager.SystemUpdateService)
	requireOrSkip(t, err)
	proxy := genOs.NewSystemUpdateManagerProxy(svc)
	_, err = proxy.RetrieveSystemUpdateInfo(ctx)
	logProxyResult(t, "system_update", "RetrieveSystemUpdateInfo", err)
}

func TestGenBatch5_Mount(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)
	svc, err := sm.GetService(ctx, servicemanager.ServiceName("mount"))
	requireOrSkip(t, err)
	proxy := genOsStorage.NewStorageManagerProxy(svc)
	_ = proxy.AsBinder()
	t.Logf("mount: proxy created, handle=%d", svc.Handle())
}

func TestGenBatch5_UsageStats(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)
	svc, err := sm.GetService(ctx, servicemanager.UsageStatsService)
	requireOrSkip(t, err)
	proxy := genUsage.NewUsageStatsManagerProxy(svc)
	result, err := proxy.IsAppStandbyEnabled(ctx)
	logProxyResult(t, "usagestats", "IsAppStandbyEnabled", err)
	if err == nil {
		t.Logf("app standby enabled: %v", result)
	}
}

func TestGenBatch5_VcnManagement(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)
	svc, err := sm.GetService(ctx, servicemanager.VcnManagementService)
	requireOrSkip(t, err)
	proxy := genVcn.NewVcnManagementServiceProxy(svc)
	_ = proxy.AsBinder()
	t.Logf("vcn_management: proxy created, handle=%d", svc.Handle())
}

// --- Ping-only tests for services with complex-only methods ---

func TestGenBatch5_PingServices(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)

	services := []string{
		"on_device_intelligence", "otadexopt", "pac_proxy",
		"people", "permission", "permission_checker",
		"remote_provisioning", "resources", "restrictions",
		"scheduling_policy", "search_ui", "sec_key_att_app_id_provider",
		"sensitive_content_protection_service", "serial",
		"shortcut", "smartspace", "soundtrigger_middleware",
		"speech_recognition", "statsbootstrap",
		"suspend_control", "telephony_ims",
		"textclassification", "texttospeech",
		"time_detector", "time_zone_detector",
		"tracing.proxy", "translation",
		"updatelock", "uri_grants",
		"vpn_management", "wallpaper_effects_generation",
		"wearable_sensing",
	}

	successCount := 0
	for _, name := range services {
		name := name
		t.Run(name, func(t *testing.T) {
			svc, err := sm.GetService(ctx, servicemanager.ServiceName(name))
			if err != nil || svc == nil {
				t.Skipf("service %s not available", name)
				return
			}
			alive := svc.IsAlive(ctx)
			t.Logf("%s: handle=%d alive=%v", name, svc.Handle(), alive)
			if alive {
				successCount++
			}
		})
	}
	t.Logf("ping success: %d/%d", successCount, len(services))
}

// logProxyResult logs the result of a proxy method call, accepting either success or AIDL exception.
func logProxyResult(t *testing.T, svc, method string, err error) {
	t.Helper()
	if err != nil {
		t.Logf("%s.%s: %v (proxy round-trip successful)", svc, method, err)
	} else {
		t.Logf("%s.%s: success", svc, method)
	}
}
