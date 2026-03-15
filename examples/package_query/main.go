// Query installed packages: check availability, get installer, SDK target.
//
// Build:
//
//	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o build/package_query ./examples/package_query/
//	adb push package_query /data/local/tmp/ && adb shell /data/local/tmp/package_query
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/xaionaro-go/binder/binder"
	"github.com/xaionaro-go/binder/binder/versionaware"
	"github.com/xaionaro-go/binder/android/content/pm"
	"github.com/xaionaro-go/binder/kernelbinder"
	"github.com/xaionaro-go/binder/servicemanager"
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

	// Check some well-known packages
	packages := []string{
		"com.android.settings",
		"com.android.systemui",
		"com.android.launcher3",
		"com.android.chrome",
		"com.google.android.gms",
		"com.example.nonexistent",
	}

	fmt.Println("Package availability:")
	for _, pkg := range packages {
		available, err := pkgMgr.IsPackageAvailable(ctx, pkg)
		if err != nil {
			fmt.Printf("  %-40s error: %v\n", pkg, err)
			continue
		}
		fmt.Printf("  %-40s %v\n", pkg, available)
	}

	// Query details for installed packages
	fmt.Println("\nPackage details:")
	for _, pkg := range packages {
		sdk, err := pkgMgr.GetTargetSdkVersion(ctx, pkg)
		if err != nil {
			continue
		}
		installer, err := pkgMgr.GetInstallerPackageName(ctx, pkg)
		if err != nil {
			installer = "(unknown)"
		}
		fmt.Printf("  %-40s SDK %d  installer: %s\n", pkg, sdk, installer)
	}

	// Use the native package manager for faster queries
	nativeSvc, err := sm.GetService(ctx, servicemanager.ServiceName("package_native"))
	if err != nil {
		return
	}
	nativePkgMgr := pm.NewPackageManagerNativeProxy(nativeSvc)

	for _, pkg := range packages[:3] {
		installer, err := nativePkgMgr.GetInstallerForPackage(ctx, pkg)
		if err != nil {
			continue
		}
		fmt.Printf("  %-40s native installer: %s\n", pkg, installer)
	}
}
