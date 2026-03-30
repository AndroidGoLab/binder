// Binary security_test_apk probes whether an app-sandboxed process can
// reach critical HAL binder services. All calls are read-only and
// non-destructive. Intended for security research.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/facebookincubator/go-belt"
	"github.com/facebookincubator/go-belt/tool/logger"
	"github.com/facebookincubator/go-belt/tool/logger/implementation/logrus"

	genApp "github.com/AndroidGoLab/binder/android/app"
	genBoot "github.com/AndroidGoLab/binder/android/hardware/boot"
	genKeymint "github.com/AndroidGoLab/binder/android/hardware/security/keymint"
	genInstalld "github.com/AndroidGoLab/binder/android/os"
	genUsb "github.com/AndroidGoLab/binder/android/hardware/usb"
	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/binder/versionaware"
	"github.com/AndroidGoLab/binder/kernelbinder"
	"github.com/AndroidGoLab/binder/servicemanager"
)

// serviceProbe defines a single binder service to probe.
type serviceProbe struct {
	// ServiceName is the name passed to ServiceManager.CheckService.
	ServiceName string
	// CallMethod calls a read-only proxy method. Returns a result string
	// and error. If nil, only lookup is tested.
	CallMethod func(ctx context.Context, svc binder.IBinder) (string, error)
}

var probes = []serviceProbe{
	{
		ServiceName: "android.hardware.security.keymint.IKeyMintDevice/default",
		CallMethod: func(ctx context.Context, svc binder.IBinder) (string, error) {
			proxy := genKeymint.NewKeyMintDeviceProxy(svc)
			info, err := proxy.GetHardwareInfo(ctx)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("version=%d secLevel=%d name=%q author=%q",
				info.VersionNumber, info.SecurityLevel, info.KeyMintName, info.KeyMintAuthorName), nil
		},
	},
	{
		ServiceName: "android.hardware.usb.IUsb/default",
		CallMethod: func(ctx context.Context, svc binder.IBinder) (string, error) {
			proxy := genUsb.NewUsbProxy(svc)
			return "queryPortStatus sent", proxy.QueryPortStatus(ctx, 0)
		},
	},
	{
		ServiceName: "android.hardware.boot.IBootControl/default",
		CallMethod: func(ctx context.Context, svc binder.IBinder) (string, error) {
			proxy := genBoot.NewBootControlProxy(svc)
			slot, err := proxy.GetCurrentSlot(ctx)
			return fmt.Sprintf("currentSlot=%d", slot), err
		},
	},
	{
		ServiceName: "installd",
		CallMethod: func(ctx context.Context, svc binder.IBinder) (string, error) {
			proxy := genInstalld.NewInstalldProxy(svc)
			supported, err := proxy.IsQuotaSupported(ctx, "")
			return fmt.Sprintf("isQuotaSupported=%v", supported), err
		},
	},
	{
		ServiceName: "activity",
		CallMethod: func(ctx context.Context, svc binder.IBinder) (string, error) {
			proxy := genApp.NewActivityManagerProxy(svc)
			isMonkey, err := proxy.IsUserAMonkey(ctx)
			return fmt.Sprintf("isUserAMonkey=%v", isMonkey), err
		},
	},
}

func main() {
	ctx := context.Background()
	l := logrus.Default().WithLevel(logger.LevelDebug)
	ctx = belt.CtxWithBelt(ctx, belt.New())
	ctx = logger.CtxWithLogger(ctx, l)

	fmt.Println("=== Binder HAL Security Probe ===")
	fmt.Printf("PID: %d  UID: %d\n", os.Getpid(), os.Getuid())
	fmt.Println()

	results := runProbes(ctx)

	fmt.Println()
	fmt.Println("=== Summary ===")
	for _, r := range results {
		fmt.Println(r)
	}
}

func runProbes(ctx context.Context) []string {
	var results []string

	// Open the binder driver.
	driver, err := kernelbinder.Open(ctx, binder.WithMapSize(128*1024))
	if err != nil {
		msg := fmt.Sprintf("/dev/binder open: FAILED (%v)", err)
		fmt.Println(msg)
		return []string{msg}
	}
	defer driver.Close(ctx)
	fmt.Println("/dev/binder open: SUCCESS")
	results = append(results, "/dev/binder open: SUCCESS")

	// Create version-aware transport.
	transport, err := versionaware.NewTransport(ctx, driver, 0)
	if err != nil {
		msg := fmt.Sprintf("VersionAwareTransport: FAILED (%v)", err)
		fmt.Println(msg)
		results = append(results, msg)
		return results
	}
	defer transport.Close(ctx)
	fmt.Println("VersionAwareTransport: SUCCESS")
	results = append(results, "VersionAwareTransport: SUCCESS")

	sm := servicemanager.New(transport)

	// First, list what services are visible at all.
	results = append(results, probeListServices(ctx, sm)...)
	fmt.Println()

	// Probe each target service.
	for _, probe := range probes {
		r := probeServiceEntry(ctx, sm, probe)
		results = append(results, r...)
		fmt.Println()
	}

	return results
}

func probeListServices(ctx context.Context, sm *servicemanager.ServiceManager) []string {
	var results []string

	services, err := sm.ListServices(ctx)
	if err != nil {
		msg := fmt.Sprintf("ListServices: FAILED (%v)", err)
		fmt.Println(msg)
		results = append(results, msg)
		return results
	}

	fmt.Printf("ListServices: SUCCESS (%d services visible)\n", len(services))
	results = append(results, fmt.Sprintf("ListServices: SUCCESS (%d services visible)", len(services)))

	// Check which of our target services appear in the list.
	serviceSet := make(map[string]bool, len(services))
	for _, s := range services {
		serviceSet[string(s)] = true
	}

	for _, probe := range probes {
		visible := serviceSet[probe.ServiceName]
		status := "NOT LISTED"
		if visible {
			status = "LISTED"
		}
		msg := fmt.Sprintf("  %s: %s", probe.ServiceName, status)
		fmt.Println(msg)
		results = append(results, msg)
	}

	return results
}

func probeServiceEntry(
	ctx context.Context,
	sm *servicemanager.ServiceManager,
	probe serviceProbe,
) []string {
	var results []string
	header := fmt.Sprintf("[%s]", probe.ServiceName)
	fmt.Println(header)

	// Step 1: CheckService lookup.
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	svcBinder, err := sm.CheckService(checkCtx, servicemanager.ServiceName(probe.ServiceName))
	if err != nil {
		msg := fmt.Sprintf("%s CheckService: FAILED (%v)", header, err)
		fmt.Println(msg)
		return append(results, msg)
	}
	if svcBinder == nil {
		msg := fmt.Sprintf("%s CheckService: NOT FOUND (service not registered)", header)
		fmt.Println(msg)
		return append(results, msg)
	}
	msg := fmt.Sprintf("%s CheckService: SUCCESS (handle=%d)", header, svcBinder.Handle())
	fmt.Println(msg)
	results = append(results, msg)

	// Step 2: Call the read-only method via generated proxy.
	if probe.CallMethod == nil {
		return results
	}

	callCtx, callCancel := context.WithTimeout(ctx, 5*time.Second)
	defer callCancel()

	detail, err := probe.CallMethod(callCtx, svcBinder)
	if err != nil {
		msg := fmt.Sprintf("%s method call: FAILED (%v)", header, err)
		fmt.Println(msg)
		results = append(results, msg)

		// Classify the error for the security report.
		errStr := err.Error()
		switch {
		case strings.Contains(errStr, "PERMISSION_DENIED") ||
			strings.Contains(errStr, "permission"):
			results = append(results, "  -> ACCESS DENIED (sandbox blocks this)")
		case strings.Contains(errStr, "DEAD_OBJECT") ||
			strings.Contains(errStr, "dead"):
			results = append(results, "  -> SERVICE DEAD")
		default:
			results = append(results, fmt.Sprintf("  -> ERROR TYPE: %T", err))
		}
		return results
	}

	msg = fmt.Sprintf("%s method call: SUCCESS (%s)", header, detail)
	fmt.Println(msg)
	results = append(results, msg)

	return results
}
