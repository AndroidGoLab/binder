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
	"github.com/xaionaro-go/binder/tools/pkg/parser"
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

	// Discover all revision tags for each API level.
	apiLevels := sortedAPILevels()
	allRevTags, err := discoverRevisionTags(submoduleDirs[0], apiLevels)
	if err != nil {
		return fmt.Errorf("discovering revision tags: %w", err)
	}

	// For each API level, iterate revisions in order and deduplicate.
	// allTables maps version ID -> VersionTable.
	allTables := map[string]map[string]map[string]binder.TransactionCode{}
	// apiRevisions maps API level -> ordered list of distinct version IDs (latest first).
	apiRevisions := map[int][]string{}

	for _, level := range apiLevels {
		tags := allRevTags[level]
		if len(tags) == 0 {
			return fmt.Errorf("no revision tags found for API %d", level)
		}

		// Process revisions in ascending order.
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

	// Restore submodules before writing output (the defer also covers panics).
	restoreCommits(submoduleDirs, originalCommits)

	// Generate and write the output file.
	src, err := generateSource(defaultAPI, allTables, apiRevisions, apiLevels)
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
	allTables map[string]map[string]map[string]binder.TransactionCode,
	apiRevisions map[int][]string,
	apiLevels []int,
) ([]byte, error) {
	var buf bytes.Buffer

	buf.WriteString("// Code generated by genversions. DO NOT EDIT.\n\n")
	buf.WriteString("package versionaware\n\n")
	buf.WriteString("import \"github.com/xaionaro-go/binder/binder\"\n\n")

	fmt.Fprintf(&buf, "func init() {\n")
	fmt.Fprintf(&buf, "\tDefaultAPILevel = %d\n", defaultAPI)

	// Emit Tables (sorted by version ID for deterministic output).
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

	// Emit Revisions (sorted by API level for deterministic output).
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
