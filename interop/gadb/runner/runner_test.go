package runner_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/electricbubble/gadb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AndroidGoLab/binder/interop/gadb/runner"
)

const testDeviceSerial = "41041JEKB08092"

// requireADB skips the test if the ADB server is not reachable
// or no devices are connected.
func requireADB(t *testing.T) {
	t.Helper()
	client, err := gadb.NewClient()
	if err != nil {
		t.Skipf("ADB server not available: %v", err)
	}
	devices, err := client.DeviceList()
	if err != nil || len(devices) == 0 {
		t.Skipf("no ADB devices connected")
	}
}

func TestDiscoverDevices(t *testing.T) {
	requireADB(t)
	ctx := context.Background()

	devices, err := runner.DiscoverDevices(ctx)
	require.NoError(t, err, "DiscoverDevices must succeed when ADB server is running")
	require.NotEmpty(t, devices, "at least one device must be connected")

	var found bool
	for _, dev := range devices {
		t.Logf("device: serial=%s state=%s", dev.Serial, dev.State)
		if dev.Serial == testDeviceSerial {
			found = true
			assert.Equal(t, "online", dev.State)
		}
	}
	assert.True(t, found, "expected device %s not found", testDeviceSerial)
}

func TestPushAndRun(t *testing.T) {
	requireADB(t)

	ctx := context.Background()

	// Cross-compile examples/list_services for arm64.
	repoRoot := findRepoRoot(t)
	binaryPath := filepath.Join(t.TempDir(), "list_services")

	cmd := exec.CommandContext(ctx, "go", "build", "-o", binaryPath, "./examples/list_services/")
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=arm64", "CGO_ENABLED=0")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "cross-compile failed: %s", out)

	// Push and run.
	dr, err := runner.NewDeviceRunner(testDeviceSerial)
	require.NoError(t, err)

	const remotePath = "/data/local/tmp/list_services_test"

	err = dr.PushBinary(ctx, binaryPath, remotePath)
	require.NoError(t, err, "PushBinary must succeed")

	result, err := dr.Run(ctx, remotePath, 30*time.Second)
	require.NoError(t, err, "Run must succeed")

	t.Logf("exit=%d duration=%s stdout:\n%s", result.ExitCode, result.Duration, result.Stdout)
	assert.Equal(t, 0, result.ExitCode, "list_services should exit 0")
	assert.Contains(t, result.Stdout, "alive", "output should contain at least one alive service")

	err = dr.Cleanup(ctx, remotePath)
	require.NoError(t, err, "Cleanup must succeed")
}

func TestRunWithTimeout(t *testing.T) {
	requireADB(t)

	ctx := context.Background()

	dr, err := runner.NewDeviceRunner(testDeviceSerial)
	require.NoError(t, err)

	// "sleep 60" with a 2-second timeout should be killed.
	result, err := dr.Run(ctx, "sleep 60", 2*time.Second)
	require.NoError(t, err, "Run must succeed even on timeout")

	t.Logf("exit=%d duration=%s", result.ExitCode, result.Duration)
	// Android timeout returns 124 when the command is killed.
	assert.NotEqual(t, 0, result.ExitCode, "timed-out command should have non-zero exit code")
	assert.Less(t, result.Duration, 30*time.Second, "command should finish well before 30s")
}

// findRepoRoot walks up from the working directory to locate the go.mod file.
func findRepoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	require.NoError(t, err)

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	// Fall back: try to find via git.
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	require.NoError(t, err, "cannot find repo root")
	return strings.TrimSpace(string(out))
}
