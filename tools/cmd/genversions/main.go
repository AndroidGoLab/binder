package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/xaionaro-go/aidl/binder"
	"github.com/xaionaro-go/aidl/tools/pkg/parser"
)

// apiLevelTag maps Android API levels to the AOSP git tags used for
// checking out the corresponding source code.
var apiLevelTag = map[int]string{
	34: "android-14.0.0_r1",
	35: "android-15.0.0_r1",
	36: "android-16.0.0_r1",
}

// submoduleNames lists the 3rdparty submodule directory basenames.
var submoduleNames = []string{
	"frameworks-base",
	"frameworks-native",
	"hardware-interfaces",
	"system-hardware-interfaces",
}

func main() {
	defaultAPI := flag.Int("default-api", 36, "API level that the compiled proxy code was generated against")
	thirdpartyDir := flag.String("3rdparty", "tools/pkg/3rdparty", "Path to the 3rdparty directory containing AOSP submodules")
	outputFile := flag.String("output", "binder/versionaware/codes_gen.go", "Output file path for generated code")
	flag.Parse()

	if err := run(*defaultAPI, *thirdpartyDir, *outputFile); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(
	defaultAPI int,
	thirdpartyDir string,
	outputFile string,
) error {
	absThirdparty, err := filepath.Abs(thirdpartyDir)
	if err != nil {
		return fmt.Errorf("resolving 3rdparty path: %w", err)
	}

	if _, err := os.Stat(absThirdparty); os.IsNotExist(err) {
		return fmt.Errorf("3rdparty directory not found: %s", absThirdparty)
	}

	submoduleDirs := make([]string, len(submoduleNames))
	for i, name := range submoduleNames {
		submoduleDirs[i] = filepath.Join(absThirdparty, name)
	}

	// Save current commits so we can restore them after checkout.
	originalCommits, err := saveCurrentCommits(submoduleDirs)
	if err != nil {
		return fmt.Errorf("saving current commits: %w", err)
	}

	// Ensure submodules are restored regardless of how we exit.
	defer restoreCommits(submoduleDirs, originalCommits)

	// Collect sorted API levels for deterministic iteration.
	apiLevels := sortedAPILevels()

	// Parse each API level's AIDL files into a version table.
	allTables := make(map[int]map[string]map[string]binder.TransactionCode, len(apiLevels))
	for _, level := range apiLevels {
		tag := apiLevelTag[level]
		fmt.Fprintf(os.Stderr, "Fetching API %d (tag %s)...\n", level, tag)

		if err := checkoutTag(submoduleDirs, tag); err != nil {
			return fmt.Errorf("checking out tag %s for API %d: %w", tag, level, err)
		}

		table, err := parseVersionTable(absThirdparty)
		if err != nil {
			return fmt.Errorf("parsing API %d: %w", level, err)
		}
		allTables[level] = table

		fmt.Fprintf(os.Stderr, "API %d: %d interfaces\n", level, len(table))
	}

	// Restore submodules before writing output (the defer also covers panics).
	restoreCommits(submoduleDirs, originalCommits)

	// Generate and write the output file.
	src, err := generateSource(defaultAPI, apiLevels, allTables)
	if err != nil {
		return fmt.Errorf("generating source: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(outputFile), 0o755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}
	if err := os.WriteFile(outputFile, src, 0o644); err != nil {
		return fmt.Errorf("writing output: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Wrote %s (%d bytes)\n", outputFile, len(src))
	return nil
}

// sortedAPILevels returns the API level keys in ascending order.
func sortedAPILevels() []int {
	levels := make([]int, 0, len(apiLevelTag))
	for level := range apiLevelTag {
		levels = append(levels, level)
	}
	sort.Ints(levels)
	return levels
}

// saveCurrentCommits records HEAD for each submodule directory.
func saveCurrentCommits(
	dirs []string,
) (map[string]string, error) {
	commits := make(map[string]string, len(dirs))
	for _, dir := range dirs {
		out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
		if err != nil {
			return nil, fmt.Errorf("rev-parse HEAD in %s: %w", dir, err)
		}
		commits[dir] = strings.TrimSpace(string(out))
	}
	return commits, nil
}

// restoreCommits checks out the original commit in each submodule.
func restoreCommits(
	dirs []string,
	commits map[string]string,
) {
	for _, dir := range dirs {
		commit, ok := commits[dir]
		if !ok {
			continue
		}
		cmd := exec.Command("git", "-C", dir, "checkout", commit)
		cmd.Stderr = os.Stderr
		_ = cmd.Run()
	}
}

// checkoutTag fetches and checks out a tag in all submodule directories.
func checkoutTag(
	dirs []string,
	tag string,
) error {
	for _, dir := range dirs {
		fetch := exec.Command("git", "-C", dir, "fetch", "--depth=1", "origin", "tag", tag)
		fetch.Stderr = os.Stderr
		if err := fetch.Run(); err != nil {
			return fmt.Errorf("fetching tag %s in %s: %w", tag, filepath.Base(dir), err)
		}

		checkout := exec.Command("git", "-C", dir, "checkout", "FETCH_HEAD")
		checkout.Stderr = os.Stderr
		if err := checkout.Run(); err != nil {
			return fmt.Errorf("checking out FETCH_HEAD in %s: %w", filepath.Base(dir), err)
		}
	}
	return nil
}

// parseVersionTable walks the 3rdparty tree, parses all .aidl files,
// and builds a map of descriptor -> method -> transaction code.
func parseVersionTable(
	thirdpartyDir string,
) (map[string]map[string]binder.TransactionCode, error) {
	aidlFiles, err := discoverAIDLFiles(thirdpartyDir)
	if err != nil {
		return nil, fmt.Errorf("discovering AIDL files: %w", err)
	}
	fmt.Fprintf(os.Stderr, "  Parsing %d AIDL files...\n", len(aidlFiles))

	table := make(map[string]map[string]binder.TransactionCode)
	var parseFailCount int

	for _, path := range aidlFiles {
		doc, err := parser.ParseFile(path)
		if err != nil {
			parseFailCount++
			continue
		}

		if doc.Package == nil || doc.Package.Name == "" {
			continue
		}

		extractInterfaces(doc.Package.Name, doc.Definitions, table)
	}

	fmt.Fprintf(os.Stderr, "  Found %d interfaces (%d parse failures)\n", len(table), parseFailCount)
	return table, nil
}

// extractInterfaces recursively extracts interface declarations from a
// list of definitions, handling nested types.
func extractInterfaces(
	packageName string,
	defs []parser.Definition,
	table map[string]map[string]binder.TransactionCode,
) {
	for _, def := range defs {
		iface, ok := def.(*parser.InterfaceDecl)
		if !ok {
			continue
		}
		if len(iface.Methods) == 0 {
			continue
		}

		descriptor := packageName + "." + iface.IntfName
		methods := make(map[string]binder.TransactionCode, len(iface.Methods))

		counter := 0
		for _, m := range iface.Methods {
			if m.TransactionID != 0 {
				counter = m.TransactionID - 1
			}
			code := binder.FirstCallTransaction + binder.TransactionCode(counter)
			methods[m.MethodName] = code
			counter++
		}

		table[descriptor] = methods

		// Process nested types within this interface.
		if len(iface.NestedTypes) > 0 {
			extractInterfaces(descriptor, iface.NestedTypes, table)
		}
	}
}

// discoverAIDLFiles walks the directory tree and returns all .aidl files,
// excluding versioned aidl_api snapshot directories.
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
			return nil
		}

		if info.IsDir() {
			if info.Name() == "aidl_api" {
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

// generateSource produces the Go source for codes_gen.go.
func generateSource(
	defaultAPI int,
	apiLevels []int,
	allTables map[int]map[string]map[string]binder.TransactionCode,
) ([]byte, error) {
	var buf bytes.Buffer

	buf.WriteString("// Code generated by genversions. DO NOT EDIT.\n\n")
	buf.WriteString("package versionaware\n\n")
	buf.WriteString("import \"github.com/xaionaro-go/aidl/binder\"\n\n")

	fmt.Fprintf(&buf, "func init() {\n")
	fmt.Fprintf(&buf, "\tDefaultAPILevel = %d\n", defaultAPI)
	buf.WriteString("\tTables = MultiVersionTable{\n")

	for _, level := range apiLevels {
		table := allTables[level]
		fmt.Fprintf(&buf, "\t\t%d: VersionTable{\n", level)

		descriptors := sortedKeys(table)
		for _, desc := range descriptors {
			methods := table[desc]
			if len(methods) == 0 {
				continue
			}

			fmt.Fprintf(&buf, "\t\t\t%q: {\n", desc)

			methodNames := sortedKeys(methods)
			for _, name := range methodNames {
				code := methods[name]
				offset := code - binder.FirstCallTransaction
				fmt.Fprintf(&buf, "\t\t\t\t%q: binder.FirstCallTransaction + %d,\n", name, offset)
			}

			buf.WriteString("\t\t\t},\n")
		}

		buf.WriteString("\t\t},\n")
	}

	buf.WriteString("\t}\n")
	buf.WriteString("}\n")

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("formatting generated code: %w\n\nRaw source:\n%s", err, buf.String())
	}
	return formatted, nil
}

// sortedKeys returns the keys of a map[string]V sorted alphabetically.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
