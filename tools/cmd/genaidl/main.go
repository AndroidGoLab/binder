package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/xaionaro-go/binder/binder"
	"github.com/xaionaro-go/binder/tools/pkg/codegen"
	"github.com/xaionaro-go/binder/tools/pkg/parser"
	"github.com/xaionaro-go/binder/tools/pkg/resolver"
)

// apiLevelMajorVersion maps Android API levels to the major.minor.patch
// prefix used in AOSP tag names (e.g. "android-16.0.0_r4").
var apiLevelMajorVersion = map[int]string{
	34: "14.0.0",
	35: "15.0.0",
	36: "16.0.0",
}

// submoduleNames lists the 3rdparty submodule directory basenames.
var submoduleNames = []string{
	"frameworks-base",
	"frameworks-native",
	"frameworks-hardware-interfaces",
	"frameworks-av",
	"hardware-interfaces",
	"system-hardware-interfaces",
	"system-netd",
	"system-connectivity-wificond",
	"packages-modules-bluetooth",
}

type searchPathsFlag []string

func (s *searchPathsFlag) String() string {
	return strings.Join(*s, ",")
}

func (s *searchPathsFlag) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func main() {
	outputDir := flag.String("output", ".", "Output directory for generated Go proxy files")
	thirdpartyDir := flag.String("3rdparty", "tools/pkg/3rdparty", "Path to the 3rdparty directory containing AOSP submodules")
	codesOutput := flag.String("codes-output", "", "Output file for version-aware transaction code tables (e.g. binder/versionaware/codes_gen.go)")
	defaultAPI := flag.Int("default-api", 36, "API level that the compiled proxy code was generated against")
	smokeTests := flag.Bool("smoke-tests", false, "Generate smoke tests for all proxy types")
	versions := flag.Bool("versions", false, "Fetch AOSP tags and build multi-version transaction code tables")

	var searchPaths searchPathsFlag
	flag.Var(&searchPaths, "I", "Search path for AIDL imports (can be repeated)")

	flag.Parse()

	positionalFiles := flag.Args()

	if err := run(
		*outputDir,
		*thirdpartyDir,
		*codesOutput,
		*defaultAPI,
		*smokeTests,
		*versions,
		searchPaths,
		positionalFiles,
	); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(
	outputDir string,
	thirdpartyDir string,
	codesOutput string,
	defaultAPI int,
	smokeTests bool,
	fetchVersions bool,
	searchPaths []string,
	positionalFiles []string,
) error {
	// Decide mode: explicit files (aidlgen-style) vs. discovery (aospgen-style).
	if len(positionalFiles) > 0 {
		return runExplicitFiles(outputDir, searchPaths, positionalFiles)
	}

	return runDiscovery(
		outputDir,
		thirdpartyDir,
		codesOutput,
		defaultAPI,
		smokeTests,
		fetchVersions,
	)
}

// runExplicitFiles processes individually specified AIDL files with explicit
// search paths, equivalent to the former aidlgen tool.
func runExplicitFiles(
	outputDir string,
	searchPaths []string,
	files []string,
) error {
	if len(searchPaths) == 0 {
		return fmt.Errorf("no search paths specified; use -I <search-path>")
	}

	r := resolver.New(searchPaths)
	gen := codegen.NewGenerator(r, outputDir)

	for _, f := range files {
		if err := r.ResolveFile(f); err != nil {
			return fmt.Errorf("resolving %s: %w", f, err)
		}
	}

	if err := gen.GenerateAll(); err != nil {
		return fmt.Errorf("generating code: %w", err)
	}

	return nil
}

// runDiscovery discovers AIDL files from the 3rdparty tree and generates
// proxy code, optionally also generating version-aware transaction code
// tables. Equivalent to the former aospgen + genversions tools combined.
func runDiscovery(
	outputDir string,
	thirdpartyDir string,
	codesOutput string,
	defaultAPI int,
	smokeTests bool,
	fetchVersions bool,
) error {
	absThirdparty, err := filepath.Abs(thirdpartyDir)
	if err != nil {
		return fmt.Errorf("resolving 3rdparty path: %w", err)
	}

	if _, err := os.Stat(absThirdparty); os.IsNotExist(err) {
		return fmt.Errorf("3rdparty directory not found: %s", absThirdparty)
	}

	// Generate proxy code from discovered AIDL files.
	if err := generateProxyCode(absThirdparty, outputDir, smokeTests); err != nil {
		return err
	}

	// Generate transaction code tables if requested.
	if codesOutput == "" {
		return nil
	}

	return generateCodeTables(
		absThirdparty,
		codesOutput,
		defaultAPI,
		fetchVersions,
	)
}

// generateProxyCode discovers AIDL files, resolves them, and generates Go
// proxy/stub code (the aospgen workflow).
func generateProxyCode(
	absThirdparty string,
	outputDir string,
	smokeTests bool,
) error {
	fmt.Fprintf(os.Stderr, "Discovering AIDL files in %s...\n", absThirdparty)
	aidlFiles, err := discoverAIDLFiles(absThirdparty)
	if err != nil {
		return fmt.Errorf("discovering AIDL files: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Found %d AIDL files\n", len(aidlFiles))

	fmt.Fprintf(os.Stderr, "Discovering search roots...\n")
	searchRoots, err := discoverSearchRoots(aidlFiles)
	if err != nil {
		return fmt.Errorf("discovering search roots: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Found %d search roots\n", len(searchRoots))

	r := resolver.New(searchRoots)
	r.SetSkipUnresolved(true)

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

	allDefs := r.Registry().All()
	realDefCount := 0
	for _, def := range allDefs {
		if isRealDefinition(def) {
			realDefCount++
		}
	}
	fmt.Fprintf(os.Stderr, "Total definitions: %d (real: %d)\n", len(allDefs), realDefCount)

	fmt.Fprintf(os.Stderr, "Generating Go code into %s...\n", outputDir)
	gen := codegen.NewGenerator(r, outputDir)
	gen.SetSkipErrors(true)
	if err := gen.GenerateAll(); err != nil {
		codegenErrors := strings.Split(err.Error(), "\n")
		fmt.Fprintf(os.Stderr, "Codegen completed with %d definition errors (skipped)\n", len(codegenErrors))
	}

	if smokeTests {
		fmt.Fprintf(os.Stderr, "Generating smoke tests...\n")
		if err := gen.GenerateAllSmokeTests(); err != nil {
			smokeErrors := strings.Split(err.Error(), "\n")
			fmt.Fprintf(os.Stderr, "Smoke test generation completed with %d errors (skipped)\n", len(smokeErrors))
		}
	}

	genCount, err := countGeneratedFiles(outputDir)
	if err != nil {
		return fmt.Errorf("counting generated files: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Generated %d Go files\n", genCount)

	return nil
}

// generateCodeTables builds version-aware transaction code tables, optionally
// fetching multiple AOSP revisions (the genversions workflow).
func generateCodeTables(
	absThirdparty string,
	codesOutput string,
	defaultAPI int,
	fetchVersions bool,
) error {
	allTables := map[string]map[string]map[string]binder.TransactionCode{}
	apiRevisions := map[int][]string{}
	apiLevels := sortedAPILevels()

	// Fetch and process historical AOSP revisions if requested.
	if fetchVersions {
		if err := fetchAOSPVersionTables(
			absThirdparty,
			apiLevels,
			allTables,
			apiRevisions,
		); err != nil {
			return err
		}
	}

	// Always add a local entry from the current 3rdparty state.
	localVersionID := fmt.Sprintf("%d.local", defaultAPI)
	fmt.Fprintf(os.Stderr, "Parsing current 3rdparty state as %s...\n", localVersionID)

	localTable, err := parseVersionTable(absThirdparty)
	if err != nil {
		return fmt.Errorf("parsing local version table: %w", err)
	}

	allTables[localVersionID] = localTable

	// Add local entry to the revisions for the default API level.
	apiRevisions[defaultAPI] = append([]string{localVersionID}, apiRevisions[defaultAPI]...)

	// Ensure the default API level appears in apiLevels for output.
	hasDefaultAPI := false
	for _, level := range apiLevels {
		if level == defaultAPI {
			hasDefaultAPI = true
			break
		}
	}
	if !hasDefaultAPI {
		apiLevels = append(apiLevels, defaultAPI)
		sort.Ints(apiLevels)
	}

	src, err := generateSource(defaultAPI, allTables, apiRevisions, apiLevels)
	if err != nil {
		return fmt.Errorf("generating source: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(codesOutput), 0o755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}
	if err := os.WriteFile(codesOutput, src, 0o644); err != nil {
		return fmt.Errorf("writing output: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Wrote %s (%d bytes)\n", codesOutput, len(src))
	return nil
}

// fetchAOSPVersionTables checks out each AOSP revision tag and builds
// transaction code tables, populating allTables and apiRevisions.
func fetchAOSPVersionTables(
	absThirdparty string,
	apiLevels []int,
	allTables map[string]map[string]map[string]binder.TransactionCode,
	apiRevisions map[int][]string,
) error {
	submoduleDirs := make([]string, len(submoduleNames))
	for i, name := range submoduleNames {
		submoduleDirs[i] = filepath.Join(absThirdparty, name)
	}

	originalCommits, err := saveCurrentCommits(submoduleDirs)
	if err != nil {
		return fmt.Errorf("saving current commits: %w", err)
	}
	defer restoreCommits(submoduleDirs, originalCommits)

	allRevTags, err := discoverRevisionTags(submoduleDirs[0], apiLevels)
	if err != nil {
		return fmt.Errorf("discovering revision tags: %w", err)
	}

	for _, level := range apiLevels {
		tags := allRevTags[level]
		if len(tags) == 0 {
			return fmt.Errorf("no revision tags found for API %d", level)
		}

		sort.Slice(tags, func(i, j int) bool {
			return tags[i].Revision < tags[j].Revision
		})

		var prevTable map[string]map[string]binder.TransactionCode
		var prevVersionID string
		var distinctVersions []string

		for _, rt := range tags {
			fmt.Fprintf(os.Stderr, "Fetching API %d revision r%d (tag %s)...\n", level, rt.Revision, rt.Tag)

			if err := checkoutTag(submoduleDirs, rt.Tag); err != nil {
				return fmt.Errorf("checking out tag %s for API %d r%d: %w", rt.Tag, level, rt.Revision, err)
			}

			table, err := parseVersionTable(absThirdparty)
			if err != nil {
				return fmt.Errorf("parsing API %d r%d: %w", level, rt.Revision, err)
			}

			vid := rt.versionID()

			if prevTable != nil && tablesEqual(prevTable, table) {
				fmt.Fprintf(os.Stderr, "  -> same as %s, skipping\n", prevVersionID)
				continue
			}

			allTables[vid] = table
			distinctVersions = append(distinctVersions, vid)
			prevTable = table
			prevVersionID = vid

			fmt.Fprintf(os.Stderr, "API %d r%d: %d interfaces (distinct)\n", level, rt.Revision, len(table))
		}

		// Store revisions latest-first for probing (most likely match).
		reversed := make([]string, len(distinctVersions))
		for i, v := range distinctVersions {
			reversed[len(distinctVersions)-1-i] = v
		}
		apiRevisions[level] = reversed
	}

	// Restore submodules before returning (the defer also covers panics).
	restoreCommits(submoduleDirs, originalCommits)
	return nil
}

// --- Discovery and parsing functions (from aospgen) ---

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
			return nil // skip inaccessible paths
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

// --- Version tables functions (from genversions) ---

// revisionTag represents a single AOSP revision tag.
type revisionTag struct {
	APILevel int
	Revision int    // e.g., 4 for "android-16.0.0_r4"
	Tag      string // e.g., "android-16.0.0_r4"
}

// versionID returns the version string like "36.r4".
func (r revisionTag) versionID() string {
	return fmt.Sprintf("%d.r%d", r.APILevel, r.Revision)
}

// sortedAPILevels returns the API level keys in ascending order.
func sortedAPILevels() []int {
	levels := make([]int, 0, len(apiLevelMajorVersion))
	for level := range apiLevelMajorVersion {
		levels = append(levels, level)
	}
	sort.Ints(levels)
	return levels
}

// discoverRevisionTags queries git ls-remote for all android-X.Y.Z_rN tags
// for each API level and returns them grouped.
func discoverRevisionTags(
	repoDir string,
	apiLevels []int,
) (map[int][]revisionTag, error) {
	result := make(map[int][]revisionTag, len(apiLevels))
	tagRe := regexp.MustCompile(`refs/tags/(android-[\d.]+_r(\d+))$`)

	for _, level := range apiLevels {
		majorVersion, ok := apiLevelMajorVersion[level]
		if !ok {
			return nil, fmt.Errorf("no major version mapping for API %d", level)
		}

		pattern := fmt.Sprintf("android-%s_r*", majorVersion)
		out, err := exec.Command("git", "-C", repoDir, "ls-remote", "--tags", "origin", pattern).Output()
		if err != nil {
			return nil, fmt.Errorf("ls-remote for API %d: %w", level, err)
		}

		var tags []revisionTag
		for _, line := range strings.Split(string(out), "\n") {
			matches := tagRe.FindStringSubmatch(line)
			if matches == nil {
				continue
			}
			tag := matches[1]
			rev, err := strconv.Atoi(matches[2])
			if err != nil {
				continue
			}
			tags = append(tags, revisionTag{
				APILevel: level,
				Revision: rev,
				Tag:      tag,
			})
		}

		if len(tags) == 0 {
			return nil, fmt.Errorf("no tags found for API %d (pattern %s)", level, pattern)
		}

		result[level] = tags
		fmt.Fprintf(os.Stderr, "API %d: found %d revision tags\n", level, len(tags))
	}

	return result, nil
}

// tablesEqual checks if two version tables have identical content.
func tablesEqual(
	a, b map[string]map[string]binder.TransactionCode,
) bool {
	if len(a) != len(b) {
		return false
	}

	for desc, aMethods := range a {
		bMethods, ok := b[desc]
		if !ok {
			return false
		}
		if len(aMethods) != len(bMethods) {
			return false
		}
		for name, aCode := range aMethods {
			if bMethods[name] != aCode {
				return false
			}
		}
	}
	return true
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
		table[descriptor] = codegen.ComputeTransactionCodes(iface.Methods)

		if len(iface.NestedTypes) > 0 {
			extractInterfaces(descriptor, iface.NestedTypes, table)
		}
	}
}

// --- Source generation (from genversions) ---

// generateSource produces the Go source for codes_gen.go.
func generateSource(
	defaultAPI int,
	allTables map[string]map[string]map[string]binder.TransactionCode,
	apiRevisions map[int][]string,
	apiLevels []int,
) ([]byte, error) {
	var buf bytes.Buffer

	buf.WriteString("// Code generated by genaidl. DO NOT EDIT.\n\n")
	buf.WriteString("package versionaware\n\n")
	buf.WriteString("import \"github.com/xaionaro-go/binder/binder\"\n\n")

	fmt.Fprintf(&buf, "func init() {\n")
	fmt.Fprintf(&buf, "\tDefaultAPILevel = %d\n", defaultAPI)

	versionIDs := sortedKeys(allTables)
	buf.WriteString("\tTables = MultiVersionTable{\n")

	for _, vid := range versionIDs {
		table := allTables[vid]
		fmt.Fprintf(&buf, "\t\t%q: VersionTable{\n", vid)

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

	buf.WriteString("\tRevisions = APIRevisions{\n")
	for _, level := range apiLevels {
		revs := apiRevisions[level]
		if len(revs) == 0 {
			continue
		}
		fmt.Fprintf(&buf, "\t\t%d: {", level)
		for i, rev := range revs {
			if i > 0 {
				buf.WriteString(", ")
			}
			fmt.Fprintf(&buf, "%q", rev)
		}
		buf.WriteString("},\n")
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
