package codegen

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/xaionaro-go/binder/tools/pkg/parser"
	"github.com/xaionaro-go/binder/tools/pkg/resolver"
	"github.com/xaionaro-go/binder/tools/pkg/validate"
)

// Generator generates Go code from AIDL files.
type Generator struct {
	Resolver   *resolver.Resolver
	OutputDir  string
	SkipErrors bool
	// CycleTypes collects empty parcelables that were redirected to "types"
	// sub-packages during codegen to break import cycles. Populated during
	// GenerateAll and used to generate the sub-package files afterward.
	// Maps qualified AIDL name → types sub-package AIDL name.
	CycleTypes map[string]string
}

// NewGenerator creates a Generator that writes output to outputDir.
func NewGenerator(
	r *resolver.Resolver,
	outputDir string,
) *Generator {
	return &Generator{
		Resolver:  r,
		OutputDir: outputDir,
	}
}

// SetSkipErrors configures whether the generator skips definitions
// that fail codegen and continues with the rest.
func (g *Generator) SetSkipErrors(
	skip bool,
) {
	g.SkipErrors = skip
}

// GenerateFile generates Go code for all definitions in a single AIDL document.
// It runs semantic validation before code generation.
func (g *Generator) GenerateFile(
	doc *parser.Document,
) (_err error) {
	if errs := g.validateDocument(doc); len(errs) > 0 {
		if !g.SkipErrors {
			return errors.Join(errs...)
		}
	}

	pkg := ""
	if doc.Package != nil {
		pkg = doc.Package.Name
	}

	goPackage := lastPackageSegment(pkg)
	if goPackage == "" {
		goPackage = "aidl"
	}

	outDir := filepath.Join(g.OutputDir, AIDLToGoPackage(pkg))

	for _, def := range doc.Definitions {
		qualifiedName := def.GetName()
		if pkg != "" {
			qualifiedName = pkg + "." + def.GetName()
		}

		src, err := g.generateDefinition(def, goPackage, qualifiedName)
		if err != nil {
			return fmt.Errorf("generating %s: %w", qualifiedName, err)
		}

		if src == nil {
			continue
		}

		fileName := AIDLToGoFileName(def.GetName())
		if err := writeOutputFile(outDir, fileName, src); err != nil {
			return fmt.Errorf("writing %s: %w", fileName, err)
		}
	}

	return nil
}

// GenerateAll generates Go code for all definitions registered in the resolver.
// If skipErrors is false (default), generation stops at the first error.
// Use SetSkipErrors(true) to skip definitions that fail codegen and continue.
func (g *Generator) GenerateAll() (_err error) {
	registry := g.Resolver.Registry
	allDefs := registry.All()

	// Build the import graph for cycle detection. Augment with edges
	// from existing Go files so that edges added by previous codegen
	// runs are visible to the cycle detector.
	importGraph := BuildImportGraph(registry)

	// Detect Go type name collisions within the same package. When a
	// nested type like "VolumeShaper.Configuration" flattens to
	// "VolumeShaperConfiguration" and a real type with that name also
	// exists, skip the empty forward-declared one.
	skippedDefs := g.detectGoNameCollisions(allDefs)

	// Collect the set of output file paths and directories that will
	// be generated. Used later to remove stale files from previous
	// runs that could introduce import cycles.
	generatedFiles := make(map[string]bool)
	generatedDirs := make(map[string]bool)
	for qualifiedName, def := range allDefs {
		pkg := packageFromDef(qualifiedName, def.GetName())
		outDir := filepath.Join(g.OutputDir, AIDLToGoPackage(pkg))
		fileName := AIDLToGoFileName(def.GetName())
		generatedFiles[filepath.Join(outDir, fileName)] = true
		if !skippedDefs[qualifiedName] {
			generatedDirs[outDir] = true
		}
	}

	// Remove stale generated files for skipped definitions, which may
	// have been written by a previous run.
	for qualifiedName, def := range allDefs {
		if !skippedDefs[qualifiedName] {
			continue
		}
		pkg := packageFromDef(qualifiedName, def.GetName())
		outDir := filepath.Join(g.OutputDir, AIDLToGoPackage(pkg))
		fileName := AIDLToGoFileName(def.GetName())
		_ = os.Remove(filepath.Join(outDir, fileName))
	}

	// Collect cycle types for sub-package generation.
	g.CycleTypes = make(map[string]string)
	cycleTypeCallback := func(qualifiedName, typesPkg string) {
		g.CycleTypes[qualifiedName] = typesPkg
	}

	var errs []error
	for qualifiedName, def := range allDefs {
		if skippedDefs[qualifiedName] {
			continue
		}

		pkg := packageFromDef(qualifiedName, def.GetName())
		goPackage := lastPackageSegment(pkg)
		if goPackage == "" {
			goPackage = "aidl"
		}

		src, err := g.generateDefinition(def, goPackage, qualifiedName,
			WithImportGraph(importGraph),
			WithCycleTypeCallback(cycleTypeCallback),
		)
		if err != nil {
			if g.SkipErrors {
				errs = append(errs, fmt.Errorf("generating %s: %w", qualifiedName, err))
				continue
			}
			return fmt.Errorf("generating %s: %w", qualifiedName, err)
		}

		if src == nil {
			continue
		}

		outDir := filepath.Join(g.OutputDir, AIDLToGoPackage(pkg))
		fileName := AIDLToGoFileName(def.GetName())

		if err := writeOutputFile(outDir, fileName, src); err != nil {
			if g.SkipErrors {
				errs = append(errs, fmt.Errorf("writing %s/%s: %w", outDir, fileName, err))
				continue
			}
			return fmt.Errorf("writing %s/%s: %w", outDir, fileName, err)
		}
	}

	// Generate "types" sub-packages for empty parcelables that were
	// redirected to break import cycles.
	// Generate cycle types iteratively. Each round may discover new
	// types (via cycleTypeCallback during generation with strict SCC).
	// Loop until no new types are discovered.
	generatedCycleTypes := make(map[string]bool)
	for {
		expandedCycleTypes := g.expandCycleTypes(registry)

		newTypes := false
		for qualifiedName, typesPkg := range expandedCycleTypes {
			if generatedCycleTypes[qualifiedName] {
				continue
			}
			generatedCycleTypes[qualifiedName] = true
			newTypes = true

			def, ok := registry.Lookup(qualifiedName)
			if !ok {
				continue
			}
			goTypeName := AIDLToGoName(def.GetName())
			outDir := filepath.Join(g.OutputDir, AIDLToGoPackage(typesPkg))
			fileName := strings.ToLower(goTypeName) + ".go"

			src := g.generateCycleTypeSource(qualifiedName, typesPkg, registry, importGraph, cycleTypeCallback)
			if src == nil {
				continue
			}

			generatedFiles[filepath.Join(outDir, fileName)] = true
			generatedDirs[outDir] = true

			if err := writeOutputFile(outDir, fileName, src); err != nil {
				if g.SkipErrors {
					errs = append(errs, fmt.Errorf("writing cycle type %s: %w", qualifiedName, err))
					continue
				}
				return fmt.Errorf("writing cycle type %s: %w", qualifiedName, err)
			}
		}

		if !newTypes {
			break
		}
	}

	// Note: types sub-packages contain copies of cycle-blocked types
	// for cross-package use. The parent package retains its copy for
	// intra-package references. Both exist intentionally — they are
	// separate Go types used in different import contexts.

	// Remove stale generated files and subdirectories from previous
	// runs. Stale files can introduce import cycles when a previously
	// generated type referenced a cross-package type that no longer
	// exists in the spec.
	for dir := range generatedDirs {
		removeStaleGeneratedFiles(dir, generatedFiles, generatedDirs)
	}

	return errors.Join(errs...)
}

// expandCycleTypes takes the initial set of cycle types and expands it
// to include all co-package type dependencies. When a type `T` in package
// `P` is moved to `P.types`, any types from `P` that `T` references must
// also be in `P.types` (otherwise `P.types` would import `P`, recreating
// the cycle).
func (g *Generator) expandCycleTypes(
	registry *resolver.TypeRegistry,
) map[string]string {
	result := make(map[string]string)
	for k, v := range g.CycleTypes {
		result[k] = v
	}

	// Build a map from original package → types sub-package.
	pkgToTypes := make(map[string]string)
	for qn, typesPkg := range result {
		def, ok := registry.Lookup(qn)
		if !ok {
			continue
		}
		origPkg := packageFromDef(qn, def.GetName())
		pkgToTypes[origPkg] = typesPkg
	}

	// Iteratively expand until no new types are added.
	allDefs := registry.All()
	changed := true
	for changed {
		changed = false
		for qn, typesPkg := range result {
			def, ok := registry.Lookup(qn)
			if !ok {
				continue
			}
			origPkg := packageFromDef(qn, def.GetName())

			// Find types referenced by this definition.
			typeNames := collectTypeNames(def)
			for _, tn := range typeNames {
				// Resolve to see if it's in the same original package.
				resolvedQN := resolveTypeQN(tn, origPkg, registry)
				if resolvedQN == "" {
					continue
				}
				resolvedDef, ok := allDefs[resolvedQN]
				if !ok {
					continue
				}
				resolvedPkg := packageFromDef(resolvedQN, resolvedDef.GetName())
				if resolvedPkg != origPkg {
					continue // different package — not a co-package dependency
				}
				// Skip interfaces — they stay in the original package.
				if _, isIface := resolvedDef.(*parser.InterfaceDecl); isIface {
					continue
				}
				if _, exists := result[resolvedQN]; !exists {
					result[resolvedQN] = typesPkg
					changed = true
				}
			}
			_ = typesPkg
		}
	}

	return result
}

// resolveTypeQN resolves a type name to its qualified name using the registry.
func resolveTypeQN(
	typeName string,
	currentPkg string,
	registry *resolver.TypeRegistry,
) string {
	if _, ok := registry.Lookup(typeName); ok {
		return typeName
	}
	candidate := currentPkg + "." + typeName
	if _, ok := registry.Lookup(candidate); ok {
		return candidate
	}
	if qn, _, ok := registry.LookupQualifiedByShortName(typeName); ok {
		return qn
	}
	return ""
}

// generateCycleTypeSource generates Go source for a type in a "types"
// sub-package. For types with fields, it uses the existing generators
// with no import graph (types-only graph is acyclic). For empty types,
// it generates a minimal stub.
func (g *Generator) generateCycleTypeSource(
	qualifiedName string,
	typesPkg string,
	registry *resolver.TypeRegistry,
	importGraph *ImportGraph,
	cycleTypeCallback func(string, string),
) []byte {
	def, ok := registry.Lookup(qualifiedName)
	if !ok {
		return nil
	}

	goPackage := "types"
	origPkg := packageFromDef(qualifiedName, def.GetName())

	// For interface types, generate ONLY the Go interface definition
	// (no proxy/stub). The proxy/stub stays in the original package.
	if iface, isIface := def.(*parser.InterfaceDecl); isIface {
		return g.generateCycleInterfaceDefinition(iface, goPackage, origPkg, registry)
	}

	// Use the ORIGINAL package as CurrentPkg so that co-package type
	// references resolve as same-package (no import).
	// For types sub-packages, ALL cross-package imports within the SCC
	// must go through types sub-packages (not just back-edges), because
	// any path through the SCC can create cycles when the types
	// sub-package is a node. Use a "strict" graph that treats all
	// intra-SCC edges as cycle-causing.
	strictGraph := importGraph.StrictForSCC(origPkg)
	src, err := g.generateDefinition(def, goPackage, qualifiedName,
		WithCurrentPkg(origPkg),
		WithRegistry(registry),
		WithImportGraph(strictGraph),
		WithCycleTypeCallback(cycleTypeCallback),
	)
	if err != nil || src == nil {
		goTypeName := AIDLToGoName(def.GetName())
		return generateEmptyCycleType(goPackage, goTypeName)
	}
	return src
}

// generateCycleInterfaceDefinition generates a Go interface definition
// for use in a types sub-package. Only the interface type is emitted
// (with AsBinder method); proxy/stub code stays in the original package.
func (g *Generator) generateCycleInterfaceDefinition(
	iface *parser.InterfaceDecl,
	goPackage string,
	origPkg string,
	registry *resolver.TypeRegistry,
) []byte {
	f := NewGoFile(goPackage)
	f.AddImport("github.com/xaionaro-go/binder/binder", "")

	goName := deriveInterfaceName(iface.IntfName)

	f.P("// Code generated by aidlgen. DO NOT EDIT.")
	f.P("")
	f.P("type %s interface {", goName)
	f.P("	AsBinder() binder.IBinder")
	f.P("}")

	src, err := f.Bytes()
	if err != nil {
		return nil
	}
	return src
}

// deriveInterfaceName converts an AIDL interface name to a Go interface name.
// "IServiceManager" → "IServiceManager" (kept as-is since it's already exported).
func deriveInterfaceName(aidlName string) string {
	return AIDLToGoName(aidlName)
}

// generateEmptyCycleType generates a minimal Go source file for an empty
// parcelable type.
func generateEmptyCycleType(goPkg, goTypeName string) []byte {
	f := NewGoFile(goPkg)
	f.AddImport("github.com/xaionaro-go/binder/parcel", "")

	f.P("// Code generated by aidlgen. DO NOT EDIT.")
	f.P("")
	f.P("type %s struct{}", goTypeName)
	f.P("")
	f.P("var _ parcel.Parcelable = (*%s)(nil)", goTypeName)
	f.P("")
	f.P("func (s *%s) MarshalParcel(p *parcel.Parcel) error {", goTypeName)
	f.P("	_headerPos := parcel.WriteParcelableHeader(p)")
	f.P("	parcel.WriteParcelableFooter(p, _headerPos)")
	f.P("	return nil")
	f.P("}")
	f.P("")
	f.P("func (s *%s) UnmarshalParcel(p *parcel.Parcel) error {", goTypeName)
	f.P("	_endPos, _err := parcel.ReadParcelableHeader(p)")
	f.P("	if _err != nil { return _err }")
	f.P("	parcel.SkipToParcelableEnd(p, _endPos)")
	f.P("	return nil")
	f.P("}")

	src, err := f.Bytes()
	if err != nil {
		return nil
	}
	return src
}

// detectGoNameCollisions finds definitions that would produce the same Go
// type name within the same Go package. When a collision is detected, the
// empty (forward-declared) definition is marked for skipping. If both are
// non-empty, the nested one (containing ".") is skipped.
func (g *Generator) detectGoNameCollisions(
	allDefs map[string]parser.Definition,
) map[string]bool {
	type entry struct {
		qualifiedName string
		def           parser.Definition
	}
	// Key: "goPkgPath/GoTypeName"
	seen := map[string]entry{}
	skipped := map[string]bool{}

	for qualifiedName, def := range allDefs {
		pkg := packageFromDef(qualifiedName, def.GetName())
		goPkgPath := AIDLToGoPackage(pkg)
		goTypeName := AIDLToGoName(def.GetName())
		key := goPkgPath + "/" + goTypeName

		prev, exists := seen[key]
		if !exists {
			seen[key] = entry{qualifiedName, def}
			continue
		}

		// Collision detected. Skip whichever is empty/forward-declared.
		prevEmpty := isEmptyParcelable(prev.def)
		currEmpty := isEmptyParcelable(def)
		switch {
		case currEmpty && !prevEmpty:
			skipped[qualifiedName] = true
		case prevEmpty && !currEmpty:
			skipped[prev.qualifiedName] = true
			seen[key] = entry{qualifiedName, def}
		case strings.Contains(def.GetName(), "."):
			// Both non-empty or both empty: skip the nested type.
			skipped[qualifiedName] = true
		default:
			skipped[prev.qualifiedName] = true
			seen[key] = entry{qualifiedName, def}
		}
	}

	return skipped
}

// isEmptyParcelable returns true if the definition is a parcelable with
// no fields, constants, or nested types.
func isEmptyParcelable(def parser.Definition) bool {
	parcDecl, ok := def.(*parser.ParcelableDecl)
	if !ok {
		return false
	}
	return len(parcDecl.Fields) == 0 && len(parcDecl.Constants) == 0 && len(parcDecl.NestedTypes) == 0
}

// GenerateAllSmokeTests generates smoke_test.go files for all packages
// that contain interface definitions. Each file contains one test function
// per proxy type in the package. Only interfaces whose generated proxy
// file exists in the output directory are included.
func (g *Generator) GenerateAllSmokeTests() (_err error) {
	registry := g.Resolver.Registry
	allDefs := registry.All()

	// Group interfaces by AIDL package, filtering to those with
	// successfully generated proxy files.
	pkgInterfaces := make(map[string][]*parser.InterfaceDecl)
	for qualifiedName, def := range allDefs {
		iface, ok := def.(*parser.InterfaceDecl)
		if !ok {
			continue
		}

		pkg := packageFromDef(qualifiedName, def.GetName())
		outDir := filepath.Join(g.OutputDir, AIDLToGoPackage(pkg))
		fileName := AIDLToGoFileName(def.GetName())
		proxyFile := filepath.Join(outDir, fileName)
		if _, err := os.Stat(proxyFile); err != nil {
			continue
		}

		pkgInterfaces[pkg] = append(pkgInterfaces[pkg], iface)
	}

	var errs []error
	for pkg, interfaces := range pkgInterfaces {
		goPackage := lastPackageSegment(pkg)
		if goPackage == "" {
			goPackage = "aidl"
		}

		src, err := GenerateSmokeTests(interfaces, goPackage)
		if err != nil {
			if g.SkipErrors {
				errs = append(errs, fmt.Errorf("generating smoke tests for %s: %w", pkg, err))
				continue
			}
			return fmt.Errorf("generating smoke tests for %s: %w", pkg, err)
		}

		outDir := filepath.Join(g.OutputDir, AIDLToGoPackage(pkg))
		if err := writeOutputFile(outDir, "smoke_test.go", src); err != nil {
			if g.SkipErrors {
				errs = append(errs, fmt.Errorf("writing smoke tests for %s: %w", pkg, err))
				continue
			}
			return fmt.Errorf("writing smoke tests for %s: %w", pkg, err)
		}
	}

	return errors.Join(errs...)
}

// generateDefinition dispatches to the appropriate generator for a definition.
func (g *Generator) generateDefinition(
	def parser.Definition,
	goPackage string,
	qualifiedName string,
	extraOptions ...GenOption,
) ([]byte, error) {
	registry := g.Resolver.Registry
	aidlPkg := packageFromDef(qualifiedName, def.GetName())
	baseOpts := []GenOption{WithRegistry(registry), WithCurrentPkg(aidlPkg)}
	baseOpts = append(baseOpts, extraOptions...)
	switch d := def.(type) {
	case *parser.InterfaceDecl:
		return GenerateInterface(d, goPackage, qualifiedName, baseOpts...)
	case *parser.ParcelableDecl:
		return GenerateParcelable(d, goPackage, qualifiedName, baseOpts...)
	case *parser.EnumDecl:
		return GenerateEnum(d, goPackage, baseOpts...)
	case *parser.UnionDecl:
		return GenerateUnion(d, goPackage, qualifiedName, baseOpts...)
	default:
		return nil, fmt.Errorf("unsupported definition type %T", def)
	}
}

// writeOutputFile creates the directory and writes the file.
func writeOutputFile(
	dir string,
	fileName string,
	src []byte,
) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating directory %s: %w", dir, err)
	}

	filePath := filepath.Join(dir, fileName)
	if err := os.WriteFile(filePath, src, 0o644); err != nil {
		return fmt.Errorf("writing file %s: %w", filePath, err)
	}

	return nil
}

// validateDocument runs semantic validation on a parsed AIDL document
// using the resolver's type registry for type lookup.
func (g *Generator) validateDocument(
	doc *parser.Document,
) []error {
	registry := g.Resolver.Registry
	lookupType := func(qualifiedName string) bool {
		_, ok := registry.Lookup(qualifiedName)
		return ok
	}
	return validate.Validate(doc, lookupType)
}

// lastPackageSegment returns the last segment of a dotted package name.
// "android.os" -> "os"
func lastPackageSegment(pkg string) string {
	for i := len(pkg) - 1; i >= 0; i-- {
		if pkg[i] == '.' {
			return pkg[i+1:]
		}
	}
	return pkg
}

// packageFromQualified extracts the package from a fully qualified name,
// assuming the type name is a single non-dotted identifier.
// "android.os.IServiceManager" -> "android.os"
func packageFromQualified(qualifiedName string) string {
	for i := len(qualifiedName) - 1; i >= 0; i-- {
		if qualifiedName[i] == '.' {
			return qualifiedName[:i]
		}
	}
	return ""
}

// packageFromDef extracts the package from a fully qualified name using
// the definition's own name to determine where the package boundary is.
// This handles nested types correctly:
//
//	qualifiedName="android.view.accessibility.AccessibilityWindowInfo.WindowListSparseArray"
//	defName="AccessibilityWindowInfo.WindowListSparseArray"
//	-> "android.view.accessibility"
func packageFromDef(
	qualifiedName string,
	defName string,
) string {
	if len(defName) >= len(qualifiedName) {
		return ""
	}

	// qualifiedName = pkg + "." + defName
	pkg := qualifiedName[:len(qualifiedName)-len(defName)]
	pkg = strings.TrimRight(pkg, ".")
	return pkg
}
