// Query user profiles from the UserManager service.
//
// Lists all user profiles, checks for headless system user mode,
// and queries the current user's serial number.
//
// Build:
//
//	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o build/user_manager ./examples/user_manager/
//	adb push build/user_manager /data/local/tmp/ && adb shell /data/local/tmp/user_manager
package main

import (
	"context"
	"fmt"
	"os"

	genOs "github.com/AndroidGoLab/binder/android/os"
	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/binder/versionaware"
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

	um, err := genOs.GetUserManager(ctx, sm)
	if err != nil {
		fmt.Fprintf(os.Stderr, "get user service: %v\n", err)
		os.Exit(1)
	}

	// Check headless system user mode.
	headless, err := um.IsHeadlessSystemUserMode(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "IsHeadlessSystemUserMode: %v\n", err)
	} else {
		fmt.Printf("Headless system user mode: %v\n", headless)
	}

	// Get main user ID.
	mainUID, err := um.GetMainUserId(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "GetMainUserId: %v\n", err)
	} else {
		fmt.Printf("Main user ID:              %d\n", mainUID)
	}

	// Get current user serial number.
	serial, err := um.GetUserSerialNumber(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "GetUserSerialNumber: %v\n", err)
	} else {
		fmt.Printf("Current user serial:       %d\n", serial)
	}

	// List all users (exclude dying, include partial and pre-created).
	users, err := um.GetUsers(ctx, false, true, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "GetUsers: %v\n", err)
	} else {
		fmt.Printf("\nUser profiles: %d\n", len(users))
		for i, u := range users {
			fmt.Printf("  [%d] id=%d name=%q\n", i+1, u.Id, u.Name)
		}
	}
}
