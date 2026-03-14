//go:build e2e

package e2e

import (
	"context"
	"testing"

	"github.com/xaionaro-go/aidl/servicemanager"
)

func TestGenBatch7_PingServices(t *testing.T) {
	ctx := context.Background()

	services := []string{
		"netstats",
		"network_stack",
		"network_watchlist",
		"ondevicepersonalization_system_service",
		"platform_compat",
		"platform_compat_native",
		"procstats",
		"profiling_service",
		"reboot_readiness",
		"safety_center",
		"sdk_sandbox",
		"sensorservice",
		"servicediscovery",
		"soundtrigger",
		"stats",
		"statscompanion",
		"statsmanager",
		"storaged",
		"storaged_pri",
		"telecom",
		"telephony.registry",
		"tethering",
		"textservices",
		"transparency",
		"voiceinteraction",
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
