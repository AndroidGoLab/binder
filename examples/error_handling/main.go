// Graceful error handling: service availability, permissions, typed errors.
//
// Build:
//
//	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o build/error_handling ./examples/error_handling/
//	adb push build/error_handling /data/local/tmp/ && adb shell /data/local/tmp/error_handling
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/AndroidGoLab/binder/android/app"
	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/binder/versionaware"
	aidlerrors "github.com/AndroidGoLab/binder/errors"
	"github.com/AndroidGoLab/binder/kernelbinder"
	"github.com/AndroidGoLab/binder/servicemanager"
)

func main() {
	ctx := context.Background()

	driver, err := kernelbinder.Open(ctx, binder.WithMapSize(128*1024))
	if err != nil {
		fmt.Fprintf(os.Stderr, "open binder: %v\n", err)
		os.Exit(1)
	}
	defer driver.Close(ctx)

	transport, err := versionaware.NewTransport(ctx, driver, 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "version-aware transport: %v\n", err)
		os.Exit(1)
	}

	sm := servicemanager.New(transport)

	// ---------------------------------------------------------------
	// 1. Service availability: CheckService (non-blocking) vs
	//    GetService (blocking with timeout).
	// ---------------------------------------------------------------
	fmt.Println("=== Service Availability ===")

	// CheckService returns (nil, nil) when the service is not registered.
	// It never blocks.
	svc, err := sm.CheckService(ctx, "this.service.does.not.exist")
	if err != nil {
		fmt.Printf("CheckService error: %v\n", err)
	} else if svc == nil {
		fmt.Println("CheckService: service not found (nil binder, no error)")
	}

	// GetService blocks until the service appears. Use a context timeout
	// so the program does not hang forever on a missing service.
	getCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	svc, err = sm.GetService(getCtx, "this.service.does.not.exist")
	if err != nil {
		fmt.Printf("GetService with timeout: %v\n", err)
	}

	// Probe a real service: "activity" is always present on Android.
	activityBinder, err := sm.CheckService(ctx, servicemanager.ActivityService)
	if err != nil {
		fmt.Fprintf(os.Stderr, "CheckService(activity): %v\n", err)
		os.Exit(1)
	}
	if activityBinder == nil {
		fmt.Fprintln(os.Stderr, "activity service not found")
		os.Exit(1)
	}
	fmt.Printf("CheckService(activity): handle=%d\n", activityBinder.Handle())

	// IsAlive confirms the remote binder handle is still valid.
	if activityBinder.IsAlive(ctx) {
		fmt.Println("activity binder is alive")
	} else {
		fmt.Println("activity binder is DEAD")
	}
	fmt.Println()

	// ---------------------------------------------------------------
	// 2. Permission checks via ActivityManager.
	// ---------------------------------------------------------------
	fmt.Println("=== Permission Checks ===")

	am := app.NewActivityManagerProxy(activityBinder)
	myPid := int32(os.Getpid())
	myUid := int32(os.Getuid())

	permissions := []string{
		"android.permission.INTERNET",
		"android.permission.CAMERA",
		"android.permission.READ_PHONE_STATE",
	}

	for _, perm := range permissions {
		result, err := am.CheckPermission(ctx, perm, myPid, myUid)
		if err != nil {
			fmt.Printf("  %-45s error: %v\n", perm, err)
			handleTypedError(err)
			continue
		}
		status := "DENIED"
		if result == 0 { // PackageManager.PERMISSION_GRANTED == 0
			status = "GRANTED"
		}
		fmt.Printf("  %-45s %s\n", perm, status)
	}
	fmt.Println()

	// ---------------------------------------------------------------
	// 3. Typed error inspection with aidlerrors.StatusError.
	// ---------------------------------------------------------------
	fmt.Println("=== Typed Error Handling ===")

	// Demonstrate how to inspect AIDL errors returned by any binder call.
	// We intentionally call IsUserAMonkey which is safe and read-only,
	// then show the pattern for handling errors from any call.
	_, err = am.IsUserAMonkey(ctx)
	if err != nil {
		fmt.Printf("IsUserAMonkey error: %v\n", err)
		handleTypedError(err)
	} else {
		fmt.Println("IsUserAMonkey: succeeded (no error to inspect)")
		fmt.Println("Showing error-handling pattern for reference:")
		fmt.Println()
		fmt.Println(`  var se *aidlerrors.StatusError`)
		fmt.Println(`  if errors.As(err, &se) {`)
		fmt.Println(`      switch se.Exception {`)
		fmt.Println(`      case aidlerrors.ExceptionSecurity:`)
		fmt.Println(`          // permission denied`)
		fmt.Println(`      case aidlerrors.ExceptionIllegalArgument:`)
		fmt.Println(`          // bad input`)
		fmt.Println(`      case aidlerrors.ExceptionServiceSpecific:`)
		fmt.Println(`          // service-defined code in se.ServiceSpecificCode`)
		fmt.Println(`      case aidlerrors.ExceptionTransactionFailed:`)
		fmt.Println(`          // transport-level failure`)
		fmt.Println(`      }`)
		fmt.Println(`  }`)
	}
	fmt.Println()

	// ---------------------------------------------------------------
	// 4. Graceful degradation: probe several services, skip unavailable.
	// ---------------------------------------------------------------
	fmt.Println("=== Graceful Degradation ===")

	targets := []servicemanager.ServiceName{
		servicemanager.ActivityService,
		servicemanager.PowerService,
		servicemanager.WindowService,
		servicemanager.ClipboardService,
		"nonexistent_service_1",
		"nonexistent_service_2",
	}

	var available []servicemanager.ServiceName
	for _, name := range targets {
		svc, err := sm.CheckService(ctx, name)
		if err != nil {
			fmt.Printf("  %-30s error: %v\n", name, err)
			continue
		}
		if svc == nil {
			fmt.Printf("  %-30s unavailable (skipping)\n", name)
			continue
		}
		fmt.Printf("  %-30s handle=%d, alive=%v\n", name, svc.Handle(), svc.IsAlive(ctx))
		available = append(available, name)
	}

	fmt.Printf("\nAvailable: %d / %d services\n", len(available), len(targets))
	for _, name := range available {
		fmt.Printf("  - %s\n", name)
	}
}

// handleTypedError demonstrates how to use errors.As to extract and
// switch on the AIDL exception code embedded in a StatusError.
func handleTypedError(err error) {
	var se *aidlerrors.StatusError
	if !errors.As(err, &se) {
		fmt.Printf("    (not an AIDL StatusError: %T)\n", err)
		return
	}

	fmt.Printf("    AIDL exception: %s\n", se.Exception)
	if se.Message != "" {
		fmt.Printf("    Message:        %s\n", se.Message)
	}

	switch se.Exception {
	case aidlerrors.ExceptionSecurity:
		fmt.Println("    -> Permission denied. Run with a higher-privilege UID or SELinux context.")
	case aidlerrors.ExceptionIllegalArgument:
		fmt.Println("    -> Bad argument. Verify the parameters match the AIDL interface.")
	case aidlerrors.ExceptionServiceSpecific:
		fmt.Printf("    -> Service-specific error code: %d\n", se.ServiceSpecificCode)
	case aidlerrors.ExceptionTransactionFailed:
		fmt.Println("    -> Transaction failed. The service may have died or the binder buffer is full.")
	default:
		fmt.Printf("    -> Unhandled exception code: %d\n", int32(se.Exception))
	}
}
