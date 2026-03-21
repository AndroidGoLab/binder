package versionaware

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xaionaro-go/binder/binder"
)

func TestCacheRoundTrip_Gob(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "test.gob")
	fingerprint := "api=36;jars=test"

	original := VersionTable{
		"android.app.IActivityManager": {
			"isUserAMonkey":   binder.TransactionCode(110),
			"getProcessLimit": binder.TransactionCode(52),
		},
		"android.os.IServiceManager": {
			"checkService": binder.TransactionCode(2),
			"addService":   binder.TransactionCode(3),
		},
	}
	scannedJARs := []string{"framework.jar", "services.jar"}

	saveCachedTable(context.Background(), cachePath, fingerprint, original, scannedJARs)

	// Verify file was created.
	_, err := os.Stat(cachePath)
	require.NoError(t, err, "cache file should exist")

	// Load and verify.
	cached := loadCachedTable(cachePath, fingerprint)
	require.NotNil(t, cached, "cache should load successfully")
	loaded := cached.ResolvedTable
	assert.Equal(t, len(original), len(loaded), "interface count mismatch")

	for desc, methods := range original {
		loadedMethods := loaded[desc]
		require.NotNil(t, loadedMethods, "missing descriptor %s", desc)
		for method, code := range methods {
			assert.Equal(t, code, loadedMethods[method],
				"code mismatch for %s.%s", desc, method)
		}
	}

	// Verify scanned JARs were persisted.
	assert.ElementsMatch(t, scannedJARs, cached.ScannedJARs,
		"scanned JARs should round-trip through cache")
}

func TestCacheRoundTrip_FingerprintMismatch(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "test.gob")

	table := VersionTable{
		"android.app.IActivityManager": {
			"isUserAMonkey": binder.TransactionCode(110),
		},
	}

	saveCachedTable(context.Background(), cachePath, "fingerprint-v1", table, nil)

	// Different fingerprint should return nil.
	cached := loadCachedTable(cachePath, "fingerprint-v2")
	assert.Nil(t, cached, "mismatched fingerprint should return nil")

	// Same fingerprint should work.
	cached = loadCachedTable(cachePath, "fingerprint-v1")
	assert.NotNil(t, cached, "matching fingerprint should load")
}

func TestCacheLoad_MissingFile(t *testing.T) {
	loaded := loadCachedTable("/nonexistent/path.gob", "fp")
	assert.Nil(t, loaded)
}

func TestCacheLoad_CorruptedFile(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "corrupt.gob")
	require.NoError(t, os.WriteFile(cachePath, []byte("not gob data"), 0o644))

	loaded := loadCachedTable(cachePath, "fp")
	assert.Nil(t, loaded, "corrupted file should return nil")
}

func TestResolveCode_LazyExtraction(t *testing.T) {
	// This test verifies that ResolveCode falls back to lazy extraction
	// when a descriptor is not in the pre-loaded table. It requires
	// /system/framework/ to be accessible (skipped on non-Android).
	if _, err := os.ReadDir(frameworkJARDir); err != nil {
		t.Skipf("skipping: %s not accessible: %v", frameworkJARDir, err)
	}

	transport := &Transport{
		apiLevel:    36,
		table:       VersionTable{},
		version:     "36.test",
		ScannedJARs: map[string]bool{},
	}

	// Descriptor not in table — lazy extraction should find it.
	code, err := transport.ResolveCode(context.Background(), "android.app.IActivityManager", "isUserAMonkey")
	require.NoError(t, err, "lazy extraction should find IActivityManager.isUserAMonkey")
	assert.NotZero(t, code, "transaction code should be nonzero")

	// After lazy extraction, the descriptor should be in the table.
	assert.NotNil(t, transport.table["android.app.IActivityManager"],
		"descriptor should be cached in table after lazy extraction")

	// Second call should hit the fast path.
	code2, err := transport.ResolveCode(context.Background(), "android.app.IActivityManager", "isUserAMonkey")
	require.NoError(t, err)
	assert.Equal(t, code, code2, "second call should return same code")
}

func TestResolveCode_LazyExtraction_WithCache(t *testing.T) {
	if _, err := os.ReadDir(frameworkJARDir); err != nil {
		t.Skipf("skipping: %s not accessible: %v", frameworkJARDir, err)
	}

	dir := t.TempDir()
	cachePath := filepath.Join(dir, "lazy.gob")

	transport := &Transport{
		apiLevel:    36,
		table:       VersionTable{},
		version:     "36.test",
		cachePath:   cachePath,
		ScannedJARs: map[string]bool{},
	}

	// Lazy extraction should update the cache file.
	_, err := transport.ResolveCode(context.Background(), "android.app.IActivityManager", "isUserAMonkey")
	require.NoError(t, err)

	// Verify cache file was created.
	_, err = os.Stat(cachePath)
	assert.NoError(t, err, "cache file should be created after lazy extraction")

	// Load cache and verify the descriptor is there.
	fingerprint := resolvedTableFingerprint(36, "")
	cached := loadCachedTable(cachePath, fingerprint)
	require.NotNil(t, cached, "cache should be loadable")
	assert.NotNil(t, cached.ResolvedTable["android.app.IActivityManager"],
		"lazy-extracted descriptor should be in cache")
}

func TestResolveCode_NoLazyForKnownDescriptor(t *testing.T) {
	// If the descriptor exists in the table but the method doesn't,
	// lazy extraction should NOT be attempted (the method genuinely
	// doesn't exist).
	transport := &Transport{
		apiLevel: 36,
		table: VersionTable{
			"android.app.IActivityManager": {
				"isUserAMonkey": binder.TransactionCode(110),
			},
		},
		version: "36.test",
	}

	_, err := transport.ResolveCode(context.Background(), "android.app.IActivityManager", "nonExistentMethod")
	assert.Error(t, err, "unknown method on known descriptor should fail")
}
