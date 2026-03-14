package resolver

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testdataDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "testdata")
}

func TestResolver_ResolveFile_Standalone(t *testing.T) {
	r := New([]string{testdataDir()})

	err := r.ResolveFile(filepath.Join(testdataDir(), "com/example/IStandalone.aidl"))
	require.NoError(t, err)

	_, ok := r.Registry().Lookup("com.example.IStandalone")
	assert.True(t, ok, "IStandalone should be registered")
}

func TestResolver_ResolveFile_TransitiveImports(t *testing.T) {
	r := New([]string{testdataDir()})

	err := r.ResolveFile(filepath.Join(testdataDir(), "com/example/IFoo.aidl"))
	require.NoError(t, err)

	// IFoo itself.
	_, ok := r.Registry().Lookup("com.example.IFoo")
	assert.True(t, ok, "IFoo should be registered")

	// Direct import: MyData.
	_, ok = r.Registry().Lookup("com.other.MyData")
	assert.True(t, ok, "MyData should be registered (direct import)")

	// Transitive import: Status (imported by MyData).
	_, ok = r.Registry().Lookup("com.other.Status")
	assert.True(t, ok, "Status should be registered (transitive import)")
}

func TestResolver_ResolveFile_SkipAlreadyResolved(t *testing.T) {
	r := New([]string{testdataDir()})

	err := r.ResolveFile(filepath.Join(testdataDir(), "com/example/IFoo.aidl"))
	require.NoError(t, err)

	// Resolving the same file again should be a no-op.
	err = r.ResolveFile(filepath.Join(testdataDir(), "com/example/IFoo.aidl"))
	require.NoError(t, err)

	all := r.Registry().All()
	assert.Len(t, all, 3, "should have exactly 3 definitions: IFoo, MyData, Status")
}

func TestResolver_ResolveFile_NotFound(t *testing.T) {
	r := New([]string{testdataDir()})

	err := r.ResolveFile(filepath.Join(testdataDir(), "nonexistent.aidl"))
	assert.Error(t, err)
}

func TestResolver_ResolveFile_ImportNotFound(t *testing.T) {
	r := New([]string{"/nonexistent/path"})

	// Create a document that imports something unfindable.
	err := r.ResolveFile(filepath.Join(testdataDir(), "com/example/IFoo.aidl"))
	// IFoo imports com.other.MyData, which should not be found in /nonexistent/path.
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot find AIDL file")
}

func TestResolver_ResolveDocument(t *testing.T) {
	r := New([]string{testdataDir()})

	// Parse manually, then resolve the document.
	doc, err := parseTestFile(filepath.Join(testdataDir(), "com/example/IStandalone.aidl"))
	require.NoError(t, err)

	err = r.ResolveDocument(doc, filepath.Join(testdataDir(), "com/example/IStandalone.aidl"))
	require.NoError(t, err)

	_, ok := r.Registry().Lookup("com.example.IStandalone")
	assert.True(t, ok)
}

func TestResolver_Registry(t *testing.T) {
	r := New(nil)
	assert.NotNil(t, r.Registry())
}

func TestResolver_NestedInterface(t *testing.T) {
	r := New([]string{testdataDir()})

	err := r.ResolveFile(filepath.Join(testdataDir(), "com/example/ParcelWithNestedInterface.aidl"))
	require.NoError(t, err)

	_, ok := r.Registry().Lookup("com.example.ParcelWithNestedInterface")
	assert.True(t, ok, "outer parcelable should be registered")

	_, ok = r.Registry().Lookup("com.example.ParcelWithNestedInterface.IInner")
	assert.True(t, ok, "nested interface should be registered")
}

func TestResolver_NestedParcelable(t *testing.T) {
	r := New([]string{testdataDir()})

	err := r.ResolveFile(filepath.Join(testdataDir(), "com/example/ParcelWithNestedParcelable.aidl"))
	require.NoError(t, err)

	_, ok := r.Registry().Lookup("com.example.ParcelWithNestedParcelable")
	assert.True(t, ok, "outer parcelable should be registered")

	_, ok = r.Registry().Lookup("com.example.ParcelWithNestedParcelable.InnerData")
	assert.True(t, ok, "nested parcelable should be registered")
}

func TestResolver_DeeplyNested(t *testing.T) {
	r := New([]string{testdataDir()})

	err := r.ResolveFile(filepath.Join(testdataDir(), "com/example/DeeplyNested.aidl"))
	require.NoError(t, err)

	_, ok := r.Registry().Lookup("com.example.DeeplyNested")
	assert.True(t, ok, "level 1 should be registered")

	_, ok = r.Registry().Lookup("com.example.DeeplyNested.Level2")
	assert.True(t, ok, "level 2 should be registered")

	_, ok = r.Registry().Lookup("com.example.DeeplyNested.Level2.Level3")
	assert.True(t, ok, "level 3 should be registered")
}
