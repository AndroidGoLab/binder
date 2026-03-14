//go:build e2e

package e2e

import (
	"context"
	"testing"

	"github.com/xaionaro-go/aidl/servicemanager"
)

func TestGenBatch6_PingServices(t *testing.T) {
	ctx := context.Background()

	services := []string{
		"app_prediction",
		"app_search",
		"appwidget",
		"attestation_verification",
		"batterystats",
		"carrier_config",
		"connectivity_native",
		"device_lock",
		"dropbox",
		"ecm_enhanced_confirmation",
		"healthconnect",
		"input_method",
		"ipsec",
		"lock_settings",
		"media.audio_flinger",
		"media.audio_policy",
		"media.camera",
		"media.camera.proxy",
		"media.extractor",
		"media.metrics",
		"media.player",
		"media.resource_manager",
		"media.resource_observer",
		"media_communication",
		"nearby",
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
