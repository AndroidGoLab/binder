// List all registered Android binder services and ping each one.
//
// Build:
//
//	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o list_services ./examples/list_services/
//	adb push list_services /data/local/tmp/ && adb shell /data/local/tmp/list_services
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/xaionaro-go/aidl/binder"
	"github.com/xaionaro-go/aidl/kernelbinder"
	"github.com/xaionaro-go/aidl/servicemanager"
)

func main() {
	ctx := context.Background()

	driver, err := kernelbinder.Open(ctx, binder.WithMapSize(128*1024))
	if err != nil {
		fmt.Fprintf(os.Stderr, "open binder: %v\n", err)
		os.Exit(1)
	}
	defer driver.Close(ctx)

	sm := servicemanager.New(driver)

	services, err := sm.ListServices(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "list services: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Found %d registered services:\n\n", len(services))

	for i, name := range services {
		svc, err := sm.CheckService(ctx, name)
		status := "unreachable"
		if err == nil && svc != nil {
			if svc.IsAlive(ctx) {
				status = "alive"
			} else {
				status = "dead"
			}
		}
		fmt.Printf("  [%3d] %-60s %s\n", i+1, name, status)
	}
}
