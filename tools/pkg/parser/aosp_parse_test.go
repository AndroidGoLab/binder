//go:build aosp_parse

package parser

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestParseAllAOSP(t *testing.T) {
	rootDir := filepath.Join("..", "3rdparty")
	if _, err := os.Stat(rootDir); os.IsNotExist(err) {
		t.Skip("3rdparty directory not found; skipping AOSP parse test")
	}

	var files []string
	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".aidl") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking 3rdparty: %v", err)
	}

	if len(files) == 0 {
		t.Fatal("no .aidl files found in 3rdparty/")
	}

	t.Logf("Found %d .aidl files", len(files))

	type failureRecord struct {
		path   string
		errMsg string
	}

	var (
		successCount int
		failCount    int
		failures     = make(map[string][]failureRecord) // category -> records
	)

	for _, path := range files {
		_, parseErr := ParseFile(path)
		if parseErr == nil {
			successCount++
			continue
		}

		failCount++
		category := categorizeError(parseErr.Error())
		failures[category] = append(failures[category], failureRecord{
			path:   path,
			errMsg: parseErr.Error(),
		})
	}

	total := successCount + failCount
	successPct := float64(successCount) / float64(total) * 100
	failPct := float64(failCount) / float64(total) * 100

	t.Logf("")
	t.Logf("=== AOSP Parse Results ===")
	t.Logf("Total files:  %d", total)
	t.Logf("Success:      %d (%.1f%%)", successCount, successPct)
	t.Logf("Failed:       %d (%.1f%%)", failCount, failPct)
	t.Logf("")

	// Sort categories by count descending.
	type categoryEntry struct {
		category string
		records  []failureRecord
	}
	var sorted []categoryEntry
	for cat, recs := range failures {
		sorted = append(sorted, categoryEntry{category: cat, records: recs})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return len(sorted[i].records) > len(sorted[j].records)
	})

	t.Logf("=== Failure Categories ===")
	for _, entry := range sorted {
		t.Logf("")
		t.Logf("Category: %s (%d files)", entry.category, len(entry.records))

		// Show up to 3 example files per category.
		limit := 3
		if len(entry.records) < limit {
			limit = len(entry.records)
		}
		for i := 0; i < limit; i++ {
			t.Logf("  example: %s", entry.records[i].path)
			t.Logf("    error: %s", entry.records[i].errMsg)
		}
	}

	// Per-submodule breakdown.
	type submoduleStats struct {
		total   int
		success int
	}
	submodules := make(map[string]*submoduleStats)

	failedFiles := make(map[string]bool)
	for _, recs := range failures {
		for _, r := range recs {
			failedFiles[r.path] = true
		}
	}

	for _, path := range files {
		rel, _ := filepath.Rel(rootDir, path)
		parts := strings.SplitN(rel, string(filepath.Separator), 2)
		name := parts[0]
		if submodules[name] == nil {
			submodules[name] = &submoduleStats{}
		}
		submodules[name].total++
		if !failedFiles[path] {
			submodules[name].success++
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
		pct := float64(s.success) / float64(s.total) * 100
		t.Logf("  %-30s %5d / %5d  (%.1f%%)", name, s.success, s.total, pct)
	}
}

// categorizeError maps parse error messages to root-cause categories.
//
// The categories correspond to specific AIDL language features that the
// parser does not yet support.
func categorizeError(errMsg string) string {
	switch {
	// Nested type definition: enum/parcelable/union/interface inside another type.
	// Parser tries to parse a field type, sees "enum"/"parcelable"/etc. keyword
	// and fails because it expects an identifier (type name) not a keyword.
	case strings.Contains(errMsg, "got enum"):
		return "nested type definition (enum inside interface/parcelable)"

	case strings.Contains(errMsg, "got parcelable"):
		return "nested type definition (parcelable inside parcelable)"

	case strings.Contains(errMsg, "got union"):
		return "nested type definition (union inside type)"

	case strings.Contains(errMsg, "got interface"):
		return "nested type definition (interface inside type)"

	// @EnforcePermission("SET_TIME") -- annotation with positional string arg
	// instead of key=value pairs.
	case strings.Contains(errMsg, "expected identifier, got string"):
		return "annotation with positional value (e.g. @EnforcePermission(\"...\"))"

	// byte[6] bdAddr -- fixed-size array with integer size.
	case strings.Contains(errMsg, "expected ], got integer"):
		return "fixed-size array (e.g. byte[6])"

	// byte[BYTE_SIZE_OF_CACHE_TOKEN] -- fixed-size array with const-expr size.
	case strings.Contains(errMsg, "expected ], got identifier"):
		return "fixed-size array with const-expr size (e.g. byte[CONST])"

	// parcelable ActivityManager.MemoryInfo; -- dotted name in parcelable decl.
	case strings.Contains(errMsg, "expected {, got ."):
		return "dotted parcelable name (e.g. parcelable Foo.Bar)"

	// Multiple foreign-language header directives on a parcelable declaration.
	case strings.Contains(errMsg, "ndk_header") || strings.Contains(errMsg, "rust_type"):
		return "multiple foreign headers (e.g. cpp_header + ndk_header + rust_type)"

	// Map<String, List<Foo>> -- nested generics produce '>>' token.
	case strings.Contains(errMsg, "expected >, got >>"):
		return "nested generics (>> tokenized as shift instead of two closes)"

	// parcelable Foo<T>; -- generic type parameters on parcelable declarations.
	case strings.Contains(errMsg, "expected {, got <"):
		return "generic type parameters on parcelable (e.g. parcelable Foo<T>)"

	// 0xFFu8 -- typed integer suffix not handled by lexer.
	case strings.Contains(errMsg, "got identifier (\"u8\")") ||
		strings.Contains(errMsg, "got identifier (\"u4\")"):
		return "typed integer suffix (e.g. 0xFFu8)"

	// Catch-all categories.
	case strings.Contains(errMsg, "expected ;"):
		return "expected ';' (other)"

	case strings.Contains(errMsg, "expected identifier"):
		return "expected identifier (other)"

	case strings.Contains(errMsg, "expected {"):
		return "expected '{' (other)"

	case strings.Contains(errMsg, "expected definition"):
		return "unexpected token at top level"

	case strings.Contains(errMsg, "expected constant expression"):
		return "expected constant expression"

	case strings.Contains(errMsg, "unexpected character"):
		return "unexpected character (lexer error)"

	case strings.Contains(errMsg, "unterminated"):
		return "unterminated literal"

	case strings.Contains(errMsg, "reading"):
		return "file read error"

	default:
		return errMsg
	}
}
