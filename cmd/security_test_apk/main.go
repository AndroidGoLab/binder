// Binary security_test_apk probes whether an app-sandboxed process can
// reach critical HAL binder services. All calls are read-only and
// non-destructive. Intended for security research.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
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

// serviceProbe defines a single binder service to probe.
type serviceProbe struct {
	// ServiceName is the name passed to ServiceManager.CheckService.
	ServiceName string
	// Descriptor is the AIDL interface descriptor for the service.
	Descriptor string
	// Method is the AIDL method name to call (read-only).
	Method string
	// BuildData builds the parcel for the method call. If nil, an
	// empty parcel (with just the interface token) is sent.
	BuildData func(p *parcel.Parcel)
}

var probes = []serviceProbe{
	{
		ServiceName: "android.hardware.security.keymint.IKeyMintDevice/default",
		Descriptor:  "android.hardware.security.keymint.IKeyMintDevice",
		Method:      "getHardwareInfo",
	},
	{
		ServiceName: "android.hardware.usb.IUsb/default",
		Descriptor:  "android.hardware.usb.IUsb",
		Method:      "queryPortStatus",
		BuildData: func(p *parcel.Parcel) {
			// queryPortStatus(long transactionId) — use 0.
			p.WriteInt64(0)
		},
	},
	{
		ServiceName: "android.hardware.boot.IBootControl/default",
		Descriptor:  "android.hardware.boot.IBootControl",
		Method:      "getCurrentSlot",
	},
	{
		ServiceName: "installd",
		Descriptor:  "android.os.IInstalld",
		Method:      "isQuotaSupported",
		BuildData: func(p *parcel.Parcel) {
			// isQuotaSupported(String volumeUuid) — use empty string.
			p.WriteString16("")
		},
	},
}

func main() {
	ctx := context.Background()
	l := logrus.Default().WithLevel(logger.LevelDebug)
	ctx = belt.CtxWithBelt(ctx, belt.New())
	ctx = logger.CtxWithLogger(ctx, l)

	fmt.Println("=== Binder HAL Security Probe ===")
	fmt.Printf("PID: %d  UID: %d\n", os.Getpid(), os.Getuid())
	fmt.Println()

	results := runProbes(ctx)

	fmt.Println()
	fmt.Println("=== Summary ===")
	for _, r := range results {
		fmt.Println(r)
	}
}

func runProbes(ctx context.Context) []string {
	var results []string

	// Open the binder driver.
	driver, err := kernelbinder.Open(ctx, binder.WithMapSize(128*1024))
	if err != nil {
		msg := fmt.Sprintf("/dev/binder open: FAILED (%v)", err)
		fmt.Println(msg)
		return []string{msg}
	}
	defer driver.Close(ctx)
	fmt.Println("/dev/binder open: SUCCESS")
	results = append(results, "/dev/binder open: SUCCESS")

	// Create version-aware transport.
	transport, err := versionaware.NewTransport(ctx, driver, 0)
	if err != nil {
		msg := fmt.Sprintf("VersionAwareTransport: FAILED (%v)", err)
		fmt.Println(msg)
		results = append(results, msg)
		return results
	}
	defer transport.Close(ctx)
	fmt.Println("VersionAwareTransport: SUCCESS")
	results = append(results, "VersionAwareTransport: SUCCESS")

	sm := servicemanager.New(transport)

	// First, list what services are visible at all.
	results = append(results, probeListServices(ctx, sm)...)
	fmt.Println()

	// Probe each target service.
	for _, probe := range probes {
		r := probeService(ctx, sm, transport, probe)
		results = append(results, r...)
		fmt.Println()
	}

	return results
}

func probeListServices(ctx context.Context, sm *servicemanager.ServiceManager) []string {
	var results []string

	services, err := sm.ListServices(ctx)
	if err != nil {
		msg := fmt.Sprintf("ListServices: FAILED (%v)", err)
		fmt.Println(msg)
		results = append(results, msg)
		return results
	}

	fmt.Printf("ListServices: SUCCESS (%d services visible)\n", len(services))
	results = append(results, fmt.Sprintf("ListServices: SUCCESS (%d services visible)", len(services)))

	// Check which of our target services appear in the list.
	serviceSet := make(map[string]bool, len(services))
	for _, s := range services {
		serviceSet[string(s)] = true
	}

	for _, probe := range probes {
		visible := serviceSet[probe.ServiceName]
		status := "NOT LISTED"
		if visible {
			status = "LISTED"
		}
		msg := fmt.Sprintf("  %s: %s", probe.ServiceName, status)
		fmt.Println(msg)
		results = append(results, msg)
	}

	return results
}

func probeService(
	ctx context.Context,
	sm *servicemanager.ServiceManager,
	transport *versionaware.Transport,
	probe serviceProbe,
) []string {
	var results []string
	header := fmt.Sprintf("[%s]", probe.ServiceName)
	fmt.Println(header)

	// Step 1: CheckService lookup.
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	svcBinder, err := sm.CheckService(ctx, servicemanager.ServiceName(probe.ServiceName))
	if err != nil {
		msg := fmt.Sprintf("%s CheckService: FAILED (%v)", header, err)
		fmt.Println(msg)
		return append(results, msg)
	}
	if svcBinder == nil {
		msg := fmt.Sprintf("%s CheckService: NOT FOUND (service not registered)", header)
		fmt.Println(msg)
		return append(results, msg)
	}
	msg := fmt.Sprintf("%s CheckService: SUCCESS (handle=%d)", header, svcBinder.Handle())
	fmt.Println(msg)
	results = append(results, msg)

	// Step 2: Call the read-only method.
	code, err := transport.ResolveCode(ctx, probe.Descriptor, probe.Method)
	if err != nil {
		msg := fmt.Sprintf("%s ResolveCode(%s): FAILED (%v)", header, probe.Method, err)
		fmt.Println(msg)
		return append(results, msg)
	}

	data := parcel.New()
	data.WriteInterfaceToken(probe.Descriptor)
	if probe.BuildData != nil {
		probe.BuildData(data)
	}

	reply, err := svcBinder.Transact(ctx, code, 0, data)
	if err != nil {
		msg := fmt.Sprintf("%s %s: FAILED (%v)", header, probe.Method, err)
		fmt.Println(msg)
		results = append(results, msg)

		// Classify the error for the security report.
		errStr := err.Error()
		switch {
		case strings.Contains(errStr, "PERMISSION_DENIED") ||
			strings.Contains(errStr, "permission"):
			results = append(results, "  -> ACCESS DENIED (sandbox blocks this)")
		case strings.Contains(errStr, "DEAD_OBJECT") ||
			strings.Contains(errStr, "dead"):
			results = append(results, "  -> SERVICE DEAD")
		default:
			results = append(results, fmt.Sprintf("  -> ERROR TYPE: %T", err))
		}
		return results
	}

	// Try to read the AIDL status from the reply.
	if err := binder.ReadStatus(reply); err != nil {
		msg := fmt.Sprintf("%s %s: STATUS ERROR (%v)", header, probe.Method, err)
		fmt.Println(msg)
		results = append(results, msg)
		return results
	}

	msg = fmt.Sprintf("%s %s: SUCCESS (reply %d bytes)", header, probe.Method, reply.Len())
	fmt.Println(msg)
	results = append(results, msg)

	// For KeyMint, try to parse the hardware info for extra detail.
	if probe.Descriptor == "android.hardware.security.keymint.IKeyMintDevice" {
		results = append(results, parseKeyMintHardwareInfo(header, reply)...)
	}

	return results
}

func parseKeyMintHardwareInfo(header string, reply *parcel.Parcel) []string {
	var results []string

	// Read parcelable header.
	endPos, err := readParcelableHeader(reply)
	if err != nil {
		return results
	}

	versionNumber, err := reply.ReadInt32()
	if err != nil {
		return results
	}

	secLevel, err := reply.ReadInt32()
	if err != nil {
		return results
	}

	name, err := reply.ReadString16()
	if err != nil {
		return results
	}

	author, err := reply.ReadString16()
	if err != nil {
		return results
	}

	_ = endPos
	msg := fmt.Sprintf("%s  HardwareInfo: version=%d secLevel=%d name=%q author=%q",
		header, versionNumber, secLevel, name, author)
	fmt.Println(msg)
	results = append(results, msg)
	return results
}

func readParcelableHeader(p *parcel.Parcel) (int, error) {
	startPos := p.Position()
	headerSize, err := p.ReadInt32()
	if err != nil {
		return 0, err
	}
	return startPos + int(headerSize), nil
}
