// List all installed packages on the device.
//
// Build:
//
//	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o build/list_packages ./examples/list_packages/
//	adb push build/list_packages /data/local/tmp/ && adb shell /data/local/tmp/list_packages
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/AndroidGoLab/binder/android/content/pm"
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

	svc, err := sm.GetService(ctx, servicemanager.ServiceName("package"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "get package service: %v\n", err)
		os.Exit(1)
	}

	pkgMgr := pm.NewPackageManagerProxy(svc)

	// List all installed packages.
	packages, err := pkgMgr.GetAllPackages(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "get all packages: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Installed packages (%d total):\n", len(packages))
	for _, pkg := range packages {
		fmt.Printf("  %s\n", pkg)
	}

	// Show packages for the current process UID.
	uid := int32(os.Getuid())
	uidPackages, err := pkgMgr.GetPackagesForUid(ctx, uid)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nget packages for uid %d: %v\n", uid, err)
		os.Exit(1)
	}

	fmt.Printf("\nPackages for UID %d (%d total):\n", uid, len(uidPackages))
	for _, pkg := range uidPackages {
		fmt.Printf("  %s\n", pkg)
	}
}
