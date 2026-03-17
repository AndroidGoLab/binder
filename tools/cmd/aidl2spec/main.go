package main

import (
	"flag"
	"fmt"
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
	"github.com/xaionaro-go/binder/tools/pkg/spec"
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
	thirdpartyDir := flag.String("3rdparty", "", "Path to the 3rdparty directory containing AOSP submodules")
	outputDir := flag.String("output", "specs/", "Output directory for spec files")
	fetchVersions := flag.Bool("versions", false, "Fetch AOSP tags and embed multi-version transaction codes")
	defaultAPI := flag.Int("default-api", 36, "API level for the local version entry")

	var searchPaths searchPathsFlag
	flag.Var(&searchPaths, "I", "Search path for AIDL imports (can be repeated)")

	flag.Parse()

	positionalFiles := flag.Args()

	if err := run(
		*thirdpartyDir,
		*outputDir,
		*fetchVersions,
		*defaultAPI,
		searchPaths,
		positionalFiles,
	); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(
	thirdpartyDir string,
	outputDir string,
	fetchVersions bool,
	defaultAPI int,
	searchPaths []string,
	positionalFiles []string,
) error {
	if len(positionalFiles) > 0 {
		return runExplicitFiles(outputDir, searchPaths, positionalFiles)
	}

	if thirdpartyDir == "" {
		return fmt.Errorf("-3rdparty flag is required in discovery mode")
	}

	return runDiscovery(
		thirdpartyDir,
		outputDir,
		fetchVersions,
		defaultAPI,
	)
}

func runExplicitFiles(
	outputDir string,
	searchPaths []string,
	files []string,
) error {
	if len(searchPaths) == 0 {
		return fmt.Errorf("no search paths specified; use -I <search-path>")
	}

	r := resolver.New(searchPaths)
	r.SetSkipUnresolved(true)

	for _, f := range files {
		if err := r.ResolveFile(f); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: resolving %s: %v\n", f, err)
			continue
		}
	}

	allDefs := r.Registry.All()
	specs := convertToSpecs(allDefs)

	return spec.WriteAllSpecs(outputDir, specs)
}

func runDiscovery(
	thirdpartyDir string,
	outputDir string,
	fetchVersions bool,
	defaultAPI int,
) error {
	absThirdparty, err := filepath.Abs(thirdpartyDir)
	if err != nil {
		return fmt.Errorf("resolving 3rdparty path: %w", err)
	}

	if _, err := os.Stat(absThirdparty); os.IsNotExist(err) {
		return fmt.Errorf("3rdparty directory not found: %s", absThirdparty)
	}

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

	allDefs := r.Registry.All()
	fmt.Fprintf(os.Stderr, "Total definitions: %d\n", len(allDefs))

	specs := convertToSpecs(allDefs)

	if fetchVersions {
		if err := embedVersionCodes(absThirdparty, defaultAPI, specs); err != nil {
			return fmt.Errorf("embedding version codes: %w", err)
		}
	}

	if err := spec.WriteAllSpecs(outputDir, specs); err != nil {
		return fmt.Errorf("writing specs: %w", err)
	}

	var specCount int
	for range specs {
		specCount++
	}
	fmt.Fprintf(os.Stderr, "Wrote %d spec files to %s\n", specCount, outputDir)

	return nil
}

// convertToSpecs converts the resolver's type registry into spec.PackageSpec
// grouped by Go package.
func convertToSpecs(
	allDefs map[string]parser.Definition,
) map[string]*spec.PackageSpec {
	specs := map[string]*spec.PackageSpec{}

	// Process definitions in sorted order for deterministic output.
	qualifiedNames := make([]string, 0, len(allDefs))
	for qn := range allDefs {
		qualifiedNames = append(qualifiedNames, qn)
	}
	sort.Strings(qualifiedNames)

	for _, qualifiedName := range qualifiedNames {
		def := allDefs[qualifiedName]
		aidlPkg, typeName := splitQualifiedName(qualifiedName)
		if aidlPkg == "" {
			continue
		}

		goPkg := codegen.AIDLToGoPackage(aidlPkg)
		ps, ok := specs[goPkg]
		if !ok {
			ps = &spec.PackageSpec{
				AIDLPackage: aidlPkg,
				GoPackage:   goPkg,
			}
			specs[goPkg] = ps
		}

		convertDefinition(ps, typeName, qualifiedName, def)
	}

	// Sort all slices within each package spec for deterministic output.
	for _, ps := range specs {
		sortPackageSpec(ps)
	}

	return specs
}

// splitQualifiedName splits "android.os.IServiceManager" into
// ("android.os", "IServiceManager"). For nested types like
// "android.os.IServiceManager.InnerType", returns ("android.os", "IServiceManager.InnerType").
func splitQualifiedName(
	qualifiedName string,
) (string, string) {
	parts := strings.Split(qualifiedName, ".")
	if len(parts) < 2 {
		return "", qualifiedName
	}

	// Find the split point: the first segment that starts with an uppercase letter
	// is the type name boundary. Everything before it is the package.
	for i := 1; i < len(parts); i++ {
		if len(parts[i]) > 0 && parts[i][0] >= 'A' && parts[i][0] <= 'Z' {
			return strings.Join(parts[:i], "."), strings.Join(parts[i:], ".")
		}
	}

	// Fallback: last segment is the type name.
	return strings.Join(parts[:len(parts)-1], "."), parts[len(parts)-1]
}

func convertDefinition(
	ps *spec.PackageSpec,
	typeName string,
	qualifiedName string,
	def parser.Definition,
) {
	switch d := def.(type) {
	case *parser.InterfaceDecl:
		iface := convertInterface(typeName, qualifiedName, d)
		ps.Interfaces = append(ps.Interfaces, iface)

	case *parser.ParcelableDecl:
		if !isRealParcelable(d) {
			return
		}
		parc := convertParcelable(typeName, d)
		ps.Parcelables = append(ps.Parcelables, parc)

	case *parser.EnumDecl:
		enum := convertEnum(typeName, d)
		ps.Enums = append(ps.Enums, enum)

	case *parser.UnionDecl:
		union := convertUnion(typeName, d)
		ps.Unions = append(ps.Unions, union)
	}
}

func convertInterface(
	typeName string,
	qualifiedName string,
	iface *parser.InterfaceDecl,
) spec.InterfaceSpec {
	is := spec.InterfaceSpec{
		Name:       typeName,
		Descriptor: qualifiedName,
		Oneway:     iface.Oneway,
	}

	codes := codegen.ComputeTransactionCodes(iface.Methods)

	for _, m := range iface.Methods {
		ms := convertMethod(m, codes)
		is.Methods = append(is.Methods, ms)
	}

	for _, c := range iface.Constants {
		cs := convertConstant(c)
		is.Constants = append(is.Constants, cs)
	}

	return is
}

func convertMethod(
	m *parser.MethodDecl,
	codes map[string]binder.TransactionCode,
) spec.MethodSpec {
	ms := spec.MethodSpec{
		Name:   m.MethodName,
		Oneway: m.Oneway,
	}

	if code, ok := codes[m.MethodName]; ok {
		ms.TransactionCodeOffset = int(code - binder.FirstCallTransaction)
	}

	if m.ReturnType != nil {
		ms.ReturnType = convertTypeRef(m.ReturnType)
	}

	for _, p := range m.Params {
		ps := convertParam(p)
		ms.Params = append(ms.Params, ps)
	}

	ms.Annotations = collectAnnotationNames(m.Annots)

	return ms
}

func convertParam(
	p *parser.ParamDecl,
) spec.ParamSpec {
	ps := spec.ParamSpec{
		Name: p.ParamName,
		Type: convertTypeRef(p.Type),
	}

	switch p.Direction {
	case parser.DirectionIn:
		ps.Direction = spec.DirectionIn
	case parser.DirectionOut:
		ps.Direction = spec.DirectionOut
	case parser.DirectionInOut:
		ps.Direction = spec.DirectionInOut
	}

	ps.Annotations = collectAnnotationNames(p.Annots)

	return ps
}

func convertTypeRef(
	ts *parser.TypeSpecifier,
) spec.TypeRef {
	if ts == nil {
		return spec.TypeRef{}
	}

	tr := spec.TypeRef{
		Name:       ts.Name,
		IsArray:    ts.IsArray,
		FixedSize:  ts.FixedSize,
		IsNullable: hasAnnotation(ts.Annots, "nullable"),
	}

	for _, arg := range ts.TypeArgs {
		tr.TypeArgs = append(tr.TypeArgs, convertTypeRef(arg))
	}

	return tr
}

func convertParcelable(
	typeName string,
	d *parser.ParcelableDecl,
) spec.ParcelableSpec {
	ps := spec.ParcelableSpec{
		Name:        typeName,
		Annotations: collectAnnotationNames(d.Annots),
	}

	for _, f := range d.Fields {
		ps.Fields = append(ps.Fields, convertField(f))
	}

	for _, c := range d.Constants {
		ps.Constants = append(ps.Constants, convertConstant(c))
	}

	// Record nested type names for reference.
	for _, nd := range d.NestedTypes {
		ps.NestedTypes = append(ps.NestedTypes, typeName+"."+nd.GetName())
	}

	return ps
}

func convertField(
	f *parser.FieldDecl,
) spec.FieldSpec {
	fs := spec.FieldSpec{
		Name:        f.FieldName,
		Type:        convertTypeRef(f.Type),
		Annotations: collectAnnotationNames(f.Annots),
	}

	if f.DefaultValue != nil {
		fs.DefaultValue = constExprToString(f.DefaultValue)
	}

	return fs
}

func convertEnum(
	typeName string,
	d *parser.EnumDecl,
) spec.EnumSpec {
	es := spec.EnumSpec{
		Name:        typeName,
		Annotations: collectAnnotationNames(d.Annots),
	}

	if d.BackingType != nil {
		es.BackingType = d.BackingType.Name
	}

	for _, e := range d.Enumerators {
		ev := spec.EnumeratorSpec{
			Name: e.Name,
		}
		if e.Value != nil {
			ev.Value = constExprToString(e.Value)
		}
		es.Values = append(es.Values, ev)
	}

	return es
}

func convertUnion(
	typeName string,
	d *parser.UnionDecl,
) spec.UnionSpec {
	us := spec.UnionSpec{
		Name:        typeName,
		Annotations: collectAnnotationNames(d.Annots),
	}

	for _, f := range d.Fields {
		us.Fields = append(us.Fields, convertField(f))
	}

	return us
}

func convertConstant(
	c *parser.ConstantDecl,
) spec.ConstantSpec {
	cs := spec.ConstantSpec{
		Name: c.ConstName,
	}

	if c.Type != nil {
		cs.Type = c.Type.Name
	}

	if c.Value != nil {
		cs.Value = constExprToString(c.Value)
	}

	return cs
}

// constExprToString renders a ConstExpr back to its string representation.
func constExprToString(
	expr parser.ConstExpr,
) string {
	if expr == nil {
		return ""
	}

	switch e := expr.(type) {
	case *parser.IntegerLiteral:
		return e.Value
	case *parser.FloatLiteral:
		return e.Value
	case *parser.StringLiteralExpr:
		return `"` + e.Value + `"`
	case *parser.CharLiteralExpr:
		return "'" + e.Value + "'"
	case *parser.BoolLiteral:
		if e.Value {
			return "true"
		}
		return "false"
	case *parser.NullLiteral:
		return "null"
	case *parser.IdentExpr:
		return e.Name
	case *parser.UnaryExpr:
		return e.Op.String() + constExprToString(e.Operand)
	case *parser.BinaryExpr:
		return constExprToString(e.Left) + " " + e.Op.String() + " " + constExprToString(e.Right)
	case *parser.TernaryExpr:
		return constExprToString(e.Cond) + " ? " + constExprToString(e.Then) + " : " + constExprToString(e.Else)
	default:
		return fmt.Sprintf("%v", expr)
	}
}

func hasAnnotation(
	annots []*parser.Annotation,
	name string,
) bool {
	for _, a := range annots {
		if a.Name == name {
			return true
		}
	}
	return false
}

func collectAnnotationNames(
	annots []*parser.Annotation,
) []string {
	if len(annots) == 0 {
		return nil
	}

	names := make([]string, 0, len(annots))
	for _, a := range annots {
		names = append(names, a.Name)
	}
	return names
}

// isRealParcelable returns true if the parcelable has actual content
// (fields, constants, or nested types), not just a forward declaration.
func isRealParcelable(
	d *parser.ParcelableDecl,
) bool {
	return len(d.Fields) > 0 || len(d.Constants) > 0 || len(d.NestedTypes) > 0
}

func sortPackageSpec(
	ps *spec.PackageSpec,
) {
	sort.Slice(ps.Interfaces, func(i, j int) bool {
		return ps.Interfaces[i].Name < ps.Interfaces[j].Name
	})
	sort.Slice(ps.Parcelables, func(i, j int) bool {
		return ps.Parcelables[i].Name < ps.Parcelables[j].Name
	})
	sort.Slice(ps.Enums, func(i, j int) bool {
		return ps.Enums[i].Name < ps.Enums[j].Name
	})
	sort.Slice(ps.Unions, func(i, j int) bool {
		return ps.Unions[i].Name < ps.Unions[j].Name
	})
}

// --- Discovery functions (shared with genaidl) ---

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
			continue
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

// --- Version codes embedding ---

func embedVersionCodes(
	absThirdparty string,
	defaultAPI int,
	specs map[string]*spec.PackageSpec,
) error {
	apiLevels := sortedAPILevels()

	allTables := map[string]map[string]map[string]binder.TransactionCode{}
	apiRevisions := map[int][]string{}

	if err := fetchAOSPVersionTables(
		absThirdparty,
		apiLevels,
		allTables,
		apiRevisions,
	); err != nil {
		return err
	}

	// Add a local entry from the current 3rdparty state.
	localVersionID := fmt.Sprintf("%d.local", defaultAPI)
	fmt.Fprintf(os.Stderr, "Parsing current 3rdparty state as %s...\n", localVersionID)

	localTable, err := parseVersionTable(absThirdparty)
	if err != nil {
		return fmt.Errorf("parsing local version table: %w", err)
	}
	allTables[localVersionID] = localTable
	apiRevisions[defaultAPI] = append([]string{localVersionID}, apiRevisions[defaultAPI]...)

	// Embed version codes into interface specs.
	for _, ps := range specs {
		for i := range ps.Interfaces {
			iface := &ps.Interfaces[i]
			versionCodes := buildVersionCodesForInterface(iface.Descriptor, allTables)
			if len(versionCodes) > 0 {
				iface.VersionCodes = versionCodes
			}
		}
	}

	return nil
}

// buildVersionCodesForInterface builds the version_codes map for a single
// interface descriptor from the collected version tables.
func buildVersionCodesForInterface(
	descriptor string,
	allTables map[string]map[string]map[string]binder.TransactionCode,
) map[string]map[string]int {
	result := map[string]map[string]int{}

	versionIDs := sortedKeys(allTables)
	for _, vid := range versionIDs {
		table := allTables[vid]
		methods, ok := table[descriptor]
		if !ok {
			continue
		}

		offsets := map[string]int{}
		for methodName, code := range methods {
			offsets[methodName] = int(code - binder.FirstCallTransaction)
		}
		if len(offsets) > 0 {
			result[vid] = offsets
		}
	}

	return result
}

// --- AOSP tag fetching functions ---

// revisionTag represents a single AOSP revision tag.
type revisionTag struct {
	APILevel int
	Revision int
	Tag      string
}

func (r revisionTag) versionID() string {
	return fmt.Sprintf("%d.r%d", r.APILevel, r.Revision)
}

func sortedAPILevels() []int {
	levels := make([]int, 0, len(apiLevelMajorVersion))
	for level := range apiLevelMajorVersion {
		levels = append(levels, level)
	}
	sort.Ints(levels)
	return levels
}

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

	restoreCommits(submoduleDirs, originalCommits)
	return nil
}

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

// --- Utility ---

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
