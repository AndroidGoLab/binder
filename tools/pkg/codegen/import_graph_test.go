package codegen

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xaionaro-go/binder/tools/pkg/parser"
	"github.com/xaionaro-go/binder/tools/pkg/resolver"
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
	// In a direct A <-> B cycle, only one direction needs to be broken
	// (the back-edge in DFS order). At least one direction must be
	// cycle-causing to prevent Go import cycles.
	aToB := g.WouldCauseCycle("pkg.a", "pkg.b")
	bToA := g.WouldCauseCycle("pkg.b", "pkg.a")
	assert.True(t, aToB || bToA, "at least one direction must be cycle-causing")
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
	// In a transitive cycle A -> B -> C -> A, only one edge (the
	// back-edge in DFS order) needs to be broken. At least one of
	// the three directions must be cycle-causing.
	aToB := g.WouldCauseCycle("pkg.a", "pkg.b")
	bToC := g.WouldCauseCycle("pkg.b", "pkg.c")
	cToA := g.WouldCauseCycle("pkg.c", "pkg.a")
	assert.True(t, aToB || bToC || cToA, "at least one edge must be cycle-causing")
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
	// Direct cycle between pkg.a and pkg.b: at least one direction
	// must be broken.
	aToB := g.WouldCauseCycle("pkg.a", "pkg.b")
	bToA := g.WouldCauseCycle("pkg.b", "pkg.a")
	assert.True(t, aToB || bToA, "at least one direction must be cycle-causing")
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

	// In a direct A <-> B cycle with parcelable types, BOTH directions
	// should redirect to types sub-packages (not any). Only
	// interface types fall back to any.
	f1 := NewGoFile("a")
	r1 := NewTypeRefResolver(registry, "pkg.a", f1)
	r1.ImportGraph = importGraph
	goTypeFromA := r1.GoTypeRef(&parser.TypeSpecifier{Name: "pkg.b.TypeB"})

	f2 := NewGoFile("b")
	r2 := NewTypeRefResolver(registry, "pkg.b", f2)
	r2.ImportGraph = importGraph
	goTypeFromB := r2.GoTypeRef(&parser.TypeSpecifier{Name: "pkg.a.TypeA"})

	// Neither direction should be broken to any since both
	// types are parcelables that can use types sub-packages.
	assert.NotEqual(t, "any", goTypeFromA, "parcelable type should redirect to types sub-package, not any")
	assert.NotEqual(t, "any", goTypeFromB, "parcelable type should redirect to types sub-package, not any")

	// Same-package resolution should always work.
	f3 := NewGoFile("a")
	r3 := NewTypeRefResolver(registry, "pkg.a", f3)
	r3.ImportGraph = importGraph
	goTypeSame := r3.GoTypeRef(&parser.TypeSpecifier{Name: "pkg.a.TypeA"})
	assert.Equal(t, "TypeA", goTypeSame)
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

	// Determine which direction is the back-edge (broken).
	aToB := importGraph.WouldCauseCycle("pkg.a", "pkg.b")

	// Test from the perspective of the broken direction's source package.
	var srcPkg, targetTypeName string
	switch {
	case aToB:
		srcPkg = "pkg.a"
		targetTypeName = "pkg.b.TypeB"
	default:
		srcPkg = "pkg.b"
		targetTypeName = "pkg.a.TypeA"
	}

	f := NewGoFile(lastPackageSegment(srcPkg))
	typeRef := NewTypeRefResolver(registry, srcPkg, f)
	typeRef.ImportGraph = importGraph

	opts := GenOptions{Registry: registry, ImportGraph: importGraph}

	// marshalForTypeWithCycleCheck should return empty marshal info
	// for cycle-broken types (the back-edge direction).
	ts := &parser.TypeSpecifier{Name: targetTypeName}
	info := marshalForTypeWithCycleCheck(ts, opts, typeRef)
	require.Empty(t, info.WriteExpr, "cycle-broken type should have empty WriteExpr")
	require.Empty(t, info.ReadExpr, "cycle-broken type should have empty ReadExpr")
}
