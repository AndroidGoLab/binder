package codegen

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/xaionaro-go/binder/tools/pkg/parser"
	"github.com/xaionaro-go/binder/tools/pkg/resolver"
)

// ImportGraph represents the directed dependency graph between AIDL packages.
// An edge from package A to package B means that package A references a type
// defined in package B.
type ImportGraph struct {
	// Edges maps each package to the set of packages it depends on.
	Edges map[string]map[string]bool
	// CyclePkgs is the set of packages that participate in import cycles.
	// Two packages in the same strongly connected component (SCC) of size > 1
	// would create a cycle if they imported each other.
	CyclePkgs map[string]int // package -> SCC index
	// BackEdges records the specific edges within SCCs that are back-edges
	// (their removal makes the subgraph acyclic). Only these edges need
	// to be broken to prevent Go import cycles.
	BackEdges map[string]map[string]bool
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
		Edges:     make(map[string]map[string]bool),
		CyclePkgs: make(map[string]int),
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
			if g.Edges[srcPkg] == nil {
				g.Edges[srcPkg] = make(map[string]bool)
			}
			g.Edges[srcPkg][targetPkg] = true
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
	if g.BackEdges != nil {
		return g.BackEdges[srcPkg] != nil && g.BackEdges[srcPkg][targetPkg]
	}
	// Fallback to SCC membership check when back-edges haven't been computed.
	srcSCC, srcOK := g.CyclePkgs[srcPkg]
	targetSCC, targetOK := g.CyclePkgs[targetPkg]
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
	for pkg, deps := range g.Edges {
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

		// Sort neighbors for deterministic SCC computation.
		sortedEdges := make([]string, 0, len(g.Edges[v]))
		for w := range g.Edges[v] {
			sortedEdges = append(sortedEdges, w)
		}
		sort.Strings(sortedEdges)
		for _, w := range sortedEdges {
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
					g.CyclePkgs[pkg] = sccIndex
				}
				sccIndex++
			}
		}
	}

	sortedNodes := make([]string, 0, len(nodes))
	for node := range nodes {
		sortedNodes = append(sortedNodes, node)
	}
	sort.Strings(sortedNodes)
	for _, node := range sortedNodes {
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
	g.BackEdges = make(map[string]map[string]bool)

	// Group packages by SCC.
	sccMembers := make(map[int][]string)
	for pkg, sccIdx := range g.CyclePkgs {
		sccMembers[sccIdx] = append(sccMembers[sccIdx], pkg)
	}

	for _, members := range sccMembers {
		sort.Strings(members)
		memberSet := make(map[string]bool, len(members))
		for _, m := range members {
			memberSet[m] = true
		}

		color := make(map[string]dfsColor, len(members))

		var dfs func(u string)
		dfs = func(u string) {
			color[u] = gray
			// Sort neighbors for deterministic back-edge selection.
			neighbors := make([]string, 0, len(g.Edges[u]))
			for v := range g.Edges[u] {
				if memberSet[v] {
					neighbors = append(neighbors, v)
				}
			}
			sort.Strings(neighbors)
			for _, v := range neighbors {
				switch color[v] {
				case white:
					dfs(v)
				case gray:
					// Back-edge: u -> v where v is an ancestor.
					if g.BackEdges[u] == nil {
						g.BackEdges[u] = make(map[string]bool)
					}
					g.BackEdges[u][v] = true
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

// StrictForSCC creates a copy of the import graph that treats ALL
// intra-SCC edges as cycle-causing for the SCC containing the given
// package. This is used when generating types sub-packages, where any
// cross-package import within the SCC could create a cycle.
func (g *ImportGraph) StrictForSCC(pkg string) *ImportGraph {
	sccIdx, inSCC := g.CyclePkgs[pkg]
	if !inSCC {
		return g // not in an SCC — no strict mode needed
	}

	// Build the set of packages in this SCC.
	sccPkgs := make(map[string]bool)
	for p, idx := range g.CyclePkgs {
		if idx == sccIdx {
			sccPkgs[p] = true
		}
	}

	// Create a new graph where ALL edges between SCC members are
	// marked as back-edges (cycle-causing).
	strict := &ImportGraph{
		Edges:     g.Edges,
		CyclePkgs: g.CyclePkgs,
		BackEdges: make(map[string]map[string]bool),
	}

	// Copy existing back-edges.
	for src, targets := range g.BackEdges {
		strict.BackEdges[src] = make(map[string]bool)
		for tgt := range targets {
			strict.BackEdges[src][tgt] = true
		}
	}

	// Add ALL intra-SCC edges as back-edges.
	for src := range sccPkgs {
		for tgt := range g.Edges[src] {
			if sccPkgs[tgt] {
				if strict.BackEdges[src] == nil {
					strict.BackEdges[src] = make(map[string]bool)
				}
				strict.BackEdges[src][tgt] = true
			}
		}
	}

	return strict
}

// AugmentFromGoFiles scans Go files in outputDir for import statements
// and adds edges to the import graph. This ensures the graph includes
// dependencies from existing generated code (from previous runs), not
// just spec-defined types.
func (g *ImportGraph) AugmentFromGoFiles(outputDir string) {
	_ = filepath.Walk(outputDir, func(
		path string,
		info os.FileInfo,
		err error,
	) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}

		relPath, err := filepath.Rel(outputDir, path)
		if err != nil {
			return nil
		}

		if !strings.HasPrefix(relPath, "android/") && !strings.HasPrefix(relPath, "com/") {
			return nil
		}

		dir := filepath.Dir(relPath)
		// Skip types sub-packages — they can't create cycles (types-only
		// graph is acyclic) and including them would pollute the SCC
		// computation.
		if strings.HasSuffix(dir, "/types") || strings.Contains(dir, "/types/") {
			return nil
		}
		srcPkg := goPathToAIDLPkg(dir)

		f, fErr := os.Open(path)
		if fErr != nil {
			return nil
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		inImport := false
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())

			if line == "import (" {
				inImport = true
				continue
			}
			if inImport && line == ")" {
				inImport = false
				continue
			}

			if strings.HasPrefix(line, "import ") {
				importPath := extractImportPath(line[7:])
				targetPkg := importPathToAIDLPkg(importPath)
				if targetPkg != "" && targetPkg != srcPkg && !isTypesPkg(targetPkg) {
					if g.Edges[srcPkg] == nil {
						g.Edges[srcPkg] = make(map[string]bool)
					}
					g.Edges[srcPkg][targetPkg] = true
				}
				continue
			}

			if inImport && line != "" {
				importPath := extractImportPath(line)
				targetPkg := importPathToAIDLPkg(importPath)
				if targetPkg != "" && targetPkg != srcPkg && !isTypesPkg(targetPkg) {
					if g.Edges[srcPkg] == nil {
						g.Edges[srcPkg] = make(map[string]bool)
					}
					g.Edges[srcPkg][targetPkg] = true
				}
			}
		}

		return nil
	})
}

// goPathToAIDLPkg converts a Go directory path to an AIDL package name.
func goPathToAIDLPkg(goPath string) string {
	segments := strings.Split(goPath, string(filepath.Separator))
	for i, seg := range segments {
		if seg == "internal_" {
			segments[i] = "internal"
		}
	}
	return strings.Join(segments, ".")
}

// importPathToAIDLPkg extracts an AIDL package name from a Go import path.
func importPathToAIDLPkg(importPath string) string {
	const prefix = goModulePath + "/"
	if !strings.HasPrefix(importPath, prefix) {
		return ""
	}
	goSubPath := importPath[len(prefix):]
	if !strings.HasPrefix(goSubPath, "android/") && !strings.HasPrefix(goSubPath, "com/") {
		return ""
	}
	return goPathToAIDLPkg(goSubPath)
}

// isTypesPkg returns true if the AIDL package name is a "types" sub-package.
func isTypesPkg(aidlPkg string) bool {
	return strings.HasSuffix(aidlPkg, ".types")
}

// extractImportPath extracts the import path from a Go import line.
func extractImportPath(line string) string {
	line = strings.TrimSpace(line)
	start := strings.IndexByte(line, '"')
	if start < 0 {
		return ""
	}
	end := strings.IndexByte(line[start+1:], '"')
	if end < 0 {
		return ""
	}
	return line[start+1 : start+1+end]
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
