package codegen

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGoFile_EmptyFile(t *testing.T) {
	f := NewGoFile("mypackage")
	b, err := f.Bytes()
	require.NoError(t, err)

	src := string(b)
	assert.Contains(t, src, "package mypackage")
	assert.NotContains(t, src, "import")
}

func TestGoFile_WithImports(t *testing.T) {
	f := NewGoFile("mypackage")
	f.AddImport("fmt", "")
	f.AddImport("github.com/xaionaro-go/aidl/binder", "")
	f.P("var _ fmt.Stringer")
	f.P("var _ binder.IBinder")

	b, err := f.Bytes()
	require.NoError(t, err)

	src := string(b)
	assert.Contains(t, src, "package mypackage")
	assert.Contains(t, src, `"fmt"`)
	assert.Contains(t, src, `"github.com/xaionaro-go/aidl/binder"`)
}

func TestGoFile_WithAliasedImport(t *testing.T) {
	f := NewGoFile("mypackage")
	f.AddImport("github.com/xaionaro-go/aidl/binder", "b")
	f.P("var _ b.IBinder")

	b, err := f.Bytes()
	require.NoError(t, err)

	src := string(b)
	assert.Contains(t, src, `b "github.com/xaionaro-go/aidl/binder"`)
}

func TestGoFile_SortedImports(t *testing.T) {
	f := NewGoFile("mypackage")
	f.AddImport("z/package", "")
	f.AddImport("a/package", "")
	f.P("// body")

	b, err := f.Bytes()
	require.NoError(t, err)

	src := string(b)
	aIdx := strings.Index(src, `"a/package"`)
	zIdx := strings.Index(src, `"z/package"`)
	assert.Greater(t, zIdx, aIdx, "imports should be sorted alphabetically")
}

func TestGoFile_FormattedOutput(t *testing.T) {
	f := NewGoFile("mypackage")
	f.P("func Hello() {")
	f.P("return")
	f.P("}")

	b, err := f.Bytes()
	require.NoError(t, err)

	src := string(b)
	// gofmt should have indented the return.
	assert.Contains(t, src, "\treturn")
}

func TestGoFile_InvalidSyntax(t *testing.T) {
	f := NewGoFile("mypackage")
	f.P("func {{{ invalid")

	b, err := f.Bytes()
	// Should return error but still return unformatted source.
	assert.Error(t, err)
	assert.Contains(t, string(b), "func {{{ invalid")
}

func TestGoFile_P_MultipleLines(t *testing.T) {
	f := NewGoFile("mypackage")
	f.P("// line 1")
	f.P("// line 2")

	b, err := f.Bytes()
	require.NoError(t, err)

	src := string(b)
	assert.Contains(t, src, "// line 1")
	assert.Contains(t, src, "// line 2")
}
