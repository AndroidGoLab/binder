//go:build aosp_codegen

package codegen

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	aidlparser "github.com/AndroidGoLab/binder/tools/pkg/parser"
)

type codegenFailureRecord struct {
	path     string
	defName  string
	defType  string
	errMsg   string
	goSource string
}

// TestCodegenAllAOSP runs the codegen pipeline on all AOSP .aidl files that
// contain real definitions (not just forward declarations like `parcelable Foo;`).
//
// For each file it:
//  1. Parses the file
//  2. Skips files that only contain forward declarations
//  3. Attempts to generate Go code for each definition
//  4. Verifies the generated Go code parses as valid Go
//  5. Reports success/failure counts and categorizes failures
func TestCodegenAllAOSP(t *testing.T) {
	rootDir := filepath.Join("..", "3rdparty")
	if _, err := os.Stat(rootDir); os.IsNotExist(err) {
		t.Skip("3rdparty directory not found; skipping AOSP codegen test")
	}

	var allFiles []string
	err := filepath.Walk(rootDir, func(
		path string,
		info os.FileInfo,
		err error,
	) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".aidl") {
			allFiles = append(allFiles, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking 3rdparty: %v", err)
	}

	if len(allFiles) == 0 {
		t.Fatal("no .aidl files found in 3rdparty/")
	}

	t.Logf("Found %d total .aidl files", len(allFiles))

	var (
		totalFiles            int
		parseFailCount        int
		forwardDeclCount      int
		filesWithRealDefs     int
		totalDefinitions      int
		codegenSuccessCount   int
		codegenFailCount      int
		goParseSuccessCount   int
		goParseFailCount      int
		codegenFailures       = make(map[string][]codegenFailureRecord)
		goParseFailures       = make(map[string][]codegenFailureRecord)
	)

	for _, path := range allFiles {
		totalFiles++

		doc, parseErr := aidlparser.ParseFile(path)
		if parseErr != nil {
			parseFailCount++
			continue
		}

		if len(doc.Definitions) == 0 {
			forwardDeclCount++
			continue
		}

		// Check whether the file has any "real" definitions.
		hasRealDef := false
		for _, def := range doc.Definitions {
			if isRealDefinition(def) {
				hasRealDef = true
				break
			}
		}
		if !hasRealDef {
			forwardDeclCount++
			continue
		}

		filesWithRealDefs++

		pkg := ""
		if doc.Package != nil {
			pkg = doc.Package.Name
		}
		goPackage := lastPackageSegment(pkg)
		if goPackage == "" {
			goPackage = "gen"
		}

		for _, def := range doc.Definitions {
			if !isRealDefinition(def) {
				continue
			}

			totalDefinitions++

			qualifiedName := def.GetName()
			if pkg != "" {
				qualifiedName = pkg + "." + def.GetName()
			}

			src, codegenErr := generateDefinitionForTest(def, goPackage, qualifiedName)
			if codegenErr != nil {
				codegenFailCount++
				category := categorizeCodegenError(codegenErr.Error())
				codegenFailures[category] = append(codegenFailures[category], codegenFailureRecord{
					path:    path,
					defName: qualifiedName,
					defType: defTypeName(def),
					errMsg:  codegenErr.Error(),
				})
				continue
			}

			if src == nil {
				// Definition produced no output (e.g., forward-declared parcelable
				// that slipped through). Do not count as failure.
				totalDefinitions--
				continue
			}

			codegenSuccessCount++

			// Verify the generated Go code parses.
			fset := token.NewFileSet()
			_, goParseErr := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
			if goParseErr != nil {
				goParseFailCount++
				category := categorizeGoParseError(goParseErr.Error())
				goParseFailures[category] = append(goParseFailures[category], codegenFailureRecord{
					path:     path,
					defName:  qualifiedName,
					defType:  defTypeName(def),
					errMsg:   goParseErr.Error(),
					goSource: truncateSource(string(src), 500),
				})
			} else {
				goParseSuccessCount++
			}
		}
	}

	// Report results.
	t.Logf("")
	t.Logf("=== AOSP Codegen Results ===")
	t.Logf("Total .aidl files:            %d", totalFiles)
	t.Logf("Parse failures (skip):        %d", parseFailCount)
	t.Logf("Forward declarations (skip):  %d", forwardDeclCount)
	t.Logf("Files with real definitions:  %d", filesWithRealDefs)
	t.Logf("Total definitions attempted:  %d", totalDefinitions)
	t.Logf("")
	t.Logf("--- Codegen ---")
	t.Logf("Codegen success:  %d / %d (%.1f%%)",
		codegenSuccessCount, totalDefinitions,
		pct(codegenSuccessCount, totalDefinitions))
	t.Logf("Codegen fail:     %d / %d (%.1f%%)",
		codegenFailCount, totalDefinitions,
		pct(codegenFailCount, totalDefinitions))
	t.Logf("")
	t.Logf("--- Go Parse (of successful codegen) ---")
	t.Logf("Go parse success: %d / %d (%.1f%%)",
		goParseSuccessCount, codegenSuccessCount,
		pct(goParseSuccessCount, codegenSuccessCount))
	t.Logf("Go parse fail:    %d / %d (%.1f%%)",
		goParseFailCount, codegenSuccessCount,
		pct(goParseFailCount, codegenSuccessCount))

	if len(codegenFailures) > 0 {
		t.Logf("")
		t.Logf("=== Codegen Failure Categories ===")
		printFailureCategories(t, codegenFailures)
	}

	if len(goParseFailures) > 0 {
		t.Logf("")
		t.Logf("=== Go Parse Failure Categories ===")
		printFailureCategories(t, goParseFailures)
	}

	// Per-submodule breakdown.
	printSubmoduleBreakdown(t, allFiles, rootDir)
}

// isRealDefinition returns true if the definition contains actual content
// (not just a forward declaration like `parcelable Foo;`).
func isRealDefinition(def aidlparser.Definition) bool {
	switch d := def.(type) {
	case *aidlparser.ParcelableDecl:
		// Forward-declared parcelables have no fields and typically have
		// a cpp_header, ndk_header, or rust_type.
		if len(d.Fields) == 0 && len(d.Constants) == 0 && len(d.NestedTypes) == 0 {
			return false
		}
		return true
	case *aidlparser.InterfaceDecl:
		return true
	case *aidlparser.EnumDecl:
		return true
	case *aidlparser.UnionDecl:
		return true
	default:
		return false
	}
}

// generateDefinitionForTest dispatches to the appropriate generator.
func generateDefinitionForTest(
	def aidlparser.Definition,
	goPackage string,
	qualifiedName string,
) ([]byte, error) {
	switch d := def.(type) {
	case *aidlparser.InterfaceDecl:
		return GenerateInterface(d, goPackage, qualifiedName)
	case *aidlparser.ParcelableDecl:
		return GenerateParcelable(d, goPackage, qualifiedName)
	case *aidlparser.EnumDecl:
		return GenerateEnum(d, goPackage)
	case *aidlparser.UnionDecl:
		return GenerateUnion(d, goPackage, qualifiedName)
	default:
		return nil, nil
	}
}

// defTypeName returns a human-readable type name for a definition.
func defTypeName(def aidlparser.Definition) string {
	switch def.(type) {
	case *aidlparser.InterfaceDecl:
		return "interface"
	case *aidlparser.ParcelableDecl:
		return "parcelable"
	case *aidlparser.EnumDecl:
		return "enum"
	case *aidlparser.UnionDecl:
		return "union"
	default:
		return "unknown"
	}
}

// categorizeCodegenError maps codegen error messages to root-cause categories.
func categorizeCodegenError(errMsg string) string {
	switch {
	case strings.Contains(errMsg, "unsupported constant expression type"):
		return "unsupported constant expression type"
	case strings.Contains(errMsg, "unsupported definition type"):
		return "unsupported definition type"
	case strings.Contains(errMsg, "gofmt failed"):
		return "gofmt failed (invalid generated Go)"
	case strings.Contains(errMsg, "evaluating enumerator"):
		return "evaluating enumerator expression"
	case strings.Contains(errMsg, "evaluating constant"):
		return "evaluating constant expression"
	default:
		// Extract a short prefix for unknown categories.
		if len(errMsg) > 80 {
			return errMsg[:80] + "..."
		}
		return errMsg
	}
}

// categorizeGoParseError maps Go parse error messages to categories.
func categorizeGoParseError(errMsg string) string {
	switch {
	case strings.Contains(errMsg, "expected"):
		// Extract the "expected X, found Y" part.
		idx := strings.Index(errMsg, "expected")
		end := strings.Index(errMsg[idx:], "\n")
		if end > 0 {
			return errMsg[idx : idx+end]
		}
		if len(errMsg) > 120 {
			return errMsg[:120]
		}
		return errMsg
	default:
		if len(errMsg) > 120 {
			return errMsg[:120]
		}
		return errMsg
	}
}

// printFailureCategories prints categorized failure records sorted by count.
func printFailureCategories(
	t *testing.T,
	failures map[string][]codegenFailureRecord,
) {
	t.Helper()

	type categoryEntry struct {
		category string
		records  []codegenFailureRecord
	}

	var sorted []categoryEntry
	for cat, recs := range failures {
		sorted = append(sorted, categoryEntry{category: cat, records: recs})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return len(sorted[i].records) > len(sorted[j].records)
	})

	for _, entry := range sorted {
		t.Logf("")
		t.Logf("Category: %s (%d definitions)", entry.category, len(entry.records))

		limit := 3
		if len(entry.records) < limit {
			limit = len(entry.records)
		}
		for i := 0; i < limit; i++ {
			r := entry.records[i]
			t.Logf("  [%s] %s (%s)", r.defType, r.defName, r.path)
			t.Logf("    error: %s", r.errMsg)
			if r.goSource != "" {
				t.Logf("    generated source (truncated):\n%s", r.goSource)
			}
		}
	}
}

// printSubmoduleBreakdown prints per-submodule statistics.
func printSubmoduleBreakdown(
	t *testing.T,
	allFiles []string,
	rootDir string,
) {
	t.Helper()

	type submoduleStats struct {
		total       int
		parsed      int
		realDefs    int
	}
	submodules := make(map[string]*submoduleStats)

	for _, path := range allFiles {
		rel, _ := filepath.Rel(rootDir, path)
		parts := strings.SplitN(rel, string(filepath.Separator), 2)
		name := parts[0]
		if submodules[name] == nil {
			submodules[name] = &submoduleStats{}
		}
		submodules[name].total++

		doc, parseErr := aidlparser.ParseFile(path)
		if parseErr != nil {
			continue
		}
		submodules[name].parsed++

		for _, def := range doc.Definitions {
			if isRealDefinition(def) {
				submodules[name].realDefs++
				break
			}
		}
	}

	t.Logf("")
	t.Logf("=== Per-Submodule Breakdown ===")
	var subNames []string
	for name := range submodules {
		subNames = append(subNames, name)
	}
	sort.Strings(subNames)
	for _, name := range subNames {
		s := submodules[name]
		t.Logf("  %-35s total: %5d  parsed: %5d  with real defs: %5d",
			name, s.total, s.parsed, s.realDefs)
	}
}

// pct computes a percentage, returning 0 if denominator is 0.
func pct(
	numerator int,
	denominator int,
) float64 {
	if denominator == 0 {
		return 0
	}
	return float64(numerator) / float64(denominator) * 100
}

// truncateSource returns at most maxLen characters of source.
func truncateSource(
	src string,
	maxLen int,
) string {
	if len(src) <= maxLen {
		return src
	}
	return src[:maxLen] + "\n... (truncated)"
}
