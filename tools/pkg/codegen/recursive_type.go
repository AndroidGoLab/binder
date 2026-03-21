package codegen

import (
	"github.com/xaionaro-go/binder/tools/pkg/parser"
	"github.com/xaionaro-go/binder/tools/pkg/resolver"
)

// recursiveFieldSet detects fields that create recursive struct types within
// the same Go package. Go requires at least one pointer to break recursion;
// this function identifies which (type, field) pairs need pointer wrapping.
//
// It builds a directed graph of type references within the same package,
// finds cycles via DFS, and returns a set of edges (type -> referenced type)
// that participate in cycles. The caller uses this to emit `*TypeName`
// instead of `TypeName` for the corresponding struct fields.
type recursiveFieldSet struct {
	// edges maps "GoTypeName" -> set of "GoFieldTypeName" that need pointers.
	edges map[string]map[string]bool
}

// needsPointer returns true if a field of type fieldGoType within the struct
// structGoType must use a pointer to break a recursive type cycle.
func (r *recursiveFieldSet) needsPointer(structGoType, fieldGoType string) bool {
	if r == nil {
		return false
	}
	return r.edges[structGoType][fieldGoType]
}

// detectRecursiveTypes analyzes all type definitions in the given AIDL package
// and returns a recursiveFieldSet indicating which same-package type references
// create recursive struct definitions.
//
// Algorithm: build a directed graph where nodes are Go type names (within the
// same package) and edges represent struct field type references. Then find all
// cycles using Tarjan's SCC algorithm. For each SCC of size > 1, every edge
// within the SCC participates in recursion and needs pointer breaking.
// Self-loops (direct recursion) are also detected.
func detectRecursiveTypes(
	registry *resolver.TypeRegistry,
	currentPkg string,
) *recursiveFieldSet {
	if registry == nil || currentPkg == "" {
		return nil
	}

	allDefs := registry.All()

	// Collect definitions in the current package and build type name -> def map.
	type typeDef struct {
		qualifiedName string
		goName        string
		def           parser.Definition
	}

	pkgTypes := map[string]*typeDef{} // goName -> typeDef
	for qualifiedName, def := range allDefs {
		pkg := packageFromDef(qualifiedName, def.GetName())
		if pkg != currentPkg {
			continue
		}
		goName := AIDLToGoName(def.GetName())
		pkgTypes[goName] = &typeDef{
			qualifiedName: qualifiedName,
			goName:        goName,
			def:           def,
		}
	}

	if len(pkgTypes) == 0 {
		return nil
	}

	// Build the directed graph: edges from a type to the same-package types
	// it references in its fields. Only parcelables and unions have struct
	// fields; enums and interfaces don't create recursive struct issues.
	//
	// graph[A][B] = true means type A has a field of type B (same package).
	graph := map[string]map[string]bool{}

	for goName, td := range pkgTypes {
		var fields []*parser.FieldDecl
		switch d := td.def.(type) {
		case *parser.ParcelableDecl:
			fields = d.Fields
		case *parser.UnionDecl:
			fields = d.Fields
		default:
			continue
		}

		for _, field := range fields {
			fieldTypeNames := collectFieldTypeRefs(field.Type, currentPkg, registry)
			for _, ftName := range fieldTypeNames {
				ftGoName := AIDLToGoName(ftName)
				if _, exists := pkgTypes[ftGoName]; !exists {
					continue
				}
				// Skip references through arrays/slices — Go slices are
				// already pointer-like and don't create infinite-size structs.
				if field.Type.IsArray || field.Type.Name == "List" {
					continue
				}
				if graph[goName] == nil {
					graph[goName] = map[string]bool{}
				}
				graph[goName][ftGoName] = true
			}
		}
	}

	if len(graph) == 0 {
		return nil
	}

	// Find SCCs using Tarjan's algorithm.
	sccEdges := findRecursiveEdges(graph)
	if len(sccEdges) == 0 {
		return nil
	}

	return &recursiveFieldSet{edges: sccEdges}
}

// collectFieldTypeRefs extracts the AIDL definition names referenced by a
// field type, resolving short names to definition names via the registry.
// Only direct (non-array, non-list) references matter for recursion.
func collectFieldTypeRefs(
	ts *parser.TypeSpecifier,
	currentPkg string,
	registry *resolver.TypeRegistry,
) []string {
	if ts == nil {
		return nil
	}

	// Primitives don't reference user types.
	if _, ok := aidlPrimitiveToGo[ts.Name]; ok {
		return nil
	}

	// List/Map type args can be recursive too, but slices/maps in Go are
	// pointer-like and don't cause infinite-size struct issues.
	if ts.Name == "List" || ts.Name == "Map" {
		return nil
	}

	// Try to resolve the type name to a definition name.
	defName := resolveToDefName(ts.Name, currentPkg, registry)
	if defName == "" {
		return nil
	}

	return []string{defName}
}

// resolveToDefName resolves an AIDL type name to its definition's GetName()
// result, which is what AIDLToGoName expects. Returns "" if not found.
func resolveToDefName(
	typeName string,
	currentPkg string,
	registry *resolver.TypeRegistry,
) string {
	// Try fully qualified.
	if def, ok := registry.Lookup(typeName); ok {
		return def.GetName()
	}

	// Try current package + name.
	if currentPkg != "" {
		candidate := currentPkg + "." + typeName
		if def, ok := registry.Lookup(candidate); ok {
			return def.GetName()
		}
	}

	// Try short name lookup.
	if _, def, ok := registry.LookupQualifiedByShortName(typeName); ok {
		return def.GetName()
	}

	return ""
}

// findRecursiveEdges finds all edges within SCCs of size > 1, plus self-loops.
// Returns a map: source -> set of targets, where each edge participates in
// a recursive cycle.
func findRecursiveEdges(
	graph map[string]map[string]bool,
) map[string]map[string]bool {
	// Collect all nodes.
	nodes := map[string]bool{}
	for src, targets := range graph {
		nodes[src] = true
		for tgt := range targets {
			nodes[tgt] = true
		}
	}

	// Tarjan's SCC algorithm.
	index := 0
	nodeIndex := map[string]int{}
	nodeLowlink := map[string]int{}
	onStack := map[string]bool{}
	var stack []string
	var sccs [][]string

	var strongConnect func(v string)
	strongConnect = func(v string) {
		nodeIndex[v] = index
		nodeLowlink[v] = index
		index++
		stack = append(stack, v)
		onStack[v] = true

		for w := range graph[v] {
			_, visited := nodeIndex[w]
			switch {
			case !visited:
				strongConnect(w)
				if nodeLowlink[w] < nodeLowlink[v] {
					nodeLowlink[v] = nodeLowlink[w]
				}
			case onStack[w]:
				if nodeIndex[w] < nodeLowlink[v] {
					nodeLowlink[v] = nodeIndex[w]
				}
			}
		}

		if nodeLowlink[v] == nodeIndex[v] {
			var scc []string
			for {
				w := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				onStack[w] = false
				scc = append(scc, w)
				if w == v {
					break
				}
			}
			sccs = append(sccs, scc)
		}
	}

	for node := range nodes {
		if _, visited := nodeIndex[node]; !visited {
			strongConnect(node)
		}
	}

	// Collect edges within SCCs of size > 1, and self-loops.
	result := map[string]map[string]bool{}

	for _, scc := range sccs {
		if len(scc) < 2 {
			// Check for self-loop.
			if len(scc) == 1 && graph[scc[0]][scc[0]] {
				if result[scc[0]] == nil {
					result[scc[0]] = map[string]bool{}
				}
				result[scc[0]][scc[0]] = true
			}
			continue
		}

		sccSet := map[string]bool{}
		for _, n := range scc {
			sccSet[n] = true
		}

		// For each edge within this SCC, we need to pick which edges
		// to break. The minimum approach: break ALL edges within the SCC,
		// then the generators will use pointers for each. But we only
		// need one edge per cycle. To minimize pointer usage, find which
		// edges to break.
		//
		// Pragmatic approach: within an SCC, every type that has a field
		// referencing another type in the SCC needs at least one pointer.
		// Mark all intra-SCC edges; the generator will make all of them
		// pointers, which is safe (slightly more pointers than strictly
		// necessary, but always correct).
		for src := range sccSet {
			for tgt := range graph[src] {
				if !sccSet[tgt] {
					continue
				}
				if result[src] == nil {
					result[src] = map[string]bool{}
				}
				result[src][tgt] = true
			}
		}
	}

	return result
}
