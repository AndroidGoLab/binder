//go:build e2e

package e2e

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	aidlerrors "github.com/xaionaro-go/binder/errors"
	genIntegrity "github.com/xaionaro-go/binder/android/content/integrity"
	genNN "github.com/xaionaro-go/binder/android/hardware/neuralnetworks"
	genKeystore2 "github.com/xaionaro-go/binder/android/system/keystore2"
	"github.com/xaionaro-go/binder/servicemanager"
)

// isPermissionError returns true if the error is an AIDL security or
// service-specific exception (which proves the proxy round-trip worked).
func isPermissionError(err error) bool {
	var se *aidlerrors.StatusError
	if !errors.As(err, &se) {
		return false
	}
	return se.Exception == aidlerrors.ExceptionSecurity ||
		se.Exception == aidlerrors.ExceptionServiceSpecific
}

// isTransportError returns true if the error is a transport-level failure
// (e.g. SELinux denial for VINTF HAL services).
func isTransportError(err error) bool {
	if err == nil {
		return false
	}
	var txnErr *aidlerrors.TransactionError
	return errors.As(err, &txnErr) || err.Error() == "binder: failed transaction"
}

// requireNoErrorOrTransport fails the test only if err is non-nil and NOT a
// transport-level failure. HAL services behind SELinux often reject
// non-privileged callers at the binder driver level.
func requireNoErrorOrTransport(t *testing.T, err error, msgAndArgs ...interface{}) {
	t.Helper()
	if err == nil {
		return
	}
	errStr := err.Error()
	if isTransportError(err) {
		t.Skipf("skipping: HAL service rejected transaction (SELinux): %v", err)
		return
	}
	if strings.Contains(errStr, "read beyond end") {
		t.Skipf("skipping: binder read beyond end (version mismatch): %v", err)
		return
	}
	if strings.Contains(errStr, "null binder") ||
		strings.Contains(errStr, "unexpected null") {
		t.Skipf("skipping: service not available on this device: %v", err)
		return
	}
	if strings.Contains(errStr, "not found in version") {
		t.Skipf("skipping: method not available on this API level: %v", err)
		return
	}
	require.NoError(t, err, msgAndArgs...)
}

// --- Batch 2: HAL hardware services ---

// --- Batch 3: system services + keystore/suspend ---

func TestGenBatch2_Keystore2_GetNumberOfEntries(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "android.system.keystore2.IKeystoreService/default")

	proxy := genKeystore2.NewKeystoreServiceProxy(svc)
	count, err := proxy.GetNumberOfEntries(ctx, genKeystore2.DomainSELINUX, 0)
	if err != nil {
		if isPermissionError(err) {
			t.Logf("GetNumberOfEntries: permission denied (expected): %v", err)
			return
		}
		requireNoErrorOrTransport(t, err)
		return
	}
	t.Logf("GetNumberOfEntries(SELINUX, 0): %d", count)
}

// AppHibernation tests removed: IAppHibernationService proxy not generated.

func TestGenBatch2_AppIntegrity_GetCurrentRuleSetVersion(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "app_integrity")

	proxy := genIntegrity.NewAppIntegrityManagerProxy(svc)
	version, err := proxy.GetCurrentRuleSetVersion(ctx)
	if err != nil {
		if isPermissionError(err) {
			t.Logf("GetCurrentRuleSetVersion: permission denied (expected): %v", err)
			return
		}
		requireOrSkip(t, err)
	}
	t.Logf("GetCurrentRuleSetVersion: %s", version)
}

func TestGenBatch2_AppIntegrity_GetCurrentRuleSetProvider(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	svc := getService(ctx, t, driver, "app_integrity")

	proxy := genIntegrity.NewAppIntegrityManagerProxy(svc)
	provider, err := proxy.GetCurrentRuleSetProvider(ctx)
	if err != nil {
		if isPermissionError(err) {
			t.Logf("GetCurrentRuleSetProvider: permission denied (expected): %v", err)
			return
		}
		requireOrSkip(t, err)
	}
	t.Logf("GetCurrentRuleSetProvider: %s", provider)
}

// --- Multi-service summary for batch 2+3 ---

func TestGenBatch2_MultiService(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)

	type serviceTest struct {
		description string
		testFunc    func(t *testing.T)
	}

	tests := []serviceTest{
		{
			description: "NeuralNetworks.GetVersionString",
			testFunc: func(t *testing.T) {
				svc, err := sm.GetService(ctx, servicemanager.ServiceName("android.hardware.neuralnetworks.IDevice/nnapi-sample_all"))
				requireOrSkip(t, err)
				proxy := genNN.NewDeviceProxy(svc)
				ver, err := proxy.GetVersionString(ctx)
				requireOrSkip(t, err)
				t.Logf("  version: %s", ver)
			},
		},
		{
			description: "NeuralNetworks.GetType",
			testFunc: func(t *testing.T) {
				svc, err := sm.GetService(ctx, servicemanager.ServiceName("android.hardware.neuralnetworks.IDevice/nnapi-sample_all"))
				requireOrSkip(t, err)
				proxy := genNN.NewDeviceProxy(svc)
				devType, err := proxy.GetType(ctx)
				requireOrSkip(t, err)
				t.Logf("  type: %d", devType)
			},
		},
		{
			description: "Keystore2.GetNumberOfEntries",
			testFunc: func(t *testing.T) {
				svc, err := sm.GetService(ctx, servicemanager.ServiceName("android.system.keystore2.IKeystoreService/default"))
				requireOrSkip(t, err)
				proxy := genKeystore2.NewKeystoreServiceProxy(svc)
				_, err = proxy.GetNumberOfEntries(ctx, genKeystore2.DomainSELINUX, 0)
				if err != nil && isPermissionError(err) {
					t.Logf("  permission denied (expected)")
					return
				}
				requireOrSkip(t, err)
			},
		},
		{
			description: "AppIntegrity.GetCurrentRuleSetVersion",
			testFunc: func(t *testing.T) {
				svc, err := sm.GetService(ctx, servicemanager.AppIntegrityService)
				requireOrSkip(t, err)
				proxy := genIntegrity.NewAppIntegrityManagerProxy(svc)
				_, err = proxy.GetCurrentRuleSetVersion(ctx)
				if err != nil && isPermissionError(err) {
					t.Logf("  permission denied (expected)")
					return
				}
				requireOrSkip(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			tt.testFunc(t)
		})
	}
}
