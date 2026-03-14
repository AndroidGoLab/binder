package codegen

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xaionaro-go/aidl/tools/pkg/parser"
	"github.com/xaionaro-go/aidl/tools/pkg/resolver"
)

func TestImportGraph_NoCycle(t *testing.T) {
	registry := resolver.NewTypeRegistry()
	registry.Register("pkg.a.TypeA", &parser.ParcelableDecl{
		ParcName: "TypeA",
		Fields: []*parser.FieldDecl{
			{FieldName: "val", Type: &parser.TypeSpecifier{Name: "int"}},
		},
	})
	registry.Register("pkg.b.TypeB", &parser.ParcelableDecl{
		ParcName: "TypeB",
		Fields: []*parser.FieldDecl{
			{FieldName: "a", Type: &parser.TypeSpecifier{Name: "pkg.a.TypeA"}},
		},
	})

	g := BuildImportGraph(registry)
	assert.False(t, g.WouldCauseCycle("pkg.b", "pkg.a"))
	assert.False(t, g.WouldCauseCycle("pkg.a", "pkg.b"))
}

func TestImportGraph_DirectCycle(t *testing.T) {
	registry := resolver.NewTypeRegistry()
	registry.Register("pkg.a.TypeA", &parser.ParcelableDecl{
		ParcName: "TypeA",
		Fields: []*parser.FieldDecl{
			{FieldName: "b", Type: &parser.TypeSpecifier{Name: "pkg.b.TypeB"}},
		},
	})
	registry.Register("pkg.b.TypeB", &parser.ParcelableDecl{
		ParcName: "TypeB",
		Fields: []*parser.FieldDecl{
			{FieldName: "a", Type: &parser.TypeSpecifier{Name: "pkg.a.TypeA"}},
		},
	})

	g := BuildImportGraph(registry)
	assert.True(t, g.WouldCauseCycle("pkg.a", "pkg.b"))
	assert.True(t, g.WouldCauseCycle("pkg.b", "pkg.a"))
}

func TestImportGraph_TransitiveCycle(t *testing.T) {
	registry := resolver.NewTypeRegistry()
	registry.Register("pkg.a.TypeA", &parser.ParcelableDecl{
		ParcName: "TypeA",
		Fields: []*parser.FieldDecl{
			{FieldName: "b", Type: &parser.TypeSpecifier{Name: "pkg.b.TypeB"}},
		},
	})
	registry.Register("pkg.b.TypeB", &parser.ParcelableDecl{
		ParcName: "TypeB",
		Fields: []*parser.FieldDecl{
			{FieldName: "c", Type: &parser.TypeSpecifier{Name: "pkg.c.TypeC"}},
		},
	})
	registry.Register("pkg.c.TypeC", &parser.ParcelableDecl{
		ParcName: "TypeC",
		Fields: []*parser.FieldDecl{
			{FieldName: "a", Type: &parser.TypeSpecifier{Name: "pkg.a.TypeA"}},
		},
	})

	g := BuildImportGraph(registry)
	assert.True(t, g.WouldCauseCycle("pkg.a", "pkg.b"))
	assert.True(t, g.WouldCauseCycle("pkg.b", "pkg.c"))
	assert.True(t, g.WouldCauseCycle("pkg.c", "pkg.a"))
}

func TestImportGraph_InterfaceMethods(t *testing.T) {
	registry := resolver.NewTypeRegistry()
	registry.Register("pkg.a.IServiceA", &parser.InterfaceDecl{
		IntfName: "IServiceA",
		Methods: []*parser.MethodDecl{
			{
				MethodName: "getB",
				ReturnType: &parser.TypeSpecifier{Name: "pkg.b.TypeB"},
			},
		},
	})
	registry.Register("pkg.b.TypeB", &parser.ParcelableDecl{
		ParcName: "TypeB",
		Fields: []*parser.FieldDecl{
			{FieldName: "service", Type: &parser.TypeSpecifier{Name: "pkg.a.IServiceA"}},
		},
	})

	g := BuildImportGraph(registry)
	assert.True(t, g.WouldCauseCycle("pkg.a", "pkg.b"))
	assert.True(t, g.WouldCauseCycle("pkg.b", "pkg.a"))
}

func TestImportGraph_UnrelatedPkgsNotCycled(t *testing.T) {
	registry := resolver.NewTypeRegistry()
	registry.Register("pkg.a.TypeA", &parser.ParcelableDecl{
		ParcName: "TypeA",
		Fields: []*parser.FieldDecl{
			{FieldName: "b", Type: &parser.TypeSpecifier{Name: "pkg.b.TypeB"}},
		},
	})
	registry.Register("pkg.b.TypeB", &parser.ParcelableDecl{
		ParcName: "TypeB",
		Fields: []*parser.FieldDecl{
			{FieldName: "a", Type: &parser.TypeSpecifier{Name: "pkg.a.TypeA"}},
		},
	})
	registry.Register("pkg.c.TypeC", &parser.ParcelableDecl{
		ParcName: "TypeC",
		Fields: []*parser.FieldDecl{
			{FieldName: "val", Type: &parser.TypeSpecifier{Name: "int"}},
		},
	})

	g := BuildImportGraph(registry)
	// pkg.c is not part of the cycle.
	assert.False(t, g.WouldCauseCycle("pkg.a", "pkg.c"))
	assert.False(t, g.WouldCauseCycle("pkg.c", "pkg.a"))
}

func TestImportGraph_CycleBreakInTypeResolver(t *testing.T) {
	registry := resolver.NewTypeRegistry()
	registry.Register("pkg.a.TypeA", &parser.ParcelableDecl{
		ParcName: "TypeA",
		Fields: []*parser.FieldDecl{
			{FieldName: "b", Type: &parser.TypeSpecifier{Name: "pkg.b.TypeB"}},
		},
	})
	registry.Register("pkg.b.TypeB", &parser.ParcelableDecl{
		ParcName: "TypeB",
		Fields: []*parser.FieldDecl{
			{FieldName: "a", Type: &parser.TypeSpecifier{Name: "pkg.a.TypeA"}},
		},
	})

	importGraph := BuildImportGraph(registry)

	f := NewGoFile("a")
	resolver := NewTypeRefResolver(registry, "pkg.a", f)
	resolver.importGraph = importGraph

	// Resolving TypeB from pkg.a should produce interface{} due to cycle.
	ts := &parser.TypeSpecifier{Name: "pkg.b.TypeB"}
	goType := resolver.GoTypeRef(ts)
	assert.Equal(t, "interface{}", goType)

	// Resolving TypeA from pkg.a should work (same package).
	ts2 := &parser.TypeSpecifier{Name: "pkg.a.TypeA"}
	goType2 := resolver.GoTypeRef(ts2)
	assert.Equal(t, "TypeA", goType2)
}

func TestImportGraph_MarshalInfoForCycleBrokenType(t *testing.T) {
	registry := resolver.NewTypeRegistry()
	registry.Register("pkg.a.TypeA", &parser.ParcelableDecl{
		ParcName: "TypeA",
		Fields: []*parser.FieldDecl{
			{FieldName: "b", Type: &parser.TypeSpecifier{Name: "pkg.b.TypeB"}},
		},
	})
	registry.Register("pkg.b.TypeB", &parser.ParcelableDecl{
		ParcName: "TypeB",
		Fields: []*parser.FieldDecl{
			{FieldName: "a", Type: &parser.TypeSpecifier{Name: "pkg.a.TypeA"}},
		},
	})

	importGraph := BuildImportGraph(registry)

	f := NewGoFile("a")
	typeRef := NewTypeRefResolver(registry, "pkg.a", f)
	typeRef.importGraph = importGraph

	opts := GenOptions{Registry: registry, ImportGraph: importGraph}

	// marshalForTypeWithCycleCheck should return empty marshal info
	// for cycle-broken types.
	ts := &parser.TypeSpecifier{Name: "pkg.b.TypeB"}
	info := marshalForTypeWithCycleCheck(ts, opts, typeRef)
	require.Empty(t, info.WriteExpr, "cycle-broken type should have empty WriteExpr")
	require.Empty(t, info.ReadExpr, "cycle-broken type should have empty ReadExpr")
}
