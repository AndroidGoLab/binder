package dex

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractFromDEX_RealFile(t *testing.T) {
	const path = "/tmp/classes4.dex"

	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("skipping: %s not available: %v", path, err)
	}

	result, err := ExtractFromDEX(data)
	require.NoError(t, err)
	require.NotEmpty(t, result, "expected at least one $Stub class")

	// classes4.dex contains many $Stub classes; verify we found some.
	for iface, codes := range result {
		assert.NotEmpty(t, iface, "interface name should not be empty")
		assert.NotEmpty(t, codes, "transaction codes should not be empty for %s", iface)
	}
}

func TestExtractFromJAR_RealFile(t *testing.T) {
	const path = "/tmp/framework.jar"

	if _, err := os.Stat(path); err != nil {
		t.Skipf("skipping: %s not available: %v", path, err)
	}

	result, err := ExtractFromJAR(path)
	require.NoError(t, err)
	require.NotEmpty(t, result, "expected at least one $Stub class")

	// IActivityManager$Stub should be in classes.dex inside the JAR.
	codes, ok := result["android.app.IActivityManager"]
	require.True(t, ok, "expected android.app.IActivityManager in results")

	monkey, ok := codes["isUserAMonkey"]
	require.True(t, ok, "expected isUserAMonkey transaction code")
	assert.Equal(t, uint32(110), monkey, "isUserAMonkey transaction code")

	// Verify we found a reasonable number of methods.
	assert.Greater(t, len(codes), 100, "IActivityManager should have >100 transaction codes")
}

func TestExtractFromDEX_InvalidData(t *testing.T) {
	_, err := ExtractFromDEX(nil)
	assert.Error(t, err, "nil data should fail")

	_, err = ExtractFromDEX([]byte{})
	assert.Error(t, err, "empty data should fail")

	_, err = ExtractFromDEX([]byte("not a dex file at all, needs to be long enough for header check"))
	assert.Error(t, err, "non-DEX data should fail")
}

func TestExtractFromJAR_InvalidPath(t *testing.T) {
	_, err := ExtractFromJAR("/nonexistent/path.jar")
	assert.Error(t, err, "nonexistent JAR should fail")
}

func TestInterfaceToStubDescriptor(t *testing.T) {
	tests := []struct {
		iface string
		want  string
	}{
		{
			iface: "android.app.IActivityManager",
			want:  "Landroid/app/IActivityManager$Stub;",
		},
		{
			iface: "android.os.IServiceManager",
			want:  "Landroid/os/IServiceManager$Stub;",
		},
		{
			iface: "com.android.internal.app.IVoiceInteractor",
			want:  "Lcom/android/internal/app/IVoiceInteractor$Stub;",
		},
	}

	for _, tt := range tests {
		t.Run(tt.iface, func(t *testing.T) {
			got := interfaceToStubDescriptor(tt.iface)
			assert.Equal(t, tt.want, string(got))
		})
	}
}

func TestInterfaceToStubDescriptor_RoundTrip(t *testing.T) {
	// Verify that interfaceToStubDescriptor and stubDescriptorToInterface
	// are inverses of each other.
	interfaces := []string{
		"android.app.IActivityManager",
		"android.os.IServiceManager",
		"com.android.internal.app.IVoiceInteractor",
	}

	for _, iface := range interfaces {
		stubDesc := string(interfaceToStubDescriptor(iface))
		got := stubDescriptorToInterface(stubDesc)
		assert.Equal(t, iface, got, "round-trip failed for %s", iface)
	}
}

func TestExtractDescriptorFromJAR_RealFile(t *testing.T) {
	const path = "/tmp/framework.jar"

	if _, err := os.Stat(path); err != nil {
		t.Skipf("skipping: %s not available: %v", path, err)
	}

	// Extract only IActivityManager — should match the full extraction.
	codes, err := ExtractDescriptorFromJAR(path, "android.app.IActivityManager")
	require.NoError(t, err)
	require.NotNil(t, codes, "expected IActivityManager in framework.jar")

	monkey, ok := codes["isUserAMonkey"]
	require.True(t, ok, "expected isUserAMonkey transaction code")
	assert.Equal(t, uint32(110), monkey, "isUserAMonkey transaction code")
	assert.Greater(t, len(codes), 100, "IActivityManager should have >100 transaction codes")

	// Verify consistency with full extraction.
	fullResult, err := ExtractFromJAR(path)
	require.NoError(t, err)
	fullCodes := fullResult["android.app.IActivityManager"]
	require.NotNil(t, fullCodes)

	for method, code := range codes {
		assert.Equal(t, fullCodes[method], code, "mismatch for method %s", method)
	}
	for method, code := range fullCodes {
		assert.Equal(t, codes[method], code, "missing method %s in descriptor extraction", method)
	}
}

func TestExtractDescriptorFromJAR_NotFound(t *testing.T) {
	const path = "/tmp/framework.jar"

	if _, err := os.Stat(path); err != nil {
		t.Skipf("skipping: %s not available: %v", path, err)
	}

	codes, err := ExtractDescriptorFromJAR(path, "nonexistent.IFakeInterface")
	require.NoError(t, err)
	assert.Nil(t, codes, "nonexistent descriptor should return nil")
}

func TestStubDescriptorToInterface(t *testing.T) {
	tests := []struct {
		desc string
		want string
	}{
		{
			desc: "Landroid/app/IActivityManager$Stub;",
			want: "android.app.IActivityManager",
		},
		{
			desc: "Landroid/os/IServiceManager$Stub;",
			want: "android.os.IServiceManager",
		},
		{
			desc: "Lcom/android/internal/app/IVoiceInteractor$Stub;",
			want: "com.android.internal.app.IVoiceInteractor",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			got := stubDescriptorToInterface(tt.desc)
			assert.Equal(t, tt.want, got)
		})
	}
}
