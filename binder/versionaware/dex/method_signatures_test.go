package dex

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMethodSignatures_IActivityManager(t *testing.T) {
	const path = "/tmp/framework.jar"

	if _, err := os.Stat(path); err != nil {
		t.Skipf("skipping: %s not available: %v", path, err)
	}

	sigs, err := ExtractDescriptorSignaturesFromJAR(path, "android.app.IActivityManager")
	require.NoError(t, err)
	require.NotNil(t, sigs, "expected IActivityManager$Stub$Proxy in framework.jar")

	// isUserAMonkey takes no parameters.
	monkey, ok := sigs["isUserAMonkey"]
	require.True(t, ok, "expected isUserAMonkey in signatures")
	assert.Empty(t, monkey, "isUserAMonkey should have 0 params")

	// checkPermission takes (String, int, int).
	checkPerm, ok := sigs["checkPermission"]
	require.True(t, ok, "expected checkPermission in signatures")
	require.Len(t, checkPerm, 3, "checkPermission should have 3 params")
	assert.Equal(t, "Ljava/lang/String;", checkPerm[0])
	assert.Equal(t, "I", checkPerm[1])
	assert.Equal(t, "I", checkPerm[2])
}

func TestMethodSignatures_FullExtraction(t *testing.T) {
	const path = "/tmp/framework.jar"

	if _, err := os.Stat(path); err != nil {
		t.Skipf("skipping: %s not available: %v", path, err)
	}

	result, err := ExtractSignaturesFromJAR(path)
	require.NoError(t, err)
	require.NotEmpty(t, result, "expected at least one $Stub$Proxy class")

	// IActivityManager should be present in the full extraction.
	sigs, ok := result["android.app.IActivityManager"]
	require.True(t, ok, "expected android.app.IActivityManager in results")
	assert.Greater(t, len(sigs), 50, "IActivityManager should have many methods")

	// Verify consistency with single-descriptor extraction.
	singleSigs, err := ExtractDescriptorSignaturesFromJAR(path, "android.app.IActivityManager")
	require.NoError(t, err)
	require.NotNil(t, singleSigs)

	for method, params := range singleSigs {
		fullParams, ok := sigs[method]
		assert.True(t, ok, "missing method %s in full extraction", method)
		assert.Equal(t, params, fullParams, "param mismatch for method %s", method)
	}

	for method, params := range sigs {
		singleParams, ok := singleSigs[method]
		assert.True(t, ok, "missing method %s in single extraction", method)
		assert.Equal(t, params, singleParams, "param mismatch for method %s", method)
	}
}

func TestMethodSignatures_NotFound(t *testing.T) {
	const path = "/tmp/framework.jar"

	if _, err := os.Stat(path); err != nil {
		t.Skipf("skipping: %s not available: %v", path, err)
	}

	sigs, err := ExtractDescriptorSignaturesFromJAR(path, "nonexistent.IFakeInterface")
	require.NoError(t, err)
	assert.Nil(t, sigs, "nonexistent descriptor should return nil")
}

func TestMethodSignatures_Bluetooth(t *testing.T) {
	// Try to pull the BT JAR from the emulator.
	btPaths := []string{
		"/tmp/framework-bluetooth.jar",
		"/tmp/service-bluetooth.jar",
	}

	var path string
	for _, p := range btPaths {
		if _, err := os.Stat(p); err == nil {
			path = p
			break
		}
	}

	if path == "" {
		t.Skipf("skipping: no BT JAR available at %v", btPaths)
	}

	result, err := ExtractSignaturesFromJAR(path)
	require.NoError(t, err)

	// If the BT JAR contains $Stub$Proxy classes, verify they
	// have reasonable method signatures.
	for iface, sigs := range result {
		assert.NotEmpty(t, iface, "interface name should not be empty")
		assert.NotEmpty(t, sigs, "method signatures should not be empty for %s", iface)
	}
}

func TestInterfaceToStubProxyDescriptor(t *testing.T) {
	tests := []struct {
		iface string
		want  string
	}{
		{
			iface: "android.app.IActivityManager",
			want:  "Landroid/app/IActivityManager$Stub$Proxy;",
		},
		{
			iface: "android.os.IServiceManager",
			want:  "Landroid/os/IServiceManager$Stub$Proxy;",
		},
	}

	for _, tt := range tests {
		t.Run(tt.iface, func(t *testing.T) {
			got := interfaceToStubProxyDescriptor(tt.iface)
			assert.Equal(t, tt.want, string(got))
		})
	}
}

func TestStubProxyDescriptorBytesToInterface(t *testing.T) {
	tests := []struct {
		desc string
		want string
	}{
		{
			desc: "Landroid/app/IActivityManager$Stub$Proxy;",
			want: "android.app.IActivityManager",
		},
		{
			desc: "Landroid/os/IServiceManager$Stub$Proxy;",
			want: "android.os.IServiceManager",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			got := stubProxyDescriptorBytesToInterface([]byte(tt.desc))
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestStubProxyDescriptorRoundTrip(t *testing.T) {
	interfaces := []string{
		"android.app.IActivityManager",
		"android.os.IServiceManager",
		"com.android.internal.app.IVoiceInteractor",
	}

	for _, iface := range interfaces {
		proxyDesc := string(interfaceToStubProxyDescriptor(iface))
		got := stubProxyDescriptorBytesToInterface([]byte(proxyDesc))
		assert.Equal(t, iface, got, "round-trip failed for %s", iface)
	}
}
