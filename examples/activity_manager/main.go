// Query the Activity Manager: process limits, permissions, running state.
//
// Build:
//
//	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o build/activity_manager ./examples/activity_manager/
//	adb push activity_manager /data/local/tmp/ && adb shell /data/local/tmp/activity_manager
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/xaionaro-go/binder/binder"
	"github.com/xaionaro-go/binder/binder/versionaware"
	"github.com/xaionaro-go/binder/android/app"
	"github.com/xaionaro-go/binder/kernelbinder"
	"github.com/xaionaro-go/binder/servicemanager"
)

const (
	permissionGranted = 0
	permissionDenied  = -1
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

	svc, err := sm.GetService(ctx, servicemanager.ActivityService)
	if err != nil {
		fmt.Fprintf(os.Stderr, "get activity service: %v\n", err)
		os.Exit(1)
	}

	am := app.NewActivityManagerProxy(svc)

	limit, err := am.GetProcessLimit(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "GetProcessLimit: %v\n", err)
	} else {
		fmt.Printf("Process limit:     %d\n", limit)
	}

	monkey, err := am.IsUserAMonkey(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "IsUserAMonkey: %v\n", err)
	} else {
		fmt.Printf("Is monkey test:    %v\n", monkey)
	}

	freezer, err := am.IsAppFreezerSupported(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "IsAppFreezerSupported: %v\n", err)
	} else {
		fmt.Printf("App freezer:       %v\n", freezer)
	}

	// Check permissions for our own process
	myPid := int32(os.Getpid())
	myUid := int32(os.Getuid())

	permissions := []string{
		"android.permission.INTERNET",
		"android.permission.CAMERA",
		"android.permission.READ_PHONE_STATE",
		"android.permission.ACCESS_FINE_LOCATION",
	}

	fmt.Printf("\nPermission check (pid=%d, uid=%d):\n", myPid, myUid)
	for _, perm := range permissions {
		result, err := am.CheckPermission(ctx, perm, myPid, myUid)
		if err != nil {
			fmt.Printf("  %-45s error: %v\n", perm, err)
			continue
		}
		status := "DENIED"
		if result == permissionGranted {
			status = "GRANTED"
		}
		fmt.Printf("  %-45s %s\n", perm, status)
	}
}
