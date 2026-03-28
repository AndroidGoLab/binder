// Register a Go binder service using a generated AIDL stub.
//
// Same functionality as server_service (ping + echo), but uses the
// generated IPingService proxy and stub from com/example/ipingservice.go
// instead of raw OnTransaction + WriteString16. The AIDL source is at
// tools/pkg/3rdparty/examples/aidl/com/example/IPingService.aidl.
//
// Build:
//
//	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o build/server_service_aidl ./examples/server_service_aidl/
//	adb push build/server_service_aidl /data/local/tmp/ && adb shell /data/local/tmp/server_service_aidl
package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/binder/versionaware"
	"github.com/AndroidGoLab/binder/com/example"
	aidlerrors "github.com/AndroidGoLab/binder/errors"
	"github.com/AndroidGoLab/binder/kernelbinder"
	"github.com/AndroidGoLab/binder/servicemanager"
)

const serviceName = "com.example.ping"

// pingImpl implements the generated IPingServiceServer interface.
type pingImpl struct{}

func (p *pingImpl) Ping(ctx context.Context) (string, error) {
	return "pong", nil
}

func (p *pingImpl) Echo(ctx context.Context, message string) (string, error) {
	return message, nil
}

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
		fmt.Fprintf(os.Stderr, "transport: %v\n", err)
		os.Exit(1)
	}

	sm := servicemanager.New(transport)

	// Create a typed stub from our implementation.
	stub := example.NewPingServiceStub(&pingImpl{})

	// PingServiceStub is the TransactionReceiver for AddService.
	// It dispatches incoming transactions to the IPingService implementation.
	stubReceiver := &example.PingServiceStub{Impl: stub}

	err = sm.AddService(ctx, servicemanager.ServiceName(serviceName), stubReceiver, false, 0)

	switch {
	case err == nil:
		fmt.Printf("Registered service %q\n", serviceName)
		remoteTest(ctx, sm)
	default:
		var se *aidlerrors.StatusError
		if errors.As(err, &se) && se.Exception == aidlerrors.ExceptionSecurity {
			fmt.Println("AddService denied by SELinux (expected from shell).")
			fmt.Println("Falling back to in-process self-test.")
		} else {
			fmt.Fprintf(os.Stderr, "add service: %v\n", err)
			fmt.Fprintf(os.Stderr, "Falling back to in-process self-test.\n\n")
		}
		inProcessTest(ctx, stub)
	}
}

// remoteTest looks up the service and calls it through binder IPC.
func remoteTest(ctx context.Context, sm *servicemanager.ServiceManager) {
	remote, err := sm.GetService(ctx, servicemanager.ServiceName(serviceName))
	if err != nil {
		fmt.Fprintf(os.Stderr, "get service: %v\n", err)
		os.Exit(1)
	}

	proxy := example.NewPingServiceProxy(remote)

	result, err := proxy.Ping(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ping: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Ping -> %q\n", result)

	result, err = proxy.Echo(ctx, "hello from Go")
	if err != nil {
		fmt.Fprintf(os.Stderr, "echo: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Echo -> %q\n", result)

	fmt.Println("All remote tests passed.")
}

// inProcessTest calls the stub directly (no binder IPC).
func inProcessTest(ctx context.Context, stub example.IPingService) {
	result, err := stub.Ping(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ping: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Ping -> %q\n", result)

	result, err = stub.Echo(ctx, "hello from Go")
	if err != nil {
		fmt.Fprintf(os.Stderr, "echo: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Echo -> %q\n", result)

	fmt.Println("All in-process tests passed.")
}
