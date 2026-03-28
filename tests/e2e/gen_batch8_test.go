//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/AndroidGoLab/binder/servicemanager"
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
			// Some HAL services (e.g. secureclock) hang indefinitely in
			// the binder ioctl on certain devices/emulators. Since a
			// blocking syscall cannot be cancelled via context, we run
			// the binder calls in a goroutine and skip on timeout.
			//
			// Open the driver in the main goroutine so t.Cleanup is
			// safe; only the binder RPCs run in the background.
			driver := openBinder(t)
			sm := servicemanager.New(driver)

			type result struct {
				alive  bool
				handle uint32
				err    error
				found  bool
			}
			ch := make(chan result, 1)
			go func() {
				svc, err := sm.GetService(ctx, servicemanager.ServiceName(name))
				if err != nil {
					ch <- result{err: err}
					return
				}
				if svc == nil {
					ch <- result{}
					return
				}
				alive := svc.IsAlive(ctx)
				ch <- result{alive: alive, handle: svc.Handle(), found: true}
			}()

			select {
			case r := <-ch:
				if r.err != nil {
					t.Skipf("service unavailable: %v", r.err)
					return
				}
				if !r.found {
					t.Skipf("service %s returned nil", name)
					return
				}
				t.Logf("%s alive: %v, handle: %d", name, r.alive, r.handle)
			case <-time.After(10 * time.Second):
				t.Skipf("service %s: timed out (binder call hung)", name)
			}
		})
	}
}
