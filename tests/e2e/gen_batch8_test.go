//go:build e2e

package e2e

import (
	"context"
	"testing"

	"github.com/xaionaro-go/aidl/servicemanager"
)

func TestGenBatch8_PingServices(t *testing.T) {
	ctx := context.Background()

	services := []string{
		"wifi",
		"wifip2p",
		"wifiscanner",
		"window",
		"adservices_manager",
		"android.frameworks.cameraservice.service.ICameraService/default",
		"android.frameworks.location.altitude.IAltitudeService/default",
		"android.frameworks.sensorservice.ISensorManager/default",
		"android.frameworks.stats.IStats/default",
		"android.frameworks.vibrator.IVibratorControlService/default",
		"android.hardware.neuralnetworks.IDevice/nnapi-sample_quant",
		"android.hardware.neuralnetworks.IDevice/nnapi-sample_sl_shim",
		"android.hardware.security.keymint.IRemotelyProvisionedComponent/default",
		"android.security.apc",
		"android.security.authorization",
		"android.security.compat",
		"android.security.identity",
		"android.security.legacykeystore",
		"android.security.maintenance",
		"android.security.metrics",
		"drm.drmManager",
		"imms",
		"ions",
		"iphonesubinfo",
		"isms",
		"isub",
		// VINTF HAL services (empty descriptor in service list, but AIDL-based)
		"android.hardware.camera.provider.ICameraProvider/internal/1",
		"android.hardware.light.ILights/default",
		"android.hardware.power.stats.IPowerStats/default",
		"android.hardware.radio.config.IRadioConfig/default",
		"android.hardware.radio.data.IRadioData/slot1",
		"android.hardware.radio.ims.IRadioIms/slot1",
		"android.hardware.radio.ims.media.IImsMedia/default",
		"android.hardware.radio.messaging.IRadioMessaging/slot1",
		"android.hardware.radio.modem.IRadioModem/slot1",
		"android.hardware.radio.network.IRadioNetwork/slot1",
		"android.hardware.radio.sim.IRadioSim/slot1",
		"android.hardware.radio.voice.IRadioVoice/slot1",
		"android.hardware.rebootescrow.IRebootEscrow/default",
		"android.hardware.security.keymint.IKeyMintDevice/default",
		"android.hardware.security.secureclock.ISecureClock/default",
		"android.hardware.security.sharedsecret.ISharedSecret/default",
		"android.hardware.sensors.ISensors/default",
		"android.hardware.usb.IUsb/default",
		"android.service.gatekeeper.IGateKeeperService",
		"android.system.net.netd.INetd/default",
		// Framework services with non-standard descriptors
		"battery",
		"netd_listener",
		"settings",
		"simphonebook",
		// Debug/internal services (no AIDL descriptor, but registered)
		"DockObserver",
		"app_binding",
		"binder_calls_stats",
		"cacheinfo",
		"cpu_monitor",
		"cpuinfo",
		"dbinfo",
		"device_config",
		"devicestoragemonitor",
		"diskstats",
		"dnsresolver",
		"emergency_affordance",
		"gfxinfo",
		"location_time_zone_manager",
		"looper_stats",
		"mdns",
		"meminfo",
		"netd",
		"network_time_update_service",
		"runtime",
		"suspend_control_internal",
		"system_server_dumper",
		"testharness",
		"vold",
		"wifinl80211",
	}

	for _, name := range services {
		name := name
		t.Run(name, func(t *testing.T) {
			driver := openBinder(t)
			sm := servicemanager.New(driver)
			svc, err := sm.GetService(ctx, name)
			if err != nil {
				t.Skipf("service unavailable: %v", err)
				return
			}
			if svc == nil {
				t.Skipf("service %s returned nil", name)
				return
			}
			alive := svc.IsAlive(ctx)
			t.Logf("%s alive: %v, handle: %d", name, alive, svc.Handle())
		})
	}
}
