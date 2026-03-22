//go:build linux

package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"unicode/utf16"

	"github.com/electricbubble/gadb"
	"github.com/facebookincubator/go-belt/tool/logger"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"

	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/binder/versionaware"
	"github.com/AndroidGoLab/binder/interop/gadb/proxy"
	"github.com/AndroidGoLab/binder/parcel"
	"github.com/AndroidGoLab/binder/servicemanager"
)

// runRemoteMode creates a gadb proxy Session to the device, builds a
// remote-aware ServiceManager, and serves the same MCP tools as device
// mode over stdio.
func runRemoteMode(
	cmd *cobra.Command,
	_ []string,
) (_err error) {
	ctx := cmd.Context()
	logger.Tracef(ctx, "runRemoteMode")
	defer func() { logger.Tracef(ctx, "/runRemoteMode: %v", _err) }()

	serial, err := cmd.Flags().GetString("serial")
	if err != nil {
		return fmt.Errorf("reading --serial: %w", err)
	}

	// Auto-discover device serial when not specified.
	if serial == "" {
		serial, err = discoverDeviceSerial()
		if err != nil {
			return fmt.Errorf("auto-discovering device: %w", err)
		}
		logger.Debugf(ctx, "auto-discovered device serial: %s", serial)
	}

	// Detect the device's API level via adb getprop.
	apiLevel, err := detectRemoteAPILevel(serial)
	if err != nil {
		return fmt.Errorf("detecting API level: %w", err)
	}
	logger.Debugf(ctx, "detected device API level: %d", apiLevel)

	// Create the proxy session: cross-compile daemon, push, start, forward, connect.
	logger.Debugf(ctx, "creating proxy session to device %s", serial)
	session, err := proxy.NewSession(ctx, serial)
	if err != nil {
		return fmt.Errorf("creating proxy session: %w", err)
	}
	defer func() {
		logger.Debugf(ctx, "closing proxy session")
		if closeErr := session.Close(ctx); closeErr != nil {
			logger.Warnf(ctx, "closing proxy session: %v", closeErr)
		}
	}()

	rt := session.Transport()

	// Resolve the version table by probing the device through the daemon.
	// Multiple revisions may exist per API level with different transaction
	// codes; probing determines which revision matches the device.
	table, err := probeRemoteVersionTable(ctx, rt, apiLevel)
	if err != nil {
		return err
	}

	sm := newRemoteServiceManager(rt, table)

	tools := &ToolSet{sm: sm}

	mcpServer := server.NewMCPServer(
		"binder-mcp",
		"1.0.0",
		server.WithToolCapabilities(false),
		server.WithRecovery(),
	)

	tools.Register(mcpServer)

	logger.Debugf(ctx, "serving MCP over stdio (remote mode)")

	errLogger := log.New(os.Stderr, "binder-mcp: ", log.LstdFlags)

	return server.ServeStdio(mcpServer, server.WithErrorLogger(errLogger))
}

// discoverDeviceSerial connects to the ADB server and returns the serial
// of the first connected device.
func discoverDeviceSerial() (string, error) {
	client, err := gadb.NewClient()
	if err != nil {
		return "", fmt.Errorf("connecting to ADB server: %w", err)
	}

	devices, err := client.DeviceList()
	if err != nil {
		return "", fmt.Errorf("listing devices: %w", err)
	}

	if len(devices) == 0 {
		return "", fmt.Errorf("no devices connected")
	}

	return devices[0].Serial(), nil
}

// detectRemoteAPILevel queries the device's ro.build.version.sdk property
// via adb shell to determine the Android API level.
func detectRemoteAPILevel(serial string) (int, error) {
	client, err := gadb.NewClient()
	if err != nil {
		return 0, fmt.Errorf("connecting to ADB server: %w", err)
	}

	devices, err := client.DeviceList()
	if err != nil {
		return 0, fmt.Errorf("listing devices: %w", err)
	}

	for _, dev := range devices {
		if dev.Serial() != serial {
			continue
		}

		output, err := dev.RunShellCommand("getprop", "ro.build.version.sdk")
		if err != nil {
			return 0, fmt.Errorf("getprop ro.build.version.sdk: %w", err)
		}

		level, err := strconv.Atoi(strings.TrimSpace(output))
		if err != nil {
			return 0, fmt.Errorf("parsing API level %q: %w", output, err)
		}

		return level, nil
	}

	return 0, fmt.Errorf("device %q not found", serial)
}

// probeRemoteVersionTable determines the correct version table for the
// device by trying each candidate revision's listServices transaction code
// through the remote transport. The first revision that produces a valid
// reply (status OK + parseable count) wins.
func probeRemoteVersionTable(
	ctx context.Context,
	rt *proxy.RemoteTransport,
	apiLevel int,
) (versionaware.VersionTable, error) {
	// Deduplicate listServices codes across revisions: if two revisions
	// share the same code, probing once suffices.
	type candidate struct {
		revision versionaware.Revision
		table    versionaware.VersionTable
		code     binder.TransactionCode
	}

	var candidates []candidate
	seenCodes := map[binder.TransactionCode]bool{}

	for _, level := range []int{apiLevel, versionaware.DefaultAPILevel} {
		for _, rev := range versionaware.Revisions[level] {
			table, ok := versionaware.Tables[rev]
			if !ok {
				continue
			}
			code := table.Resolve(serviceManagerDescriptor, "listServices")
			if code == 0 || seenCodes[code] {
				continue
			}
			seenCodes[code] = true
			candidates = append(candidates, candidate{rev, table, code})
		}
	}

	for _, c := range candidates {
		if probeListServices(ctx, rt, c.code) {
			logger.Debugf(ctx, "matched revision %s (listServices code %d)", c.revision, c.code)
			return c.table, nil
		}
		logger.Debugf(ctx, "revision %s (listServices code %d) did not match", c.revision, c.code)
	}

	return nil, fmt.Errorf(
		"no compiled version table matched device at API level %d (supported: %v)",
		apiLevel, supportedLevels(),
	)
}

// probeListServices sends a listServices transaction with the given code
// and returns true if the reply contains a valid status and service count.
func probeListServices(
	ctx context.Context,
	rt *proxy.RemoteTransport,
	code binder.TransactionCode,
) bool {
	data := parcel.New()
	defer data.Recycle()
	data.WriteInterfaceToken(serviceManagerDescriptor)
	data.WriteInt32(dumpFlagPriorityAll)

	reply, err := rt.Transact(ctx, serviceManagerDescriptor, uint32(code), 0, data)
	if err != nil {
		return false
	}
	defer reply.Recycle()

	if err := binder.ReadStatus(reply); err != nil {
		return false
	}

	count, err := reply.ReadInt32()
	if err != nil {
		return false
	}

	return count > 0
}

// supportedLevels returns the API levels that have compiled version tables.
func supportedLevels() []int {
	levels := make([]int, 0, len(versionaware.Revisions))
	for level := range versionaware.Revisions {
		levels = append(levels, level)
	}
	return levels
}

const (
	serviceManagerDescriptor = "android.os.IServiceManager"

	// dumpFlagPriorityAll combines all priority flags, matching the
	// device-mode ServiceManager constant.
	dumpFlagPriorityAll = int32(1 | 2 | 4 | 8)

	// maxRemoteServiceCount guards against corrupted reply parcels.
	maxRemoteServiceCount = int32(10000)
)

// remoteServiceManager implements ServiceLookup by sending binder
// transactions through a RemoteTransport to the device-side daemon.
//
// Descriptor discovery is lazy: CheckService returns a remoteIBinder
// immediately, and the AIDL descriptor is resolved on demand (via the
// well-known map or INTERFACE_TRANSACTION probing). This keeps
// operations like list_services fast.
type remoteServiceManager struct {
	rt    *proxy.RemoteTransport
	table versionaware.VersionTable

	// descriptorCache maps service name → AIDL descriptor. Populated
	// lazily and shared across all remoteIBinder instances.
	mu              sync.Mutex
	descriptorCache map[servicemanager.ServiceName]string
}

func newRemoteServiceManager(
	rt *proxy.RemoteTransport,
	table versionaware.VersionTable,
) *remoteServiceManager {
	return &remoteServiceManager{
		rt:              rt,
		table:           table,
		descriptorCache: make(map[servicemanager.ServiceName]string),
	}
}

func (rsm *remoteServiceManager) ListServices(
	ctx context.Context,
) (_ []servicemanager.ServiceName, _err error) {
	logger.Tracef(ctx, "remoteServiceManager.ListServices")
	defer func() { logger.Tracef(ctx, "/remoteServiceManager.ListServices: %v", _err) }()

	code := rsm.table.Resolve(serviceManagerDescriptor, "listServices")
	if code == 0 {
		return nil, fmt.Errorf("cannot resolve listServices transaction code")
	}

	data := parcel.New()
	defer data.Recycle()
	data.WriteInterfaceToken(serviceManagerDescriptor)
	data.WriteInt32(dumpFlagPriorityAll)

	reply, err := rsm.rt.Transact(ctx, serviceManagerDescriptor, uint32(code), 0, data)
	if err != nil {
		return nil, fmt.Errorf("listServices: %w", err)
	}
	defer reply.Recycle()

	if err := binder.ReadStatus(reply); err != nil {
		return nil, fmt.Errorf("listServices: %w", err)
	}

	count, err := reply.ReadInt32()
	if err != nil {
		return nil, fmt.Errorf("listServices: reading count: %w", err)
	}

	if count < 0 || count > maxRemoteServiceCount {
		return nil, fmt.Errorf("listServices: invalid service count: %d", count)
	}

	services := make([]servicemanager.ServiceName, 0, count)
	for i := int32(0); i < count; i++ {
		name, err := reply.ReadString16()
		if err != nil {
			return nil, fmt.Errorf("listServices: reading service %d: %w", i, err)
		}
		services = append(services, servicemanager.ServiceName(name))
	}

	return services, nil
}

func (rsm *remoteServiceManager) CheckService(
	ctx context.Context,
	name servicemanager.ServiceName,
) (_ binder.IBinder, _err error) {
	logger.Tracef(ctx, "remoteServiceManager.CheckService(%q)", name)
	defer func() { logger.Tracef(ctx, "/remoteServiceManager.CheckService(%q): %v", name, _err) }()

	// Verify the service is registered via CheckService RPC.
	registered, err := rsm.isServiceRegistered(ctx, name)
	if err != nil {
		return nil, err
	}

	if !registered {
		return nil, nil
	}

	// Return a lazy IBinder. Descriptor discovery happens on first
	// Transact/ResolveCode call, not here. This keeps bulk operations
	// like list_services (which call CheckService + IsAlive for each
	// service) fast.
	return &remoteIBinder{
		rsm:         rsm,
		serviceName: name,
	}, nil
}

// isServiceRegistered calls CheckService on the remote ServiceManager and
// returns true if a non-null binder handle was returned.
func (rsm *remoteServiceManager) isServiceRegistered(
	ctx context.Context,
	name servicemanager.ServiceName,
) (bool, error) {
	code := rsm.table.Resolve(serviceManagerDescriptor, "checkService")
	if code == 0 {
		return false, fmt.Errorf("cannot resolve checkService transaction code")
	}

	data := parcel.New()
	defer data.Recycle()
	data.WriteInterfaceToken(serviceManagerDescriptor)
	data.WriteString16(string(name))

	reply, err := rsm.rt.Transact(ctx, serviceManagerDescriptor, uint32(code), 0, data)
	if err != nil {
		return false, fmt.Errorf("checkService(%q): %w", name, err)
	}
	defer reply.Recycle()

	if err := binder.ReadStatus(reply); err != nil {
		return false, fmt.Errorf("checkService(%q): %w", name, err)
	}

	_, ok, err := reply.ReadNullableStrongBinder()
	if err != nil {
		return false, fmt.Errorf("checkService(%q): reading binder: %w", name, err)
	}

	return ok, nil
}

// resolveDescriptor returns the cached AIDL descriptor for a service name,
// or discovers it via the well-known map and INTERFACE_TRANSACTION probing.
func (rsm *remoteServiceManager) resolveDescriptor(
	ctx context.Context,
	name servicemanager.ServiceName,
) (string, error) {
	rsm.mu.Lock()
	defer rsm.mu.Unlock()

	if desc, ok := rsm.descriptorCache[name]; ok {
		return desc, nil
	}

	desc, err := rsm.discoverDescriptor(ctx, name)
	if err != nil {
		return "", err
	}

	rsm.descriptorCache[name] = desc
	return desc, nil
}

// discoverDescriptor finds the AIDL interface descriptor for a named service.
// Uses two strategies:
//  1. Well-known service name → descriptor map (instant).
//  2. INTERFACE_TRANSACTION probing for well-known descriptors only.
//
// Does not try all 1750+ descriptors from the version table — that would
// be prohibitively slow over the network.
func (rsm *remoteServiceManager) discoverDescriptor(
	ctx context.Context,
	name servicemanager.ServiceName,
) (string, error) {
	// Strategy 1: well-known map.
	if desc, ok := wellKnownDescriptors[string(name)]; ok {
		logger.Debugf(ctx, "descriptor for %q: well-known %q", name, desc)
		return desc, nil
	}

	// Strategy 2: probe INTERFACE_TRANSACTION for a subset of likely
	// descriptors. The daemon caches resolved descriptors, so each probe
	// is a single round-trip after the first scan.
	for _, desc := range descriptorGuesses(string(name)) {
		probed, err := probeInterfaceTransaction(ctx, rsm.rt, desc)
		if err != nil {
			continue
		}
		if probed == desc {
			logger.Debugf(ctx, "descriptor for %q: probed %q", name, desc)
			return desc, nil
		}
	}

	return "", fmt.Errorf("cannot discover AIDL descriptor for service %q", name)
}

// probeInterfaceTransaction sends INTERFACE_TRANSACTION to the daemon for
// the given descriptor and returns the descriptor string from the reply.
func probeInterfaceTransaction(
	ctx context.Context,
	rt *proxy.RemoteTransport,
	descriptor string,
) (string, error) {
	data := parcel.New()
	defer data.Recycle()

	reply, err := rt.Transact(
		ctx,
		descriptor,
		uint32(binder.InterfaceTransaction),
		0,
		data,
	)
	if err != nil {
		return "", err
	}
	defer reply.Recycle()

	return reply.ReadString16()
}

// descriptorGuesses generates a small set of likely AIDL descriptors for a
// service name based on Android naming conventions.
func descriptorGuesses(serviceName string) []string {
	// Common patterns: "foo" → "android.os.IFooManager" etc.
	// Only try a handful of plausible patterns.
	base := strings.ReplaceAll(serviceName, "_", "")
	upper := strings.ToUpper(base[:1]) + base[1:]

	return []string{
		"android.os.I" + upper + "Manager",
		"android.os.I" + upper + "Service",
		"android.os.I" + upper,
		"android.app.I" + upper + "Manager",
		"android.app.I" + upper + "Service",
		"android.app.I" + upper,
	}
}

// wellKnownDescriptors maps Android service names to their AIDL interface
// descriptors. Covers the most commonly used system services.
var wellKnownDescriptors = map[string]string{
	"accessibility":       "android.view.accessibility.IAccessibilityManager",
	"account":             "android.accounts.IAccountManager",
	"activity":            "android.app.IActivityManager",
	"activity_task":       "android.app.IActivityTaskManager",
	"alarm":               "android.app.IAlarmManager",
	"appops":              "android.app.IAppOpsService",
	"audio":               "android.media.IAudioService",
	"autofill":            "android.view.autofill.IAutoFillManager",
	"battery":             "android.os.IBatteryPropertiesRegistrar",
	"batterystats":        "com.android.internal.app.IBatteryStats",
	"bluetooth_manager":   "android.bluetooth.IBluetoothManager",
	"camera":              "android.hardware.ICameraService",
	"clipboard":           "android.content.IClipboard",
	"connectivity":        "android.net.IConnectivityManager",
	"content":             "android.content.IContentService",
	"device_policy":       "android.app.admin.IDevicePolicyManager",
	"display":             "android.hardware.display.IDisplayManager",
	"dreams":              "android.service.dreams.IDreamManager",
	"dropbox":             "com.android.internal.os.IDropBoxManagerService",
	"input":               "android.hardware.input.IInputManager",
	"input_method":        "com.android.internal.view.IInputMethodManager",
	"iphonesubinfo":       "com.android.internal.telephony.IPhoneSubInfo",
	"isms":                "com.android.internal.telephony.ISms",
	"jobscheduler":        "android.app.job.IJobScheduler",
	"location":            "android.location.ILocationManager",
	"media.audio_policy":  "android.media.IAudioPolicyService",
	"media.camera":        "android.hardware.ICameraService",
	"media_session":       "android.media.session.ISessionManager",
	"mount":               "android.os.storage.IStorageManager",
	"netd":                "android.os.INetworkManagementService",
	"network_management":  "android.os.INetworkManagementService",
	"notification":        "android.app.INotificationManager",
	"package":             "android.content.pm.IPackageManager",
	"permission":          "android.os.IPermissionController",
	"phone":               "com.android.internal.telephony.ITelephony",
	"power":               "android.os.IPowerManager",
	"role":                "android.app.role.IRoleManager",
	"sensorservice":       "android.gui.ISensorServer",
	"statusbar":           "com.android.internal.statusbar.IStatusBarService",
	"telecom":             "com.android.internal.telecom.ITelecomService",
	"telephony.registry":  "com.android.internal.telephony.ITelephonyRegistry",
	"uimode":              "android.app.IUiModeManager",
	"usb":                 "android.hardware.usb.IUsbManager",
	"user":                "android.os.IUserManager",
	"vibrator":            "android.os.IVibratorService",
	"vibrator_manager":    "android.os.IVibratorManagerService",
	"wallpaper":           "android.app.IWallpaperManager",
	"wifi":                "android.net.wifi.IWifiManager",
	"wifip2p":             "android.net.wifi.p2p.IWifiP2pManager",
	"window":              "android.view.IWindowManager",
}

// remoteIBinder implements binder.IBinder by routing all transactions
// through the RemoteTransport using the service's AIDL descriptor.
//
// Descriptor discovery is lazy: the descriptor is resolved on the first
// call to Transact or ResolveCode, not at construction time. IsAlive
// returns true unconditionally since the service was just confirmed
// registered via CheckService.
type remoteIBinder struct {
	rsm         *remoteServiceManager
	serviceName servicemanager.ServiceName

	// descriptor is populated lazily by ensureDescriptor.
	once       sync.Once
	descriptor string
	descErr    error
}

// ensureDescriptor triggers lazy descriptor resolution.
func (b *remoteIBinder) ensureDescriptor(ctx context.Context) error {
	b.once.Do(func() {
		b.descriptor, b.descErr = b.rsm.resolveDescriptor(ctx, b.serviceName)
	})
	return b.descErr
}

func (b *remoteIBinder) Transact(
	ctx context.Context,
	code binder.TransactionCode,
	flags binder.TransactionFlags,
	data *parcel.Parcel,
) (_ *parcel.Parcel, _err error) {
	logger.Tracef(ctx, "remoteIBinder.Transact(svc=%s, code=%d)", b.serviceName, code)
	defer func() { logger.Tracef(ctx, "/remoteIBinder.Transact: %v", _err) }()

	// For regular AIDL calls the parcel starts with an interface token
	// containing the descriptor. Use it when available — it may differ
	// from the service's own descriptor (e.g., querying a sub-interface).
	if token := peekInterfaceToken(data); token != "" {
		return b.rsm.rt.Transact(ctx, token, uint32(code), uint32(flags), data)
	}

	// Meta-transactions (PING, INTERFACE_TRANSACTION) carry no token.
	// Resolve the service's descriptor to route through the daemon.
	if err := b.ensureDescriptor(ctx); err != nil {
		return nil, fmt.Errorf("resolving descriptor for %q: %w", b.serviceName, err)
	}

	return b.rsm.rt.Transact(ctx, b.descriptor, uint32(code), uint32(flags), data)
}

func (b *remoteIBinder) ResolveCode(
	_ context.Context,
	descriptor string,
	method string,
) (binder.TransactionCode, error) {
	code := b.rsm.table.Resolve(descriptor, method)
	if code == 0 {
		return 0, fmt.Errorf(
			"remote: method %s.%s not found in version table",
			descriptor, method,
		)
	}
	return code, nil
}

func (b *remoteIBinder) Handle() uint32 {
	return 0
}

// IsAlive returns true unconditionally. The service was confirmed
// registered by CheckService immediately before this IBinder was created.
// Sending a PING through the daemon would require the descriptor, which
// triggers lazy discovery — too expensive for bulk liveness checks.
func (b *remoteIBinder) IsAlive(_ context.Context) bool {
	return true
}

func (b *remoteIBinder) LinkToDeath(_ context.Context, _ binder.DeathRecipient) error {
	return fmt.Errorf("remote: LinkToDeath not supported")
}

func (b *remoteIBinder) UnlinkToDeath(_ context.Context, _ binder.DeathRecipient) error {
	return fmt.Errorf("remote: UnlinkToDeath not supported")
}

func (b *remoteIBinder) Cookie() uintptr {
	return 0
}

func (b *remoteIBinder) Transport() binder.VersionAwareTransport {
	return nil
}

func (b *remoteIBinder) Identity() binder.CallerIdentity {
	return binder.DefaultCallerIdentity()
}

var _ binder.IBinder = (*remoteIBinder)(nil)

// peekInterfaceToken extracts the AIDL descriptor from a parcel's
// interface token without consuming the parcel data. Returns "" if
// the parcel is too short or the token cannot be read.
//
// Interface token layout (all little-endian):
//
//	offset 0:  int32 strict-mode policy
//	offset 4:  int32 work-source UID
//	offset 8:  int32 vendor header
//	offset 12: int32 char count (UTF-16 characters, excluding null terminator)
//	offset 16: UTF-16LE encoded string
func peekInterfaceToken(p *parcel.Parcel) string {
	if p == nil {
		return ""
	}

	raw := p.Data()

	// 4 int32 header fields = 16 bytes minimum.
	if len(raw) < 16 {
		return ""
	}

	charCount := int32(binary.LittleEndian.Uint32(raw[12:16]))
	if charCount <= 0 || charCount > 1024 {
		return ""
	}

	// UTF-16LE: (charCount + 1) * 2 bytes (+1 for null terminator).
	strBytes := int(charCount+1) * 2
	if len(raw) < 16+strBytes {
		return ""
	}

	units := make([]uint16, charCount)
	for i := range units {
		units[i] = binary.LittleEndian.Uint16(raw[16+i*2:])
	}

	return string(utf16.Decode(units))
}
