package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/xaionaro-go/aidl/tools/pkg/codegen"
	"github.com/xaionaro-go/aidl/tools/pkg/parser"
	"github.com/xaionaro-go/aidl/tools/pkg/resolver"
)

func main() {
	outputDir := flag.String("output", ".", "Output directory for generated Go files")
	thirdpartyDir := flag.String("3rdparty", "tools/pkg/3rdparty", "Path to the 3rdparty directory containing AOSP submodules")
	smokeTests := flag.Bool("smoke-tests", false, "Generate smoke tests for all proxy types")
	flag.Parse()

	if err := run(*thirdpartyDir, *outputDir, *smokeTests); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(
	thirdpartyDir string,
	outputDir string,
	smokeTests bool,
) error {
	absThirdparty, err := filepath.Abs(thirdpartyDir)
	if err != nil {
		return fmt.Errorf("resolving 3rdparty path: %w", err)
	}

	if _, err := os.Stat(absThirdparty); os.IsNotExist(err) {
		return fmt.Errorf("3rdparty directory not found: %s", absThirdparty)
	}

	// Discover all AIDL files (excluding versioned aidl_api snapshots and test dirs).
	fmt.Fprintf(os.Stderr, "Discovering AIDL files in %s...\n", absThirdparty)
	aidlFiles, err := discoverAIDLFiles(absThirdparty)
	if err != nil {
		return fmt.Errorf("discovering AIDL files: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Found %d AIDL files\n", len(aidlFiles))

	// Discover search roots by analyzing package declarations.
	fmt.Fprintf(os.Stderr, "Discovering search roots...\n")
	searchRoots, err := discoverSearchRoots(aidlFiles)
	if err != nil {
		return fmt.Errorf("discovering search roots: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Found %d search roots\n", len(searchRoots))

	// Create resolver with all search roots and skip-unresolved mode.
	r := resolver.New(searchRoots)
	r.SetSkipUnresolved(true)

	// Parse all files, register definitions, and resolve imports best-effort.
	var parseFailCount int
	var resolvedCount int
	for _, f := range aidlFiles {
		if err := r.ResolveFile(f); err != nil {
			parseFailCount++
			continue
		}
		resolvedCount++
	}
	fmt.Fprintf(os.Stderr, "Resolved %d files (%d parse failures)\n", resolvedCount, parseFailCount)

	// Count definitions, excluding forward declarations.
	allDefs := r.Registry().All()
	realDefCount := 0
	for _, def := range allDefs {
		if isRealDefinition(def) {
			realDefCount++
		}
	}
	fmt.Fprintf(os.Stderr, "Total definitions: %d (real: %d)\n", len(allDefs), realDefCount)

	// Generate Go code (skip individual definition errors to maximize output).
	fmt.Fprintf(os.Stderr, "Generating Go code into %s...\n", outputDir)
	gen := codegen.NewGenerator(r, outputDir)
	gen.SetSkipErrors(true)
	if err := gen.GenerateAll(); err != nil {
		// Count individual codegen errors.
		codegenErrors := strings.Split(err.Error(), "\n")
		fmt.Fprintf(os.Stderr, "Codegen completed with %d definition errors (skipped)\n", len(codegenErrors))
	}

	// Generate smoke tests if requested.
	if smokeTests {
		fmt.Fprintf(os.Stderr, "Generating smoke tests...\n")
		if err := gen.GenerateAllSmokeTests(); err != nil {
			smokeErrors := strings.Split(err.Error(), "\n")
			fmt.Fprintf(os.Stderr, "Smoke test generation completed with %d errors (skipped)\n", len(smokeErrors))
		}
	}

	// Count generated files.
	genCount, err := countGeneratedFiles(outputDir)
	if err != nil {
		return fmt.Errorf("counting generated files: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Generated %d Go files\n", genCount)

	return nil
}

// discoverAIDLFiles walks the directory tree and returns all .aidl files,
// excluding versioned aidl_api snapshots.
func discoverAIDLFiles(
	rootDir string,
) ([]string, error) {
	var files []string
	err := filepath.Walk(rootDir, func(
		path string,
		info os.FileInfo,
		err error,
	) error {
		if err != nil {
			return nil // skip inaccessible paths
		}

		if info.IsDir() {
			base := info.Name()
			// Skip versioned API snapshot directories and test directories.
			if base == "aidl_api" {
				return filepath.SkipDir
			}
			return nil
		}

		if strings.HasSuffix(path, ".aidl") {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

// discoverSearchRoots determines import root directories by analyzing
// package declarations in AIDL files and inferring the root from the
// file's path relative to the package structure.
func discoverSearchRoots(
	aidlFiles []string,
) ([]string, error) {
	rootSet := make(map[string]bool)
	for _, f := range aidlFiles {
		root, err := inferSearchRoot(f)
		if err != nil {
			continue // skip files that can't be analyzed
		}
		if root != "" {
			rootSet[root] = true
		}
	}

	roots := make([]string, 0, len(rootSet))
	for r := range rootSet {
		roots = append(roots, r)
	}
	return roots, nil
}

// inferSearchRoot parses an AIDL file's package declaration and computes
// the search root directory by stripping the package-derived path suffix
// from the file's directory.
func inferSearchRoot(
	filePath string,
) (string, error) {
	doc, err := parser.ParseFile(filePath)
	if err != nil {
		return "", err
	}

	if doc.Package == nil || doc.Package.Name == "" {
		return "", nil
	}

	pkgPath := strings.ReplaceAll(doc.Package.Name, ".", string(filepath.Separator))
	dir := filepath.Dir(filePath)

	// The directory should end with the package path.
	if !strings.HasSuffix(dir, pkgPath) {
		return "", nil
	}

	root := strings.TrimSuffix(dir, pkgPath)
	root = strings.TrimRight(root, string(filepath.Separator))
	if root == "" {
		return "", nil
	}

	return root, nil
}

// isRealDefinition returns true if the definition contains actual content.
func isRealDefinition(def parser.Definition) bool {
	switch d := def.(type) {
	case *parser.ParcelableDecl:
		return len(d.Fields) > 0 || len(d.Constants) > 0 || len(d.NestedTypes) > 0
	case *parser.InterfaceDecl:
		return true
	case *parser.EnumDecl:
		return true
	case *parser.UnionDecl:
		return true
	default:
		return false
	}
}

// countGeneratedFiles counts .go files in the output directory tree.
func countGeneratedFiles(
	dir string,
) (int, error) {
	count := 0
	err := filepath.Walk(dir, func(
		path string,
		info os.FileInfo,
		err error,
	) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && strings.HasSuffix(path, ".go") {
			count++
		}
		return nil
	})
	return count, err
}
