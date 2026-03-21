package codegen

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xaionaro-go/binder/tools/pkg/parser"
	"github.com/xaionaro-go/binder/tools/pkg/resolver"
)

func TestDetectRecursiveTypes_MutualRecursion(t *testing.T) {
	// Simulate BusConfigInterfaceId <-> NativeInterface mutual recursion.
	registry := resolver.NewTypeRegistry()
	registry.Register("test.pkg.InterfaceId", &parser.UnionDecl{
		UnionName: "InterfaceId",
		Fields: []*parser.FieldDecl{
			{FieldName: "nativeif", Type: &parser.TypeSpecifier{Name: "NativeIface"}},
			{FieldName: "value", Type: &parser.TypeSpecifier{Name: "int"}},
		},
	})
	registry.Register("test.pkg.NativeIface", &parser.ParcelableDecl{
		ParcName: "NativeIface",
		Fields: []*parser.FieldDecl{
			{FieldName: "interfaceId", Type: &parser.TypeSpecifier{Name: "InterfaceId"}},
		},
	})

	recFields := detectRecursiveTypes(registry, "test.pkg")
	require.NotNil(t, recFields)

	// Both directions of the cycle should be detected.
	assert.True(t, recFields.needsPointer("InterfaceId", "NativeIface"),
		"InterfaceId.NativeIface should need pointer")
	assert.True(t, recFields.needsPointer("NativeIface", "InterfaceId"),
		"NativeIface.InterfaceId should need pointer")

	// Non-recursive fields should not need pointers.
	assert.False(t, recFields.needsPointer("InterfaceId", "int32"),
		"InterfaceId.int32 should not need pointer")
}

func TestDetectRecursiveTypes_DirectSelfRecursion(t *testing.T) {
	registry := resolver.NewTypeRegistry()
	registry.Register("test.pkg.TreeNode", &parser.ParcelableDecl{
		ParcName: "TreeNode",
		Fields: []*parser.FieldDecl{
			{FieldName: "value", Type: &parser.TypeSpecifier{Name: "int"}},
			{FieldName: "child", Type: &parser.TypeSpecifier{Name: "TreeNode"}},
		},
	})

	recFields := detectRecursiveTypes(registry, "test.pkg")
	require.NotNil(t, recFields)

	assert.True(t, recFields.needsPointer("TreeNode", "TreeNode"),
		"TreeNode.child should need pointer (self-referencing)")
}

func TestDetectRecursiveTypes_NoRecursion(t *testing.T) {
	registry := resolver.NewTypeRegistry()
	registry.Register("test.pkg.Foo", &parser.ParcelableDecl{
		ParcName: "Foo",
		Fields: []*parser.FieldDecl{
			{FieldName: "bar", Type: &parser.TypeSpecifier{Name: "Bar"}},
		},
	})
	registry.Register("test.pkg.Bar", &parser.ParcelableDecl{
		ParcName: "Bar",
		Fields: []*parser.FieldDecl{
			{FieldName: "value", Type: &parser.TypeSpecifier{Name: "int"}},
		},
	})

	recFields := detectRecursiveTypes(registry, "test.pkg")

	// No cycle, so either nil or empty.
	if recFields != nil {
		assert.False(t, recFields.needsPointer("Foo", "Bar"),
			"Foo.Bar should not need pointer (no cycle)")
	}
}

func TestDetectRecursiveTypes_ArrayFieldBreaksRecursion(t *testing.T) {
	// Array fields don't create infinite-size structs because Go slices
	// are pointer-like. Verify they're excluded from cycle detection.
	registry := resolver.NewTypeRegistry()
	registry.Register("test.pkg.Parent", &parser.ParcelableDecl{
		ParcName: "Parent",
		Fields: []*parser.FieldDecl{
			{FieldName: "children", Type: &parser.TypeSpecifier{
				Name: "Child", IsArray: true,
			}},
		},
	})
	registry.Register("test.pkg.Child", &parser.ParcelableDecl{
		ParcName: "Child",
		Fields: []*parser.FieldDecl{
			{FieldName: "parent", Type: &parser.TypeSpecifier{Name: "Parent"}},
		},
	})

	recFields := detectRecursiveTypes(registry, "test.pkg")

	// The array breaks the cycle from Parent -> Child, but Child -> Parent
	// is a direct (non-array) reference. Since Parent -> Child is via array,
	// there is no actual SCC: Parent doesn't embed Child directly.
	if recFields != nil {
		assert.False(t, recFields.needsPointer("Parent", "Child"),
			"Parent.children is an array, should not need pointer")
	}
}

func TestDetectRecursiveTypes_NilRegistry(t *testing.T) {
	recFields := detectRecursiveTypes(nil, "test.pkg")
	assert.Nil(t, recFields)
}

func TestDetectRecursiveTypes_EmptyPkg(t *testing.T) {
	registry := resolver.NewTypeRegistry()
	recFields := detectRecursiveTypes(registry, "")
	assert.Nil(t, recFields)
}

func TestDetectRecursiveTypes_CrossPackageNotDetected(t *testing.T) {
	// Types in different packages can't form recursive structs
	// (they use import cycles, handled separately).
	registry := resolver.NewTypeRegistry()
	registry.Register("pkg1.Foo", &parser.ParcelableDecl{
		ParcName: "Foo",
		Fields: []*parser.FieldDecl{
			{FieldName: "bar", Type: &parser.TypeSpecifier{Name: "Bar"}},
		},
	})
	registry.Register("pkg2.Bar", &parser.ParcelableDecl{
		ParcName: "Bar",
		Fields: []*parser.FieldDecl{
			{FieldName: "foo", Type: &parser.TypeSpecifier{Name: "Foo"}},
		},
	})

	// Analyzing from pkg1's perspective: Bar is in pkg2, not same package.
	recFields := detectRecursiveTypes(registry, "pkg1")
	if recFields != nil {
		assert.False(t, recFields.needsPointer("Foo", "Bar"),
			"cross-package references should not be detected as recursive")
	}
}

func TestGenerateParcelable_RecursiveType(t *testing.T) {
	// Set up a registry with mutually recursive types and generate
	// the parcelable. The struct field should use a pointer.
	registry := resolver.NewTypeRegistry()
	registry.Register("test.pkg.Alpha", &parser.ParcelableDecl{
		ParcName: "Alpha",
		Fields: []*parser.FieldDecl{
			{FieldName: "beta", Type: &parser.TypeSpecifier{Name: "Beta"}},
			{FieldName: "value", Type: &parser.TypeSpecifier{Name: "int"}},
		},
	})
	registry.Register("test.pkg.Beta", &parser.ParcelableDecl{
		ParcName: "Beta",
		Fields: []*parser.FieldDecl{
			{FieldName: "alpha", Type: &parser.TypeSpecifier{Name: "Alpha"}},
		},
	})

	src, err := GenerateParcelable(
		registry.All()["test.pkg.Alpha"].(*parser.ParcelableDecl),
		"pkg", "test.pkg.Alpha",
		WithRegistry(registry),
		WithCurrentPkg("test.pkg"),
	)
	require.NoError(t, err)
	require.NotNil(t, src)

	srcStr := string(src)

	// The Beta field should be a pointer to break the recursive cycle.
	assert.Contains(t, srcStr, "*Beta")
	// The int field should not be a pointer.
	assert.Contains(t, srcStr, "int32")
	assert.NotContains(t, srcStr, "*int32")

	assertValidGo(t, src)
}

func TestGenerateUnion_RecursiveType(t *testing.T) {
	// Set up a registry with a union that references a parcelable
	// which references the union back.
	registry := resolver.NewTypeRegistry()
	registry.Register("test.pkg.MyUnion", &parser.UnionDecl{
		UnionName: "MyUnion",
		Fields: []*parser.FieldDecl{
			{FieldName: "nested", Type: &parser.TypeSpecifier{Name: "MyParc"}},
			{FieldName: "value", Type: &parser.TypeSpecifier{Name: "int"}},
		},
	})
	registry.Register("test.pkg.MyParc", &parser.ParcelableDecl{
		ParcName: "MyParc",
		Fields: []*parser.FieldDecl{
			{FieldName: "union", Type: &parser.TypeSpecifier{Name: "MyUnion"}},
		},
	})

	src, err := GenerateUnion(
		registry.All()["test.pkg.MyUnion"].(*parser.UnionDecl),
		"pkg", "test.pkg.MyUnion",
		WithRegistry(registry),
		WithCurrentPkg("test.pkg"),
	)
	require.NoError(t, err)
	require.NotNil(t, src)

	srcStr := string(src)

	// The MyParc field should be a pointer to break the recursive cycle.
	assert.Contains(t, srcStr, "*MyParc")

	assertValidGo(t, src)
}
