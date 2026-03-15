package codegen

import (
	"strings"

	"github.com/xaionaro-go/binder/tools/pkg/parser"
	"github.com/xaionaro-go/binder/tools/pkg/resolver"
)

// ImportGraph represents the directed dependency graph between AIDL packages.
// An edge from package A to package B means that package A references a type
// defined in package B.
type ImportGraph struct {
	// edges maps each package to the set of packages it depends on.
	edges map[string]map[string]bool
	// cyclePkgs is the set of packages that participate in import cycles.
	// Two packages in the same strongly connected component (SCC) of size > 1
	// would create a cycle if they imported each other.
	cyclePkgs map[string]int // package -> SCC index
	// backEdges records the specific edges within SCCs that are back-edges
	// (their removal makes the subgraph acyclic). Only these edges need
	// to be broken to prevent Go import cycles.
	backEdges map[string]map[string]bool
}

// BuildImportGraph scans all definitions in the registry and builds a
// package dependency graph. It computes strongly connected components
// (SCCs) to find cycles, then identifies which specific edges within
// each SCC are back-edges that must be broken.
//
// A single SCC pass that marks ALL intra-SCC edges as cycle-causing is
// too conservative: it prevents imports between packages that are only
// indirectly part of a cycle (via long chains through unrelated packages).
// Instead, after finding SCCs, a DFS within each SCC identifies back-edges
// (the minimum set of edges whose removal makes the subgraph acyclic).
// Only those back-edges are marked as cycle-causing.
func BuildImportGraph(registry *resolver.TypeRegistry) *ImportGraph {
	g := &ImportGraph{
		edges:     make(map[string]map[string]bool),
		cyclePkgs: make(map[string]int),
	}

	allDefs := registry.All()

	// Build edges: for each definition, find what packages its types reference.
	for qualifiedName, def := range allDefs {
		defName := def.GetName()
		srcPkg := packageFromDef(qualifiedName, defName)
		if srcPkg == "" {
			continue
		}

		typeNames := collectTypeNames(def)
		for _, typeName := range typeNames {
			targetPkg := g.resolveTypePkg(typeName, srcPkg, registry)
			if targetPkg == "" || targetPkg == srcPkg {
				continue
			}
			if g.edges[srcPkg] == nil {
				g.edges[srcPkg] = make(map[string]bool)
			}
			g.edges[srcPkg][targetPkg] = true
		}
	}

	// Compute SCCs to find cycles.
	g.computeSCCs()

	// Identify back-edges within each SCC using DFS. Only back-edges
	// are cycle-causing; forward and cross edges are safe to keep.
	g.computeBackEdges()

	return g
}

// WouldCauseCycle returns true if adding an import from srcPkg to targetPkg
// would create an import cycle. Only back-edges (identified by DFS within
// each SCC) are considered cycle-causing; forward and cross edges within
// the same SCC are safe.
func (g *ImportGraph) WouldCauseCycle(srcPkg, targetPkg string) bool {
	if g.backEdges != nil {
		return g.backEdges[srcPkg] != nil && g.backEdges[srcPkg][targetPkg]
	}
	// Fallback to SCC membership check when back-edges haven't been computed.
	srcSCC, srcOK := g.cyclePkgs[srcPkg]
	targetSCC, targetOK := g.cyclePkgs[targetPkg]
	if !srcOK || !targetOK {
		return false
	}
	return srcSCC == targetSCC
}

// computeSCCs finds strongly connected components using Tarjan's algorithm
// and records which packages belong to SCCs of size > 1 (i.e., cycles).
func (g *ImportGraph) computeSCCs() {
	// Collect all nodes.
	nodes := make(map[string]bool)
	for pkg, deps := range g.edges {
		nodes[pkg] = true
		for dep := range deps {
			nodes[dep] = true
		}
	}

	// Tarjan's algorithm state.
	index := 0
	nodeIndex := make(map[string]int)
	nodeLowlink := make(map[string]int)
	onStack := make(map[string]bool)
	var stack []string
	sccIndex := 0

	var strongConnect func(v string)
	strongConnect = func(v string) {
		nodeIndex[v] = index
		nodeLowlink[v] = index
		index++
		stack = append(stack, v)
		onStack[v] = true

		for w := range g.edges[v] {
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

		// If v is a root node, pop the SCC.
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

			// Only record SCCs with more than one node (actual cycles).
			if len(scc) > 1 {
				for _, pkg := range scc {
					g.cyclePkgs[pkg] = sccIndex
				}
				sccIndex++
			}
		}
	}

	for node := range nodes {
		if _, visited := nodeIndex[node]; !visited {
			strongConnect(node)
		}
	}
}

// computeBackEdges identifies back-edges within each SCC using DFS.
// A back-edge is an edge from a node to one of its ancestors in the DFS
// tree. Removing all back-edges from a directed graph makes it acyclic.
// Only intra-SCC edges are considered; cross-SCC edges cannot form cycles.
func (g *ImportGraph) computeBackEdges() {
	g.backEdges = make(map[string]map[string]bool)

	// Group packages by SCC.
	sccMembers := make(map[int][]string)
	for pkg, sccIdx := range g.cyclePkgs {
		sccMembers[sccIdx] = append(sccMembers[sccIdx], pkg)
	}

	// DFS states.
	const (
		white = 0 // not visited
		gray  = 1 // in progress (on the DFS stack)
		black = 2 // finished
	)

	for _, members := range sccMembers {
		memberSet := make(map[string]bool, len(members))
		for _, m := range members {
			memberSet[m] = true
		}

		color := make(map[string]int, len(members))

		var dfs func(u string)
		dfs = func(u string) {
			color[u] = gray
			for v := range g.edges[u] {
				if !memberSet[v] {
					continue
				}
				switch color[v] {
				case white:
					dfs(v)
				case gray:
					// Back-edge: u -> v where v is an ancestor.
					if g.backEdges[u] == nil {
						g.backEdges[u] = make(map[string]bool)
					}
					g.backEdges[u][v] = true
				}
			}
			color[u] = black
		}

		for _, m := range members {
			if color[m] == white {
				dfs(m)
			}
		}
	}
}

// collectTypeNames extracts all type names referenced by a definition.
// These are the raw AIDL type names (possibly qualified) from fields,
// method parameters, return types, etc.
func collectTypeNames(def parser.Definition) []string {
	var names []string

	switch d := def.(type) {
	case *parser.InterfaceDecl:
		for _, m := range d.Methods {
			names = append(names, collectTypeSpecNames(m.ReturnType)...)
			for _, p := range m.Params {
				names = append(names, collectTypeSpecNames(p.Type)...)
			}
		}
	case *parser.ParcelableDecl:
		for _, f := range d.Fields {
			names = append(names, collectTypeSpecNames(f.Type)...)
		}
	case *parser.UnionDecl:
		for _, f := range d.Fields {
			names = append(names, collectTypeSpecNames(f.Type)...)
		}
	}

	return names
}

// collectTypeSpecNames extracts type names from a TypeSpecifier, including
// type arguments (e.g., List<Foo> yields both "List" and "Foo").
func collectTypeSpecNames(ts *parser.TypeSpecifier) []string {
	if ts == nil {
		return nil
	}

	var names []string

	// Skip primitives and built-in types.
	if _, ok := aidlPrimitiveToGo[ts.Name]; !ok {
		if ts.Name != "List" && ts.Name != "Map" {
			names = append(names, ts.Name)
		}
	}

	for _, arg := range ts.TypeArgs {
		names = append(names, collectTypeSpecNames(arg)...)
	}

	return names
}

// resolveTypePkg resolves a type name to its AIDL package using the registry.
func (g *ImportGraph) resolveTypePkg(
	typeName string,
	currentPkg string,
	registry *resolver.TypeRegistry,
) string {
	// Try fully qualified lookup.
	if def, ok := registry.Lookup(typeName); ok {
		return packageFromDef(typeName, def.GetName())
	}

	// Try current package + name.
	candidate := currentPkg + "." + typeName
	if def, ok := registry.Lookup(candidate); ok {
		return packageFromDef(candidate, def.GetName())
	}

	// Try short name lookup.
	if qualifiedName, _, ok := registry.LookupQualifiedByShortName(typeName); ok {
		if def, ok := registry.Lookup(qualifiedName); ok {
			return packageFromDef(qualifiedName, def.GetName())
		}
	}

	// For dotted names, try resolving the first segment as a type.
	if strings.Contains(typeName, ".") {
		dotIdx := strings.IndexByte(typeName, '.')
		firstPart := typeName[:dotIdx]
		rest := typeName[dotIdx+1:]

		if parentQualified, _, ok := registry.LookupQualifiedByShortName(firstPart); ok {
			candidate := parentQualified + "." + rest
			if def, ok := registry.Lookup(candidate); ok {
				return packageFromDef(candidate, def.GetName())
			}
		}
	}

	return ""
}
