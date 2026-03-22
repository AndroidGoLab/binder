// Binary getservice_vs_checkservice compares GetService vs CheckService
// for HAL binder services to investigate the discrepancy where CheckService
// returns NOT FOUND for services that GetService can retrieve.
//
// All method calls are READ-ONLY and non-destructive.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/facebookincubator/go-belt"
	"github.com/facebookincubator/go-belt/tool/logger"
	"github.com/facebookincubator/go-belt/tool/logger/implementation/logrus"

	"github.com/xaionaro-go/binder/binder"
	"github.com/xaionaro-go/binder/binder/versionaware"
	"github.com/xaionaro-go/binder/kernelbinder"
	"github.com/xaionaro-go/binder/parcel"
	"github.com/xaionaro-go/binder/servicemanager"
)

type serviceTest struct {
	Name       string
	Descriptor string
	Method     string // read-only method to call if handle obtained
	BuildData  func(p *parcel.Parcel)
}

var services = []serviceTest{
	{
		Name:       "android.hardware.security.keymint.IKeyMintDevice/default",
		Descriptor: "android.hardware.security.keymint.IKeyMintDevice",
		Method:     "getHardwareInfo",
	},
	{
		Name:       "android.hardware.usb.IUsb/default",
		Descriptor: "android.hardware.usb.IUsb",
		Method:     "queryPortStatus",
		BuildData: func(p *parcel.Parcel) {
			p.WriteInt64(0) // transactionId
		},
	},
	{
		Name:       "android.hardware.boot.IBootControl/default",
		Descriptor: "android.hardware.boot.IBootControl",
		Method:     "getCurrentSlot",
	},
	{
		Name:       "SurfaceFlinger",
		Descriptor: "android.ui.ISurfaceComposer",
		Method:     "", // skip method call for legacy interface
	},
	{
		Name:       "activity",
		Descriptor: "android.app.IActivityManager",
		Method:     "isUserAMonkey",
	},
}

func main() {
	ctx := context.Background()
	l := logrus.Default().WithLevel(logger.LevelDebug)
	ctx = belt.CtxWithBelt(ctx, belt.New())
	ctx = logger.CtxWithLogger(ctx, l)

	fmt.Println("========================================")
	fmt.Println("GetService vs CheckService Comparison")
	fmt.Println("========================================")
	fmt.Printf("PID: %d  UID: %d  GID: %d\n", os.Getpid(), os.Getuid(), os.Getgid())
	fmt.Println()

	// Read SELinux context
	seCtx, err := os.ReadFile("/proc/self/attr/current")
	if err != nil {
		fmt.Printf("SELinux context: ERROR (%v)\n", err)
	} else {
		fmt.Printf("SELinux context: %s\n", string(seCtx))
	}
	fmt.Println()

	// Open binder driver
	driver, err := kernelbinder.Open(ctx, binder.WithMapSize(128*1024))
	if err != nil {
		fmt.Printf("FATAL: /dev/binder open failed: %v\n", err)
		os.Exit(1)
	}
	defer driver.Close(ctx)
	fmt.Println("/dev/binder: OPEN OK")

	transport, err := versionaware.NewTransport(ctx, driver, 0)
	if err != nil {
		fmt.Printf("FATAL: VersionAwareTransport failed: %v\n", err)
		os.Exit(1)
	}
	defer transport.Close(ctx)
	fmt.Println("VersionAwareTransport: OK")
	fmt.Println()

	sm := servicemanager.New(transport)

	// First: list services to see what's visible
	fmt.Println("--- ListServices ---")
	svcList, err := sm.ListServices(ctx)
	if err != nil {
		fmt.Printf("ListServices: FAILED (%v)\n", err)
	} else {
		fmt.Printf("ListServices: %d services visible\n", len(svcList))
		// Check which target services are listed
		listed := make(map[string]bool)
		for _, s := range svcList {
			listed[string(s)] = true
		}
		for _, st := range services {
			if listed[st.Name] {
				fmt.Printf("  %s: LISTED\n", st.Name)
			} else {
				fmt.Printf("  %s: NOT LISTED\n", st.Name)
			}
		}
	}
	fmt.Println()

	// Test IsDeclared for each service
	fmt.Println("--- IsDeclared ---")
	for _, st := range services {
		declared, err := sm.IsDeclared(ctx, servicemanager.ServiceName(st.Name))
		if err != nil {
			fmt.Printf("  %s: ERROR (%v)\n", st.Name, err)
		} else {
			fmt.Printf("  %s: declared=%v\n", st.Name, declared)
		}
	}
	fmt.Println()

	// Now test CheckService vs GetService for each
	fmt.Println("--- CheckService vs GetService ---")
	for _, st := range services {
		fmt.Printf("\n[%s]\n", st.Name)

		// 1) CheckService (non-blocking)
		checkCtx, checkCancel := context.WithTimeout(ctx, 5*time.Second)
		checkBinder, checkErr := sm.CheckService(checkCtx, servicemanager.ServiceName(st.Name))
		checkCancel()

		if checkErr != nil {
			fmt.Printf("  CheckService: ERROR (%v)\n", checkErr)
		} else if checkBinder == nil {
			fmt.Printf("  CheckService: NOT FOUND (nil binder)\n")
		} else {
			fmt.Printf("  CheckService: SUCCESS (handle=%d)\n", checkBinder.Handle())
		}

		// 2) GetService (blocking, with timeout)
		getCtx, getCancel := context.WithTimeout(ctx, 10*time.Second)
		getBinder, getErr := sm.GetService(getCtx, servicemanager.ServiceName(st.Name))
		getCancel()

		if getErr != nil {
			fmt.Printf("  GetService:   ERROR (%v)\n", getErr)
		} else if getBinder == nil {
			fmt.Printf("  GetService:   NOT FOUND (nil binder)\n")
		} else {
			fmt.Printf("  GetService:   SUCCESS (handle=%d)\n", getBinder.Handle())
		}

		// 3) If either succeeded, try the read-only method
		var activeBinder binder.IBinder
		var source string
		if getBinder != nil {
			activeBinder = getBinder
			source = "GetService"
		} else if checkBinder != nil {
			activeBinder = checkBinder
			source = "CheckService"
		}

		if activeBinder != nil && st.Method != "" {
			fmt.Printf("  Calling %s (via %s handle=%d)...\n", st.Method, source, activeBinder.Handle())
			callReadOnlyMethod(ctx, transport, activeBinder, st)
		}

		// 4) Compare
		if checkBinder == nil && getBinder != nil {
			fmt.Printf("  *** DISCREPANCY: CheckService=nil but GetService=handle %d ***\n", getBinder.Handle())
			fmt.Printf("  This means GetService's startIfNotFound=true triggered lazy service start!\n")
		} else if checkBinder != nil && getBinder == nil {
			fmt.Printf("  *** DISCREPANCY: CheckService=handle %d but GetService=nil ***\n", checkBinder.Handle())
		} else if checkBinder == nil && getBinder == nil {
			fmt.Printf("  Both returned nil/error - service truly unavailable\n")
		} else {
			fmt.Printf("  Both succeeded (check handle=%d, get handle=%d)\n", checkBinder.Handle(), getBinder.Handle())
		}
	}

	fmt.Println()
	fmt.Println("========================================")
	fmt.Println("Done.")
}

func callReadOnlyMethod(
	ctx context.Context,
	transport *versionaware.Transport,
	svc binder.IBinder,
	st serviceTest,
) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	code, err := transport.ResolveCode(ctx, st.Descriptor, st.Method)
	if err != nil {
		fmt.Printf("    ResolveCode(%s): FAILED (%v)\n", st.Method, err)
		return
	}

	data := parcel.New()
	data.WriteInterfaceToken(st.Descriptor)
	if st.BuildData != nil {
		st.BuildData(data)
	}

	reply, err := svc.Transact(ctx, code, 0, data)
	if err != nil {
		fmt.Printf("    %s: TRANSACT FAILED (%v)\n", st.Method, err)
		return
	}

	if err := binder.ReadStatus(reply); err != nil {
		fmt.Printf("    %s: STATUS ERROR (%v)\n", st.Method, err)
		return
	}

	fmt.Printf("    %s: SUCCESS (reply %d bytes)\n", st.Method, reply.Len())

	// For KeyMint, parse hardware info
	if st.Descriptor == "android.hardware.security.keymint.IKeyMintDevice" {
		parseKeyMintInfo(reply)
	}
}

func parseKeyMintInfo(reply *parcel.Parcel) {
	// Parcelable header: int32 dataSize
	startPos := reply.Position()
	headerSize, err := reply.ReadInt32()
	if err != nil {
		fmt.Printf("    (could not parse HardwareInfo header: %v)\n", err)
		return
	}
	_ = startPos + int(headerSize)

	version, err := reply.ReadInt32()
	if err != nil {
		return
	}
	secLevel, err := reply.ReadInt32()
	if err != nil {
		return
	}
	name, err := reply.ReadString16()
	if err != nil {
		return
	}
	author, err := reply.ReadString16()
	if err != nil {
		return
	}
	fmt.Printf("    HardwareInfo: version=%d secLevel=%d name=%q author=%q\n",
		version, secLevel, name, author)
}
