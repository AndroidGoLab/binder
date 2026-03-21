// Binary keymint_delete_test calls DeleteAllKeys on the KeyMint HAL
// via binder IPC. WARNING: this is destructive and will delete all
// hardware-backed keys on the device.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/xaionaro-go/binder/android/hardware/security/keymint"
	"github.com/xaionaro-go/binder/binder"
	"github.com/xaionaro-go/binder/binder/versionaware"
	"github.com/xaionaro-go/binder/kernelbinder"
	"github.com/xaionaro-go/binder/servicemanager"
)

func main() {
	ctx := context.Background()

	driver, err := kernelbinder.Open(ctx, binder.WithMapSize(128*1024))
	if err != nil {
		fmt.Printf("FATAL: /dev/binder open failed: %v\n", err)
		os.Exit(1)
	}
	defer driver.Close(ctx)

	transport, err := versionaware.NewTransport(ctx, driver, 0)
	if err != nil {
		fmt.Printf("FATAL: VersionAwareTransport failed: %v\n", err)
		os.Exit(1)
	}
	defer transport.Close(ctx)

	sm := servicemanager.New(transport)

	const svcName = "android.hardware.security.keymint.IKeyMintDevice/default"
	svc, err := sm.GetService(ctx, servicemanager.ServiceName(svcName))
	if err != nil {
		fmt.Printf("GetService FAILED: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("GetService: OK (handle=%d)\n", svc.Handle())

	proxy := keymint.NewKeyMintDeviceProxy(svc)

	err = proxy.DeleteAllKeys(ctx)
	if err != nil {
		fmt.Printf("DeleteAllKeys: FAILED: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("DeleteAllKeys: SUCCESS")
}
