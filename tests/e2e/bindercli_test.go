//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	emulatorPath    = "/home/streaming/Android/Sdk/emulator/emulator"
	avdName         = "Pixel_7_API_35"
	bindercliBinary   = "/tmp/bindercli-test"
	deviceBinaryDir = "/data/local/tmp"
	deviceBinary    = deviceBinaryDir + "/bindercli"
	bootTimeout     = 120 * time.Second
	bootPollPeriod  = 2 * time.Second
)

// emulatorSerial is the adb serial for the emulator (e.g., "emulator-5554").
// All adb commands use -s to target this specific device,
// so other connected devices are not affected.
var emulatorSerial string

// emulatorStartedByTest tracks whether TestMain started the emulator,
// so cleanup only kills it if we own it.
var emulatorStartedByTest bool

// onDevice reports whether the test binary is running directly on an
// Android device (i.e. /dev/binder is accessible). When true, the
// emulator/adb setup is skipped — on-device tests open /dev/binder
// directly, and bindercli tests exec the binary directly.
var onDevice bool

// bindercliAvailable is true when the bindercli binary exists at
// deviceBinary. On-device, this requires the binary to have been
// pushed separately; on-host it is built and pushed by TestMain.
var bindercliAvailable bool

func TestMain(m *testing.M) {
	// Running as root (UID 0) on Android bypasses SELinux service_manager
	// restrictions, granting unrestricted binder access to ALL HAL services
	// including IKeyMintDevice. Calling methods like DeleteAllKeys() with
	// zero-value arguments permanently destroys hardware-backed encryption
	// keys, making the device unable to boot (verified by reproduction:
	// UID 2000 → GetService blocked by SELinux → reboot OK;
	// UID 0 → DeleteAllKeys succeeded → device bricked).
	//
	// This commonly happens when `adb root` was run in a previous session
	// and adbd is still running as root. Refuse to run in this state.
	if os.Getuid() == 0 {
		os.Stderr.WriteString("FATAL: refusing to run E2E tests as root (UID 0).\n")
		os.Stderr.WriteString("Running as root bypasses SELinux and allows destructive\n")
		os.Stderr.WriteString("HAL calls (e.g. IKeyMintDevice.DeleteAllKeys) that WILL\n")
		os.Stderr.WriteString("brick the device. Run `adb unroot` first.\n")
		os.Exit(1)
	}

	if _, err := os.Stat("/dev/binder"); err == nil {
		// Running on the device itself — skip emulator setup.
		// Bindercli tests will exec the binary directly if present.
		onDevice = true
		if _, err := os.Stat(deviceBinary); err == nil {
			bindercliAvailable = true
		}
		os.Exit(m.Run())
	}

	// Running on the host — need emulator for aidlcli tests.
	serial := findEmulator()
	if serial == "" {
		startEmulator()
		emulatorStartedByTest = true
		serial = waitForEmulatorSerial()
		if serial == "" {
			logAndExit("emulator started but no emulator serial found in adb devices")
		}
	}
	emulatorSerial = serial

	if err := waitForBoot(); err != nil {
		logAndExit("emulator boot timeout: " + err.Error())
	}

	if err := buildBindercli(); err != nil {
		logAndExit("build bindercli: " + err.Error())
	}

	if err := pushBindercli(); err != nil {
		logAndExit("push bindercli: " + err.Error())
	}

	code := m.Run()

	if emulatorStartedByTest {
		_ = adbCmd("emu", "kill").Run()
	}

	os.Exit(code)
}

// findEmulator scans `adb devices` for an already-running emulator
// and returns its serial (e.g., "emulator-5554"), or "" if none found.
func findEmulator() string {
	out, err := exec.Command("adb", "devices").CombinedOutput()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == "device" && strings.HasPrefix(fields[0], "emulator-") {
			return fields[0]
		}
	}
	return ""
}

// waitForEmulatorSerial polls `adb devices` until an emulator appears.
func waitForEmulatorSerial() string {
	deadline := time.Now().Add(bootTimeout)
	for time.Now().Before(deadline) {
		serial := findEmulator()
		if serial != "" {
			return serial
		}
		time.Sleep(bootPollPeriod)
	}
	return ""
}

// adbCmd creates an exec.Command targeting the emulator via `adb -s <serial>`.
func adbCmd(args ...string) *exec.Cmd {
	fullArgs := append([]string{"-s", emulatorSerial}, args...)
	return exec.Command("adb", fullArgs...)
}

// startEmulator launches the emulator headlessly in the background.
func startEmulator() {
	cmd := exec.Command(
		emulatorPath,
		"-avd", avdName,
		"-no-window",
		"-no-audio",
		"-no-boot-anim",
		"-gpu", "swiftshader_indirect",
	)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		logAndExit("starting emulator: " + err.Error())
	}
	// Detach — we will kill via `adb -s <serial> emu kill` later.
	go func() { _ = cmd.Wait() }()
}

// waitForBoot polls `adb -s <serial> shell getprop sys.boot_completed` until "1".
func waitForBoot() error {
	deadline := time.Now().Add(bootTimeout)
	for time.Now().Before(deadline) {
		out, err := adbCmd("shell", "getprop", "sys.boot_completed").CombinedOutput()
		if err == nil && strings.TrimSpace(string(out)) == "1" {
			return nil
		}
		time.Sleep(bootPollPeriod)
	}
	return os.ErrDeadlineExceeded
}

// buildBindercli cross-compiles the CLI for the emulator (x86_64 Linux).
func buildBindercli() error {
	cmd := exec.Command(
		"go", "build",
		"-o", bindercliBinary,
		"./cmd/bindercli/",
	)
	cmd.Env = append(os.Environ(),
		"GOOS=linux",
		"GOARCH=amd64",
		"CGO_ENABLED=0",
	)
	cmd.Dir = "/home/streaming/go/src/github.com/xaionaro-go/binder"
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// pushBindercli copies the built binary to the emulator and makes it executable.
func pushBindercli() error {
	if out, err := adbCmd("push", bindercliBinary, deviceBinary).CombinedOutput(); err != nil {
		return &pushError{output: string(out), err: err}
	}
	if out, err := adbCmd("shell", "chmod", "755", deviceBinary).CombinedOutput(); err != nil {
		return &pushError{output: string(out), err: err}
	}
	return nil
}

type pushError struct {
	output string
	err    error
}

func (e *pushError) Error() string {
	return e.err.Error() + ": " + e.output
}

func (e *pushError) Unwrap() error {
	return e.err
}

// logAndExit writes to stderr and exits with code 1.
// Used only in TestMain where t.Fatal is unavailable.
func logAndExit(msg string) {
	os.Stderr.WriteString("FATAL: " + msg + "\n")
	if emulatorStartedByTest && emulatorSerial != "" {
		_ = adbCmd("emu", "kill").Run()
	}
	os.Exit(1)
}

// runBindercli executes bindercli and returns stdout, stderr, error.
// When running on the host, it uses `adb -s <serial> shell` to execute
// on the emulator. When running on-device, it execs the binary directly.
// It always injects --format json for machine-parseable output.
func runBindercli(args ...string) (string, string, error) {
	if onDevice {
		fullArgs := append([]string{"--format", "json"}, args...)
		cmd := exec.Command(deviceBinary, fullArgs...)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		return stdout.String(), stderr.String(), err
	}

	// Host path: join into a single shell command string to preserve
	// empty/quoted values across the adb shell boundary.
	parts := make([]string, 0, len(args)+3)
	parts = append(parts, deviceBinary, "--format", "json")
	for _, a := range args {
		parts = append(parts, shellQuote(a))
	}
	shellCmd := strings.Join(parts, " ")
	cmd := adbCmd("shell", shellCmd)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// shellQuote wraps a string in single quotes for shell safety.
// Single quotes within the string are escaped.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if !strings.ContainsAny(s, " \t\n'\"\\$`!") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// runBindercliOrSkip runs bindercli; skips the test when the service is unavailable.
func runBindercliOrSkip(t *testing.T, args ...string) string {
	t.Helper()
	if onDevice && !bindercliAvailable {
		t.Skip("bindercli binary not found on device at " + deviceBinary)
	}
	stdout, stderr, err := runBindercli(args...)
	if err != nil {
		combined := stderr + stdout
		switch {
		case strings.Contains(combined, "not found"),
			strings.Contains(combined, "no service with descriptor"):
			t.Skipf("service unavailable: %s", strings.TrimSpace(combined))
		}
		t.Fatalf("bindercli %v failed: %v\nstdout: %s\nstderr: %s", args, err, stdout, stderr)
	}
	return stdout
}

// --- Core service tests ---

func TestBindercli_ServiceList(t *testing.T) {
	stdout := runBindercliOrSkip(t, "service", "list")

	var rows []map[string]string
	require.NoError(t, json.Unmarshal([]byte(stdout), &rows), "unmarshal service list")
	require.NotEmpty(t, rows, "expected at least one service")

	names := make([]string, 0, len(rows))
	for _, row := range rows {
		name, ok := row["NAME"]
		require.True(t, ok, "row missing NAME field: %v", row)
		_, hasStatus := row["STATUS"]
		assert.True(t, hasStatus, "row missing STATUS field: %v", row)
		names = append(names, name)
	}

	t.Logf("found %d services", len(names))

	// At least one well-known service should be present.
	foundWellKnown := false
	for _, n := range names {
		if n == "SurfaceFlinger" || n == "activity" {
			foundWellKnown = true
			break
		}
	}
	assert.True(t, foundWellKnown, "expected SurfaceFlinger or activity in service list")
}

func TestBindercli_ServiceInspect(t *testing.T) {
	stdout := runBindercliOrSkip(t, "service", "inspect", "SurfaceFlinger")

	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &result), "unmarshal inspect result")

	assert.Equal(t, "SurfaceFlinger", result["name"])

	alive, ok := result["alive"].(bool)
	require.True(t, ok, "alive field should be bool, got %T", result["alive"])
	assert.True(t, alive, "SurfaceFlinger should be alive")

	descriptor, _ := result["descriptor"].(string)
	// Descriptor may be empty if the service doesn't respond to InterfaceTransaction.
	t.Logf("descriptor: %q", descriptor)

	t.Logf("SurfaceFlinger: handle=%v alive=%v descriptor=%s", result["handle"], alive, descriptor)
}

// --- GPS/Location ---

func TestBindercli_Location_GetAllProviders(t *testing.T) {
	stdout := runBindercliOrSkip(t,
		"android.location.ILocationManager", "get-all-providers",
	)

	var envelope map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope), "unmarshal response")

	raw, ok := envelope["result"]
	require.True(t, ok, "response missing 'result' key")

	providers, ok := raw.([]any)
	require.True(t, ok, "result should be array, got %T", raw)
	require.NotEmpty(t, providers, "expected at least one provider")

	providerStrings := make([]string, 0, len(providers))
	for _, p := range providers {
		s, ok := p.(string)
		require.True(t, ok, "provider should be string, got %T", p)
		providerStrings = append(providerStrings, s)
	}

	assert.Contains(t, providerStrings, "passive", "passive provider should always be available")
	t.Logf("providers: %v", providerStrings)
}

func TestBindercli_Location_IsProviderEnabled(t *testing.T) {
	// The userId parameter was absorbed into the proxy's Identity() call;
	// the CLI method now only takes --provider.
	stdout := runBindercliOrSkip(t,
		"android.location.ILocationManager", "is-provider-enabled-for-user",
		"--provider", "passive",
	)

	var envelope map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope), "unmarshal response")

	raw, ok := envelope["result"]
	require.True(t, ok, "response missing 'result' key")

	_, ok = raw.(bool)
	assert.True(t, ok, "result should be bool, got %T", raw)
	t.Logf("isProviderEnabledForUser(passive): %v", raw)
}

func TestBindercli_Location_GetGnssHardwareModelName(t *testing.T) {
	stdout, stderr, err := runBindercli(
		"android.location.ILocationManager", "get-gnss-hardware-model-name",
	)
	if err != nil {
		combined := stderr + stdout
		if strings.Contains(combined, "not found") ||
			strings.Contains(combined, "no service with descriptor") {
			t.Skipf("GNSS not available: %s", strings.TrimSpace(combined))
		}
		// GNSS might just return an error on emulator — skip gracefully.
		t.Skipf("GNSS hardware model name unavailable: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	var envelope map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope), "unmarshal response")

	raw, ok := envelope["result"]
	require.True(t, ok, "response missing 'result' key")

	_, ok = raw.(string)
	assert.True(t, ok, "result should be string, got %T", raw)
	t.Logf("gnssHardwareModelName: %v", raw)
}

// --- ActivityManager ---

func TestBindercli_ActivityManager_IsUserAMonkey(t *testing.T) {
	stdout := runBindercliOrSkip(t,
		"android.app.IActivityManager", "is-user-a-monkey",
	)

	var envelope map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope), "unmarshal response")

	raw, ok := envelope["result"]
	require.True(t, ok, "response missing 'result' key")

	val, ok := raw.(bool)
	require.True(t, ok, "result should be bool, got %T", raw)
	assert.False(t, val, "should not be a monkey in test")
	t.Logf("isUserAMonkey: %v", val)
}

func TestBindercli_ActivityManager_GetProcessLimit(t *testing.T) {
	stdout := runBindercliOrSkip(t,
		"android.app.IActivityManager", "get-process-limit",
	)

	var envelope map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope), "unmarshal response")

	raw, ok := envelope["result"]
	require.True(t, ok, "response missing 'result' key")

	// JSON numbers unmarshal as float64.
	_, ok = raw.(float64)
	assert.True(t, ok, "result should be numeric, got %T", raw)
	t.Logf("getProcessLimit: %v", raw)
}

func TestBindercli_ActivityManager_CheckPermission(t *testing.T) {
	stdout := runBindercliOrSkip(t,
		"android.app.IActivityManager", "check-permission",
		"--permission", "android.permission.INTERNET",
		"--pid", "1",
		"--uid", "0",
	)

	var envelope map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope), "unmarshal response")

	raw, ok := envelope["result"]
	require.True(t, ok, "response missing 'result' key")

	val, ok := raw.(float64)
	require.True(t, ok, "result should be numeric, got %T", raw)
	assert.Equal(t, float64(0), val, "root should have INTERNET permission (0 = PERMISSION_GRANTED)")
	t.Logf("checkPermission(INTERNET, pid=1, uid=0): %v", val)
}

func TestBindercli_ActivityManager_IsAppFreezerSupported(t *testing.T) {
	stdout := runBindercliOrSkip(t,
		"android.app.IActivityManager", "is-app-freezer-supported",
	)

	var envelope map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope), "unmarshal response")

	raw, ok := envelope["result"]
	require.True(t, ok, "response missing 'result' key")

	_, ok = raw.(bool)
	assert.True(t, ok, "result should be bool, got %T", raw)
	t.Logf("isAppFreezerSupported: %v", raw)
}

// --- PackageManager ---

func TestBindercli_PackageManager_IsPackageAvailable(t *testing.T) {
	// The userId parameter was absorbed into the proxy's Identity() call.
	stdout := runBindercliOrSkip(t,
		"android.content.pm.IPackageManager", "is-package-available",
		"--packageName", "com.android.settings",
	)

	var envelope map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope), "unmarshal response")

	raw, ok := envelope["result"]
	require.True(t, ok, "response missing 'result' key")

	val, ok := raw.(bool)
	require.True(t, ok, "result should be bool, got %T", raw)
	assert.True(t, val, "com.android.settings should be available")
	t.Logf("isPackageAvailable(com.android.settings): %v", val)
}

func TestBindercli_PackageManager_CheckPermission(t *testing.T) {
	// The userId parameter was absorbed into the proxy's Identity() call.
	stdout := runBindercliOrSkip(t,
		"android.content.pm.IPackageManager", "check-permission",
		"--permName", "android.permission.INTERNET",
		"--pkgName", "com.android.settings",
	)

	var envelope map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope), "unmarshal response")

	raw, ok := envelope["result"]
	require.True(t, ok, "response missing 'result' key")

	_, ok = raw.(float64)
	assert.True(t, ok, "result should be numeric, got %T", raw)
	t.Logf("checkPermission(INTERNET, com.android.settings): %v", raw)
}

// --- Power/Battery/Thermal ---

func TestBindercli_PowerManager_IsPowerSaveMode(t *testing.T) {
	stdout := runBindercliOrSkip(t,
		"android.os.IPowerManager", "is-power-save-mode",
	)

	var envelope map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope), "unmarshal response")

	raw, ok := envelope["result"]
	require.True(t, ok, "response missing 'result' key")

	_, ok = raw.(bool)
	assert.True(t, ok, "result should be bool, got %T", raw)
	t.Logf("isPowerSaveMode: %v", raw)
}

func TestBindercli_PowerManager_IsInteractive(t *testing.T) {
	stdout := runBindercliOrSkip(t,
		"android.os.IPowerManager", "is-interactive",
	)

	var envelope map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope), "unmarshal response")

	raw, ok := envelope["result"]
	require.True(t, ok, "response missing 'result' key")

	_, ok = raw.(bool)
	assert.True(t, ok, "result should be bool, got %T", raw)
	t.Logf("isInteractive: %v", raw)
}

func TestBindercli_ThermalService_GetCurrentThermalStatus(t *testing.T) {
	stdout := runBindercliOrSkip(t,
		"android.os.IThermalService", "get-current-thermal-status",
	)

	var envelope map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope), "unmarshal response")

	raw, ok := envelope["result"]
	require.True(t, ok, "response missing 'result' key")

	_, ok = raw.(float64)
	assert.True(t, ok, "result should be numeric, got %T", raw)
	t.Logf("getCurrentThermalStatus: %v", raw)
}

// --- Display ---

func TestBindercli_Display_GetDisplayIds(t *testing.T) {
	stdout := runBindercliOrSkip(t,
		"android.hardware.display.IDisplayManager", "get-display-ids",
		"--includeDisabled", "false",
	)

	var envelope map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope), "unmarshal response")

	raw, ok := envelope["result"]
	require.True(t, ok, "response missing 'result' key")

	arr, ok := raw.([]any)
	require.True(t, ok, "result should be array, got %T", raw)
	require.NotEmpty(t, arr, "expected at least one display ID")
	t.Logf("getDisplayIds: %v", arr)
}

// --- Clipboard ---

func TestBindercli_Clipboard_HasClipboardText(t *testing.T) {
	// Identity params (callingPackage, attributionTag, userId) were absorbed
	// into the proxy's Identity() call; only deviceId remains as a method arg.
	stdout := runBindercliOrSkip(t,
		"android.content.IClipboard", "has-clipboard-text",
		"--deviceId=0",
	)

	var envelope map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope), "unmarshal response")

	raw, ok := envelope["result"]
	require.True(t, ok, "response missing 'result' key")

	_, ok = raw.(bool)
	assert.True(t, ok, "result should be bool, got %T", raw)
	t.Logf("hasClipboardText: %v", raw)
}

// --- Core service commands (additional) ---

func TestBindercli_ServiceMethods(t *testing.T) {
	stdout := runBindercliOrSkip(t, "service", "methods", "activity")

	var envelope map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope), "unmarshal response")

	descriptor, ok := envelope["descriptor"].(string)
	require.True(t, ok, "response missing 'descriptor' string")
	assert.NotEmpty(t, descriptor, "descriptor should not be empty")

	methods, ok := envelope["methods"].([]any)
	require.True(t, ok, "response missing 'methods' array")
	require.NotEmpty(t, methods, "expected at least one method")

	// Verify the first method has a name field.
	firstMethod, ok := methods[0].(map[string]any)
	require.True(t, ok, "method entry should be an object")
	name, ok := firstMethod["name"].(string)
	require.True(t, ok, "method should have a 'name' string")
	assert.NotEmpty(t, name, "method name should not be empty")

	t.Logf("activity interface %s has %d methods, first: %s", descriptor, len(methods), name)
}

// --- Location (additional) ---

func TestBindercli_Location_GetGnssYearOfHardware(t *testing.T) {
	stdout, stderr, err := runBindercli(
		"android.location.ILocationManager", "get-gnss-year-of-hardware",
	)
	if err != nil {
		combined := stderr + stdout
		switch {
		case strings.Contains(combined, "not found"),
			strings.Contains(combined, "no service with descriptor"):
			t.Skipf("location service not available: %s", strings.TrimSpace(combined))
		}
		// GNSS year might not be available on emulator.
		t.Skipf("GNSS year of hardware unavailable: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	var envelope map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope), "unmarshal response")

	raw, ok := envelope["result"]
	require.True(t, ok, "response missing 'result' key")

	val, ok := raw.(float64)
	require.True(t, ok, "result should be numeric, got %T", raw)

	// Sanity check: year should be a reasonable value (0 means unknown, otherwise 2000+).
	year := int(val)
	if year != 0 {
		assert.GreaterOrEqual(t, year, 2000, "year should be >= 2000 if set")
	}
	t.Logf("getGnssYearOfHardware: %d", year)
}

// --- PackageManager (additional) ---

func TestBindercli_PackageManager_GetInstallerPackageName(t *testing.T) {
	stdout := runBindercliOrSkip(t,
		"android.content.pm.IPackageManager", "get-installer-package-name",
		"--packageName=com.android.settings",
	)

	var envelope map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope), "unmarshal response")

	_, ok := envelope["result"]
	require.True(t, ok, "response missing 'result' key")

	// The result may be null/empty if Settings was not installed via a store.
	t.Logf("getInstallerPackageName(com.android.settings): %v", envelope["result"])
}

// --- Telephony (additional) ---

func TestBindercli_Telephony_GetNetworkCountryIso(t *testing.T) {
	stdout, stderr, err := runBindercli(
		"com.android.internal.telephony.ITelephony", "get-network-country-iso-for-phone",
		"--phoneId=0",
	)
	if err != nil {
		combined := stderr + stdout
		switch {
		case strings.Contains(combined, "not found"),
			strings.Contains(combined, "no service with descriptor"):
			t.Skipf("telephony service not available: %s", strings.TrimSpace(combined))
		}
		t.Skipf("telephony unavailable: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	var envelope map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope), "unmarshal response")

	raw, ok := envelope["result"]
	require.True(t, ok, "response missing 'result' key")

	// Result should be a string (ISO country code, may be empty).
	val, ok := raw.(string)
	require.True(t, ok, "result should be string, got %T", raw)

	if val != "" {
		assert.Len(t, val, 2, "country ISO should be a 2-letter code")
	}
	t.Logf("getNetworkCountryIsoForPhone(0): %q", val)
}
