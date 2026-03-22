// Register a Go service with the ServiceManager and call it back.
//
// Build:
//
//	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o build/server_service ./examples/server_service/
//	adb push server_service /data/local/tmp/ && adb shell /data/local/tmp/server_service
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/binder/versionaware"
	"github.com/AndroidGoLab/binder/kernelbinder"
	"github.com/AndroidGoLab/binder/parcel"
	"github.com/AndroidGoLab/binder/servicemanager"
)

const (
	pingServiceDescriptor = "com.example.IPingService"
	pingServiceName       = "com.example.ping"

	codePing = binder.FirstCallTransaction + 0
	codeEcho = binder.FirstCallTransaction + 1
)

// pingService implements binder.TransactionReceiver for a simple
// ping/echo service.
type pingService struct{}

func (s *pingService) Descriptor() string {
	return pingServiceDescriptor
}

func (s *pingService) OnTransaction(
	ctx context.Context,
	code binder.TransactionCode,
	data *parcel.Parcel,
) (*parcel.Parcel, error) {
	if _, err := data.ReadInterfaceToken(); err != nil {
		return nil, err
	}

	switch code {
	case codePing:
		reply := parcel.New()
		binder.WriteStatus(reply, nil)
		reply.WriteString16("pong")
		return reply, nil

	case codeEcho:
		msg, err := data.ReadString16()
		if err != nil {
			reply := parcel.New()
			binder.WriteStatus(reply, fmt.Errorf("reading echo argument: %w", err))
			return reply, nil
		}

		reply := parcel.New()
		binder.WriteStatus(reply, nil)
		reply.WriteString16(msg)
		return reply, nil

	default:
		return nil, fmt.Errorf("unknown transaction code %d", code)
	}
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
		fmt.Fprintf(os.Stderr, "version-aware transport: %v\n", err)
		os.Exit(1)
	}

	sm := servicemanager.New(transport)

	// Register our ping service with the service manager.
	svc := &pingService{}
	if err := sm.AddService(ctx, servicemanager.ServiceName(pingServiceName), svc, false, 0); err != nil {
		fmt.Fprintf(os.Stderr, "add service: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Registered service %q\n", pingServiceName)

	// Self-test: look up the service we just registered and call it.
	remote, err := sm.GetService(ctx, servicemanager.ServiceName(pingServiceName))
	if err != nil {
		fmt.Fprintf(os.Stderr, "get service: %v\n", err)
		os.Exit(1)
	}

	// Call Ping (codePing).
	{
		data := parcel.New()
		defer data.Recycle()
		data.WriteInterfaceToken(pingServiceDescriptor)

		reply, err := remote.Transact(ctx, codePing, 0, data)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ping transact: %v\n", err)
			os.Exit(1)
		}
		defer reply.Recycle()

		if err := binder.ReadStatus(reply); err != nil {
			fmt.Fprintf(os.Stderr, "ping status: %v\n", err)
			os.Exit(1)
		}

		result, err := reply.ReadString16()
		if err != nil {
			fmt.Fprintf(os.Stderr, "ping read result: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Ping -> %q\n", result)
	}

	// Call Echo (codeEcho).
	{
		data := parcel.New()
		defer data.Recycle()
		data.WriteInterfaceToken(pingServiceDescriptor)
		data.WriteString16("hello from Go")

		reply, err := remote.Transact(ctx, codeEcho, 0, data)
		if err != nil {
			fmt.Fprintf(os.Stderr, "echo transact: %v\n", err)
			os.Exit(1)
		}
		defer reply.Recycle()

		if err := binder.ReadStatus(reply); err != nil {
			fmt.Fprintf(os.Stderr, "echo status: %v\n", err)
			os.Exit(1)
		}

		result, err := reply.ReadString16()
		if err != nil {
			fmt.Fprintf(os.Stderr, "echo read result: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Echo -> %q\n", result)
	}

	fmt.Println("All self-tests passed.")
}
