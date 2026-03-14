package codegen

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/xaionaro-go/aidl/tools/pkg/parser"
	"github.com/xaionaro-go/aidl/tools/pkg/resolver"
	"github.com/xaionaro-go/aidl/tools/pkg/validate"
)

// Generator generates Go code from AIDL files.
type Generator struct {
	resolver   *resolver.Resolver
	outputDir  string
	skipErrors bool
}

// NewGenerator creates a Generator that writes output to outputDir.
func NewGenerator(
	resolver *resolver.Resolver,
	outputDir string,
) *Generator {
	return &Generator{
		resolver:  resolver,
		outputDir: outputDir,
	}
}

// SetSkipErrors configures whether the generator skips definitions
// that fail codegen and continues with the rest.
func (g *Generator) SetSkipErrors(
	skip bool,
) {
	g.skipErrors = skip
}

// GenerateFile generates Go code for all definitions in a single AIDL document.
// It runs semantic validation before code generation.
func (g *Generator) GenerateFile(
	doc *parser.Document,
) (_err error) {
	if errs := g.validateDocument(doc); len(errs) > 0 {
		if !g.skipErrors {
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

	outDir := filepath.Join(g.outputDir, AIDLToGoPackage(pkg))

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
	registry := g.resolver.Registry()
	allDefs := registry.All()

	// Build the import graph for cycle detection.
	importGraph := BuildImportGraph(registry)

	var errs []error
	for qualifiedName, def := range allDefs {
		pkg := packageFromDef(qualifiedName, def.GetName())
		goPackage := lastPackageSegment(pkg)
		if goPackage == "" {
			goPackage = "aidl"
		}

		src, err := g.generateDefinition(def, goPackage, qualifiedName, WithImportGraph(importGraph))
		if err != nil {
			if g.skipErrors {
				errs = append(errs, fmt.Errorf("generating %s: %w", qualifiedName, err))
				continue
			}
			return fmt.Errorf("generating %s: %w", qualifiedName, err)
		}

		if src == nil {
			continue
		}

		outDir := filepath.Join(g.outputDir, AIDLToGoPackage(pkg))
		fileName := AIDLToGoFileName(def.GetName())

		if err := writeOutputFile(outDir, fileName, src); err != nil {
			if g.skipErrors {
				errs = append(errs, fmt.Errorf("writing %s/%s: %w", outDir, fileName, err))
				continue
			}
			return fmt.Errorf("writing %s/%s: %w", outDir, fileName, err)
		}
	}

	return errors.Join(errs...)
}

// GenerateAllSmokeTests generates smoke_test.go files for all packages
// that contain interface definitions. Each file contains one test function
// per proxy type in the package. Only interfaces whose generated proxy
// file exists in the output directory are included.
func (g *Generator) GenerateAllSmokeTests() (_err error) {
	registry := g.resolver.Registry()
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
		outDir := filepath.Join(g.outputDir, AIDLToGoPackage(pkg))
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
			if g.skipErrors {
				errs = append(errs, fmt.Errorf("generating smoke tests for %s: %w", pkg, err))
				continue
			}
			return fmt.Errorf("generating smoke tests for %s: %w", pkg, err)
		}

		outDir := filepath.Join(g.outputDir, AIDLToGoPackage(pkg))
		if err := writeOutputFile(outDir, "smoke_test.go", src); err != nil {
			if g.skipErrors {
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
	registry := g.resolver.Registry()
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
	registry := g.resolver.Registry()
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
