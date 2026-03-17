// Command spec2cli reads YAML spec files (produced by aidl2spec) and
// generates registry_gen.go and commands_gen.go for the bindercli tool.
// It replaces genbindercli, which scanned Go AST instead of specs.
//
// Usage:
//
//	spec2cli -specs specs/ -output cmd/bindercli/
package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/xaionaro-go/binder/tools/pkg/codegen"
	"github.com/xaionaro-go/binder/tools/pkg/parser"
	"github.com/xaionaro-go/binder/tools/pkg/spec"
)

const (
	modulePath = "github.com/xaionaro-go/binder"
)

// primitiveTypes maps Go type names to cobra flag helpers.
var primitiveTypes = map[string]flagInfo{
	"string":  {FlagMethod: "String", GetMethod: "GetString", ZeroVal: `""`},
	"bool":    {FlagMethod: "Bool", GetMethod: "GetBool", ZeroVal: "false"},
	"int32":   {FlagMethod: "Int32", GetMethod: "GetInt32", ZeroVal: "0"},
	"int64":   {FlagMethod: "Int64", GetMethod: "GetInt64", ZeroVal: "0"},
	"float32": {FlagMethod: "Float32", GetMethod: "GetFloat32", ZeroVal: "0"},
	"float64": {FlagMethod: "Float64", GetMethod: "GetFloat64", ZeroVal: "0"},
	"byte":    {FlagMethod: "Uint8", GetMethod: "GetUint8", ZeroVal: "0"},
	"uint16":  {FlagMethod: "Uint16", GetMethod: "GetUint16", ZeroVal: "0"},
}

// primitiveArrayElemTypes lists element types that can appear in
// comma-separated array flags.
var primitiveArrayElemTypes = map[string]struct{}{
	"int32":   {},
	"int64":   {},
	"bool":    {},
	"float32": {},
	"float64": {},
}

type flagInfo struct {
	FlagMethod string
	GetMethod  string
	ZeroVal    string
}

// interfaceInfo holds metadata for one AIDL interface, derived from specs.
type interfaceInfo struct {
	Descriptor       string
	ProxyConstructor string // e.g. "NewActivityManagerProxy"
	ProxyType        string // e.g. "ActivityManagerProxy"
	ImportPath       string // e.g. "github.com/xaionaro-go/binder/android/app"
	PkgName          string // e.g. "app"
	Methods          []methodInfo
}

type methodInfo struct {
	Name       string
	Params     []paramInfo // excluding ctx
	ReturnType string      // empty if error-only
}

type paramInfo struct {
	Name string
	Type string
}

// structField describes one field inside a known struct type.
type structField struct {
	Name string
	Type string
}

// structInfo holds metadata for a known parcelable struct.
type structInfo struct {
	Fields     []structField
	ImportPath string
	PkgName    string
}

// typeKind classifies a parameter type for code generation.
type typeKind int

const (
	kindUnsupported typeKind = iota
	kindPrimitive
	kindPrimitiveArray    // []byte, []string, []int32, etc.
	kindStruct            // known parcelable struct
	kindEnum              // type Foo int32
	kindBinderIBinder     // binder.IBinder
	kindInterface         // interface{}
	kindInterfaceType     // IFoo or pkg.IFoo (AIDL interface)
	kindNullable          // *T where T is supported
	kindMap               // map[K]V
	kindComplexArray      // []SomeStruct, []interface{}, etc.
	kindNullablePrimitive // *int32, *string, etc.
	kindJSONFallback      // anything else: accept JSON string
)

// knownStructs maps "importPath:TypeName" -> structInfo.
var knownStructs map[string]*structInfo

// knownEnums maps "importPath:TypeName" -> true.
var knownEnums map[string]bool

// knownServiceNames maps AIDL interface descriptors to their well-known
// Android ServiceManager names.
var knownServiceNames map[string]string

// knownInterfaces maps AIDL descriptors to interfaceInfo, used for
// resolving interface types (kindInterfaceType) in other packages.
var knownInterfaces map[string]*interfaceInfo

func main() {
	specsDir := flag.String("specs", "specs/", "Directory containing spec YAML files")
	outputDir := flag.String("output", "cmd/bindercli/", "Output directory for generated files")
	flag.Parse()

	if err := run(*specsDir, *outputDir); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(
	specsDir string,
	outputDir string,
) error {
	fmt.Fprintf(os.Stderr, "Reading specs from %s...\n", specsDir)
	specs, err := spec.ReadAllSpecs(specsDir)
	if err != nil {
		return fmt.Errorf("reading specs: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Read %d package specs\n", len(specs))

	knownStructs = map[string]*structInfo{}
	knownEnums = map[string]bool{}
	knownServiceNames = map[string]string{}
	knownInterfaces = map[string]*interfaceInfo{}

	// Phase 1: collect structs, enums, and service mappings from all specs.
	for _, ps := range specs {
		importPath := modulePath + "/" + ps.GoPackage
		pkgName := filepath.Base(ps.GoPackage)

		collectStructsAndEnums(ps, importPath, pkgName)
		collectServiceMappings(ps)
	}

	fmt.Fprintf(os.Stderr, "Scanned struct types: %d\n", len(knownStructs))
	fmt.Fprintf(os.Stderr, "Scanned enum types: %d\n", len(knownEnums))
	fmt.Fprintf(os.Stderr, "Known service mappings: %d\n", len(knownServiceNames))

	// Phase 2: build interface list.
	var interfaces []*interfaceInfo
	for _, ps := range specs {
		importPath := modulePath + "/" + ps.GoPackage
		pkgName := filepath.Base(ps.GoPackage)

		for _, iface := range ps.Interfaces {
			ii := convertInterfaceSpec(iface, ps.AIDLPackage, importPath, pkgName)
			interfaces = append(interfaces, ii)
			knownInterfaces[ii.Descriptor] = ii
		}
	}

	sort.Slice(interfaces, func(i, j int) bool {
		return interfaces[i].Descriptor < interfaces[j].Descriptor
	})

	totalMethods := 0
	commandableMethods := 0
	for _, iface := range interfaces {
		for _, m := range iface.Methods {
			totalMethods++
			if allParamsSupported(m, iface) {
				commandableMethods++
			}
		}
	}

	pct := float64(0)
	if totalMethods > 0 {
		pct = float64(commandableMethods) / float64(totalMethods) * 100
	}

	fmt.Fprintf(os.Stderr, "Scanned interfaces: %d\n", len(interfaces))
	fmt.Fprintf(os.Stderr, "Total methods: %d\n", totalMethods)
	fmt.Fprintf(os.Stderr, "Generated commands for %d/%d methods (%.1f%%)\n", commandableMethods, totalMethods, pct)

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	if err := writeRegistryGen(outputDir, interfaces); err != nil {
		return fmt.Errorf("writing registry_gen.go: %w", err)
	}

	if err := writeCommandsGen(outputDir, interfaces); err != nil {
		return fmt.Errorf("writing commands_gen.go: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Generated registry_gen.go and commands_gen.go in %s\n", outputDir)
	return nil
}

// collectStructsAndEnums populates knownStructs and knownEnums from a
// package spec's parcelables and enums.
func collectStructsAndEnums(
	ps *spec.PackageSpec,
	importPath string,
	pkgName string,
) {
	for _, parc := range ps.Parcelables {
		goName := codegen.AIDLToGoName(parc.Name)
		key := importPath + ":" + goName
		si := &structInfo{
			ImportPath: importPath,
			PkgName:    pkgName,
		}
		for _, f := range parc.Fields {
			goFieldName := codegen.AIDLToGoName(f.Name)
			goFieldType := typeRefToGoType(f.Type, ps.AIDLPackage)
			si.Fields = append(si.Fields, structField{
				Name: goFieldName,
				Type: goFieldType,
			})
		}
		knownStructs[key] = si
	}

	for _, enum := range ps.Enums {
		goName := codegen.AIDLToGoName(enum.Name)
		key := importPath + ":" + goName
		knownEnums[key] = true
	}

	// Unions are similar to structs from the CLI perspective.
	for _, union := range ps.Unions {
		goName := codegen.AIDLToGoName(union.Name)
		key := importPath + ":" + goName
		si := &structInfo{
			ImportPath: importPath,
			PkgName:    pkgName,
		}
		for _, f := range union.Fields {
			goFieldName := codegen.AIDLToGoName(f.Name)
			goFieldType := typeRefToGoType(f.Type, ps.AIDLPackage)
			si.Fields = append(si.Fields, structField{
				Name: goFieldName,
				Type: goFieldType,
			})
		}
		knownStructs[key] = si
	}
}

// collectServiceMappings populates knownServiceNames from a package spec.
func collectServiceMappings(
	ps *spec.PackageSpec,
) {
	for _, svc := range ps.Services {
		if svc.Descriptor != "" && svc.ServiceName != "" {
			knownServiceNames[svc.Descriptor] = svc.ServiceName
		}
	}
}

// typeRefToGoType converts a spec.TypeRef to a Go type string,
// using the same logic as codegen.AIDLTypeToGo but working from specs.
func typeRefToGoType(
	tr spec.TypeRef,
	currentAIDLPkg string,
) string {
	ts := typeRefToTypeSpecifier(tr)
	goType := codegen.AIDLTypeToGo(ts)

	// If the type is a user-defined type (not primitive, not generic),
	// we need to check if it's a cross-package reference. The AIDL type
	// name in the spec is fully qualified (e.g., "android.app.ProcessMemoryState")
	// for cross-package types, or short (e.g., "ProcessMemoryState") for same-package.
	if goType != "" && !isPrimitiveGoType(goType) {
		// Strip pointer/slice prefixes for analysis.
		bare := goType
		prefix := ""
		for strings.HasPrefix(bare, "*") || strings.HasPrefix(bare, "[]") {
			if strings.HasPrefix(bare, "*") {
				prefix += "*"
				bare = bare[1:]
			} else {
				prefix += "[]"
				bare = bare[2:]
			}
		}

		// If the bare type doesn't contain a dot but the original AIDL name
		// was fully qualified and from a different package, we just use the
		// Go-converted name as-is since the generated Go proxy file will
		// use unqualified names for same-package types.
		_ = prefix
	}

	return goType
}

// typeRefToTypeSpecifier converts a spec.TypeRef to a parser.TypeSpecifier.
func typeRefToTypeSpecifier(
	tr spec.TypeRef,
) *parser.TypeSpecifier {
	ts := &parser.TypeSpecifier{
		Name:      tr.Name,
		IsArray:   tr.IsArray,
		FixedSize: tr.FixedSize,
	}

	if tr.IsNullable {
		ts.Annots = append(ts.Annots, &parser.Annotation{Name: "nullable"})
	}

	for _, arg := range tr.TypeArgs {
		ts.TypeArgs = append(ts.TypeArgs, typeRefToTypeSpecifier(arg))
	}

	return ts
}

// isPrimitiveGoType returns true for Go primitive types and common
// types that don't need import resolution.
func isPrimitiveGoType(
	goType string,
) bool {
	switch goType {
	case "string", "bool", "int32", "int64", "float32", "float64",
		"byte", "uint16", "interface{}", "error", "":
		return true
	}

	if strings.HasPrefix(goType, "[]") {
		return isPrimitiveGoType(goType[2:])
	}
	if strings.HasPrefix(goType, "*") {
		return isPrimitiveGoType(goType[1:])
	}
	if strings.HasPrefix(goType, "map[") {
		return true // generic check; map types are complex anyway
	}

	return false
}

// convertInterfaceSpec converts a spec.InterfaceSpec to an interfaceInfo.
func convertInterfaceSpec(
	iface spec.InterfaceSpec,
	aidlPackage string,
	importPath string,
	pkgName string,
) *interfaceInfo {
	// The Go type name for the interface (e.g., "IActivityManager").
	goName := codegen.AIDLToGoName(iface.Name)
	proxyType := goName[1:] + "Proxy"       // "ActivityManagerProxy"
	proxyConstructor := "New" + proxyType    // "NewActivityManagerProxy"

	ii := &interfaceInfo{
		Descriptor:       iface.Descriptor,
		ProxyConstructor: proxyConstructor,
		ProxyType:        proxyType,
		ImportPath:       importPath,
		PkgName:          pkgName,
	}

	for _, m := range iface.Methods {
		mi := convertMethodSpec(m, aidlPackage)
		ii.Methods = append(ii.Methods, mi)
	}

	return ii
}

// identityParamNames maps AIDL parameter names that represent caller
// identity to their expected AIDL types. These parameters are auto-filled
// by the generated Go proxy and are NOT included in the Go method signature.
var identityParamNames = map[string]string{
	"callingPackage":  "String",
	"opPackageName":   "String",
	"attributionTag":  "String",
	"callingFeatureId": "String",
	"userId":          "int",
	"userHandle":      "int",
	"callingUserId":   "int",
	"callingPid":      "int",
	"appPid":          "int",
	"callingUid":      "int",
	"appUid":          "int",
}

// isIdentityParam returns true if the parameter is an identity parameter
// that should be filtered out of the Go proxy method signature.
func isIdentityParam(
	p spec.ParamSpec,
) bool {
	expectedAIDLType, ok := identityParamNames[p.Name]
	if !ok {
		return false
	}

	// Only match simple (non-array, non-generic) types.
	if p.Type.IsArray || len(p.Type.TypeArgs) > 0 {
		return false
	}

	return p.Type.Name == expectedAIDLType
}

// sanitizeGoIdent ensures Go reserved words don't clash with param names.
func sanitizeGoIdent(
	name string,
) string {
	switch name {
	case "break", "case", "chan", "const", "continue", "default",
		"defer", "else", "fallthrough", "for", "func", "go", "goto",
		"if", "import", "interface", "map", "package", "range",
		"return", "select", "struct", "switch", "type", "var",
		// Pre-declared identifiers.
		"append", "cap", "close", "complex", "copy", "delete",
		"imag", "len", "make", "new", "panic", "print", "println",
		"real", "recover", "error", "bool", "byte", "int", "string",
		"true", "false", "nil", "iota":
		return name + "_"
	}
	return name
}

// convertMethodSpec converts a spec.MethodSpec to a methodInfo.
func convertMethodSpec(
	m spec.MethodSpec,
	aidlPackage string,
) methodInfo {
	mi := methodInfo{
		Name: codegen.AIDLToGoName(m.Name),
	}

	for _, p := range m.Params {
		// Only "in" and "inout" parameters are method arguments in Go.
		// "out" parameters become return values in the generated Go code.
		switch p.Direction {
		case spec.DirectionOut:
			continue
		}

		// Skip identity parameters that are auto-filled by the proxy.
		if isIdentityParam(p) {
			continue
		}

		goType := typeRefToGoType(p.Type, aidlPackage)
		// Use the raw AIDL param name (sanitized for Go keywords) to match
		// the generated Go proxy method signatures. The Go proxy uses
		// lowercase camelCase param names, not PascalCase.
		mi.Params = append(mi.Params, paramInfo{
			Name: sanitizeGoIdent(p.Name),
			Type: goType,
		})
	}

	if m.ReturnType.Name != "" && m.ReturnType.Name != "void" {
		mi.ReturnType = typeRefToGoType(m.ReturnType, aidlPackage)
	}

	return mi
}

// ---- Type classification ----

// resolveTypeKey resolves a Go type string to its knownStructs/knownEnums
// key ("importPath:TypeName") using the interface's import context.
func resolveTypeKey(
	typeStr string,
	ifaceImportPath string,
) string {
	bare := typeStr
	for strings.HasPrefix(bare, "*") || strings.HasPrefix(bare, "[]") {
		if strings.HasPrefix(bare, "*") {
			bare = bare[1:]
		} else {
			bare = bare[2:]
		}
	}

	// If qualified (pkg.Type), we can't easily resolve without a full
	// import map. For spec-based generation, types are typically unqualified.
	if strings.Contains(bare, ".") {
		// binder.IBinder is special-cased elsewhere.
		return bare
	}

	return ifaceImportPath + ":" + bare
}

// classifyType determines how a parameter type should be handled.
func classifyType(
	typeStr string,
	iface *interfaceInfo,
) typeKind {
	if _, ok := primitiveTypes[typeStr]; ok {
		return kindPrimitive
	}

	if strings.HasPrefix(typeStr, "*") {
		inner := typeStr[1:]
		if _, ok := primitiveTypes[inner]; ok {
			return kindNullablePrimitive
		}
		innerKind := classifyType(inner, iface)
		switch innerKind {
		case kindUnsupported:
			return kindJSONFallback
		default:
			return kindNullable
		}
	}

	switch typeStr {
	case "[]byte", "[]string":
		return kindPrimitiveArray
	}
	if strings.HasPrefix(typeStr, "[]") {
		elem := typeStr[2:]
		if _, ok := primitiveArrayElemTypes[elem]; ok {
			return kindPrimitiveArray
		}
	}

	if typeStr == "binder.IBinder" {
		return kindBinderIBinder
	}

	if typeStr == "interface{}" {
		return kindInterface
	}

	if strings.HasPrefix(typeStr, "map[") {
		return kindMap
	}

	if strings.HasPrefix(typeStr, "[]") {
		return kindComplexArray
	}

	// Resolve type using import context.
	key := resolveTypeKey(typeStr, iface.ImportPath)
	if knownEnums[key] {
		return kindEnum
	}
	if knownStructs[key] != nil {
		return kindStruct
	}

	// AIDL interface type.
	bareName := typeStr
	if strings.Contains(typeStr, ".") {
		parts := strings.SplitN(typeStr, ".", 2)
		bareName = parts[1]
	}
	if isAIDLInterfaceName(bareName) {
		return kindInterfaceType
	}

	return kindJSONFallback
}

func isAIDLInterfaceName(
	name string,
) bool {
	if len(name) < 2 {
		return false
	}
	return name[0] == 'I' && name[1] >= 'A' && name[1] <= 'Z'
}

func allParamsSupported(
	m methodInfo,
	iface *interfaceInfo,
) bool {
	for _, p := range m.Params {
		if classifyType(p.Type, iface) == kindUnsupported {
			return false
		}
	}
	return true
}

// classifyFieldType classifies a struct field's type using the struct's
// own import context.
func classifyFieldType(
	typeStr string,
	si *structInfo,
) typeKind {
	if _, ok := primitiveTypes[typeStr]; ok {
		return kindPrimitive
	}

	if strings.HasPrefix(typeStr, "*") {
		inner := typeStr[1:]
		if _, ok := primitiveTypes[inner]; ok {
			return kindNullablePrimitive
		}
		innerKind := classifyFieldType(inner, si)
		switch innerKind {
		case kindUnsupported:
			return kindJSONFallback
		default:
			return kindNullable
		}
	}

	switch typeStr {
	case "[]byte", "[]string":
		return kindPrimitiveArray
	}
	if strings.HasPrefix(typeStr, "[]") {
		elem := typeStr[2:]
		if _, ok := primitiveArrayElemTypes[elem]; ok {
			return kindPrimitiveArray
		}
	}

	if typeStr == "binder.IBinder" {
		return kindBinderIBinder
	}
	if typeStr == "interface{}" {
		return kindInterface
	}
	if strings.HasPrefix(typeStr, "map[") {
		return kindMap
	}
	if strings.HasPrefix(typeStr, "[]") {
		return kindComplexArray
	}

	key := resolveTypeKey(typeStr, si.ImportPath)
	if knownEnums[key] {
		return kindEnum
	}
	if knownStructs[key] != nil {
		return kindStruct
	}

	bareName := typeStr
	if strings.Contains(typeStr, ".") {
		parts := strings.SplitN(typeStr, ".", 2)
		bareName = parts[1]
	}
	if isAIDLInterfaceName(bareName) {
		return kindInterfaceType
	}

	return kindJSONFallback
}

// ---- Import alias resolution for generated code ----

// genContext holds the state needed during code generation for resolving
// type names into the correct import alias in the generated file.
type genContext struct {
	iface         *interfaceInfo
	ifaceAlias    string            // import alias for the interface's package
	importAliases map[string]string // importPath -> alias in generated code
	aliasCounts   map[string]int    // for generating unique aliases
}

// resolveGoType converts a type string into a fully-qualified Go type
// expression using the import aliases in the generated code.
func (gc *genContext) resolveGoType(
	typeStr string,
) string {
	if strings.HasPrefix(typeStr, "*") {
		return "*" + gc.resolveGoType(typeStr[1:])
	}
	if strings.HasPrefix(typeStr, "[]") {
		return "[]" + gc.resolveGoType(typeStr[2:])
	}
	if strings.HasPrefix(typeStr, "map[") {
		depth := 0
		keyEnd := 0
		for i, ch := range typeStr {
			switch ch {
			case '[':
				depth++
			case ']':
				depth--
				if depth == 0 {
					keyEnd = i
				}
			}
			if keyEnd > 0 {
				break
			}
		}
		keyType := typeStr[4:keyEnd]
		valType := typeStr[keyEnd+1:]
		return "map[" + gc.resolveGoType(keyType) + "]" + gc.resolveGoType(valType)
	}

	if _, ok := primitiveTypes[typeStr]; ok {
		return typeStr
	}
	switch typeStr {
	case "interface{}", "error":
		return typeStr
	}

	// Qualified type "pkg.Type".
	if strings.Contains(typeStr, ".") {
		parts := strings.SplitN(typeStr, ".", 2)
		pkgAlias := parts[0]
		typeName := parts[1]

		// "binder.IBinder" is a well-known import.
		if pkgAlias == "binder" {
			return typeStr
		}

		// For other qualified types, we can't easily resolve without
		// a full import map. Return as-is.
		_ = typeName
		return typeStr
	}

	// Unqualified type in the same package as the interface.
	return gc.ifaceAlias + "." + typeStr
}

// resolveFieldGoType resolves a struct field's type to a Go expression
// using the struct's own import context.
func (gc *genContext) resolveFieldGoType(
	typeStr string,
	parentSI *structInfo,
) string {
	if strings.HasPrefix(typeStr, "*") {
		return "*" + gc.resolveFieldGoType(typeStr[1:], parentSI)
	}
	if strings.HasPrefix(typeStr, "[]") {
		return "[]" + gc.resolveFieldGoType(typeStr[2:], parentSI)
	}
	if strings.HasPrefix(typeStr, "map[") {
		depth := 0
		keyEnd := 0
		for i, ch := range typeStr {
			switch ch {
			case '[':
				depth++
			case ']':
				depth--
				if depth == 0 {
					keyEnd = i
				}
			}
			if keyEnd > 0 {
				break
			}
		}
		keyType := typeStr[4:keyEnd]
		valType := typeStr[keyEnd+1:]
		return "map[" + gc.resolveFieldGoType(keyType, parentSI) + "]" + gc.resolveFieldGoType(valType, parentSI)
	}

	if _, ok := primitiveTypes[typeStr]; ok {
		return typeStr
	}
	switch typeStr {
	case "interface{}", "error":
		return typeStr
	}

	if strings.Contains(typeStr, ".") {
		parts := strings.SplitN(typeStr, ".", 2)
		pkgAlias := parts[0]
		if pkgAlias == "binder" {
			return typeStr
		}
		return typeStr
	}

	// Unqualified: type is in the struct's own package.
	if parentSI.ImportPath == modulePath {
		return typeStr
	}
	alias := gc.ensureImport(parentSI.ImportPath)
	return alias + "." + typeStr
}

// ensureImport makes sure an import path has an alias in the generated
// code and returns that alias.
func (gc *genContext) ensureImport(
	importPath string,
) string {
	if importPath == "github.com/xaionaro-go/binder/binder" {
		return "binder"
	}
	if alias, ok := gc.importAliases[importPath]; ok {
		return alias
	}
	pkgName := filepath.Base(importPath)
	alias := uniqueAlias(pkgName, gc.aliasCounts)
	gc.importAliases[importPath] = alias
	return alias
}

// ---- Needs tracking for imports ----

type needsTracker struct {
	hex     bool
	json    bool
	strconv bool
	strings bool
}

func (n *needsTracker) methodNeeds(
	m methodInfo,
	iface *interfaceInfo,
) {
	for _, p := range m.Params {
		n.paramNeeds(p.Type, iface)
	}
}

func (n *needsTracker) paramNeeds(
	typeStr string,
	iface *interfaceInfo,
) {
	kind := classifyType(typeStr, iface)
	switch kind {
	case kindPrimitiveArray:
		switch typeStr {
		case "[]byte":
			n.hex = true
		case "[]string":
			n.strings = true
		default:
			n.strings = true
			n.strconv = true
		}
	case kindStruct:
		key := resolveTypeKey(typeStr, iface.ImportPath)
		si := knownStructs[key]
		if si != nil {
			n.structFieldsNeeds(si, 0)
		}
	case kindEnum:
		// enum uses GetInt32 then cast -- no extra imports
	case kindInterface, kindMap, kindComplexArray, kindJSONFallback:
		n.json = true
	case kindNullable:
		inner := typeStr[1:]
		n.paramNeeds(inner, iface)
	case kindNullablePrimitive:
		inner := typeStr[1:]
		switch inner {
		case "string":
			// no extra
		default:
			n.strconv = true
		}
	case kindInterfaceType:
		// uses GetString + conn.GetService -- no extra
	case kindBinderIBinder:
		// uses GetString + conn.GetService -- no extra
	}
}

func (n *needsTracker) structFieldsNeeds(
	si *structInfo,
	depth int,
) {
	if depth > 3 {
		n.json = true
		return
	}
	for _, f := range si.Fields {
		n.fieldNeeds(f, si, depth)
	}
}

func (n *needsTracker) fieldNeeds(
	f structField,
	parentSI *structInfo,
	depth int,
) {
	fKind := classifyFieldType(f.Type, parentSI)
	switch fKind {
	case kindPrimitiveArray:
		switch f.Type {
		case "[]byte":
			n.hex = true
		case "[]string":
			n.strings = true
		default:
			n.strings = true
			n.strconv = true
		}
	case kindStruct:
		key := resolveTypeKey(f.Type, parentSI.ImportPath)
		innerSI := knownStructs[key]
		if innerSI != nil {
			n.structFieldsNeeds(innerSI, depth+1)
		} else {
			n.json = true
		}
	case kindEnum:
		n.strconv = true
	case kindInterface, kindMap, kindComplexArray, kindJSONFallback,
		kindBinderIBinder, kindInterfaceType, kindNullable, kindNullablePrimitive:
		n.json = true
	}
}

// ---- Generation: registry_gen.go ----

func writeRegistryGen(
	outDir string,
	interfaces []*interfaceInfo,
) error {
	var buf bytes.Buffer
	buf.WriteString("// Code generated by spec2cli. DO NOT EDIT.\n\n")
	buf.WriteString("package main\n\n")
	buf.WriteString("func init() {\n")
	buf.WriteString("\tgeneratedRegistry = &Registry{\n")
	buf.WriteString("\t\tServices: map[string]*ServiceInfo{\n")

	for _, iface := range interfaces {
		fmt.Fprintf(&buf, "\t\t\t%q: {\n", iface.Descriptor)
		fmt.Fprintf(&buf, "\t\t\t\tDescriptor: %q,\n", iface.Descriptor)
		buf.WriteString("\t\t\t\tMethods: []MethodInfo{\n")
		for _, m := range iface.Methods {
			fmt.Fprintf(&buf, "\t\t\t\t\t{Name: %q", m.Name)
			if len(m.Params) > 0 {
				buf.WriteString(", Params: []ParamInfo{")
				for i, p := range m.Params {
					if i > 0 {
						buf.WriteString(", ")
					}
					fmt.Fprintf(&buf, "{Name: %q, Type: %q}", p.Name, p.Type)
				}
				buf.WriteString("}")
			}
			if m.ReturnType != "" {
				fmt.Fprintf(&buf, ", ReturnType: %q", m.ReturnType)
			}
			buf.WriteString("},\n")
		}
		buf.WriteString("\t\t\t\t},\n")
		buf.WriteString("\t\t\t},\n")
	}

	buf.WriteString("\t\t},\n")
	buf.WriteString("\t}\n")
	buf.WriteString("}\n")

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return fmt.Errorf("formatting registry_gen.go: %w\n\nsource:\n%s", err, buf.String())
	}

	return os.WriteFile(filepath.Join(outDir, "registry_gen.go"), formatted, 0o644)
}

// ---- Generation: commands_gen.go ----

type commandableIface struct {
	iface          *interfaceInfo
	importAlias    string
	commandMethods []methodInfo
}

func writeCommandsGen(
	outDir string,
	interfaces []*interfaceInfo,
) error {
	importAliases := map[string]string{}

	aliasCounts := map[string]int{
		"context": 1,
		"fmt":     1,
		"os":      1,
		"cobra":   1,
		"binder":  1,
		"main":    1,
		"hex":     1,
		"json":    1,
		"strconv": 1,
		"strings": 1,
	}

	var commandables []commandableIface
	var needs needsTracker

	for _, iface := range interfaces {
		if iface.ImportPath == modulePath {
			continue
		}

		var cmdMethods []methodInfo
		for _, m := range iface.Methods {
			if allParamsSupported(m, iface) {
				cmdMethods = append(cmdMethods, m)
				needs.methodNeeds(m, iface)
			}
		}
		if len(cmdMethods) == 0 {
			continue
		}

		alias, ok := importAliases[iface.ImportPath]
		if !ok {
			alias = uniqueAlias(iface.PkgName, aliasCounts)
			importAliases[iface.ImportPath] = alias
		}

		commandables = append(commandables, commandableIface{
			iface:          iface,
			importAlias:    alias,
			commandMethods: cmdMethods,
		})
	}

	// Generate the code body first so that ensureImport populates
	// importAliases lazily (only imports that are actually used).
	var body bytes.Buffer
	writeAddGeneratedCommands(&body, commandables)
	writeKnownServiceNamesMap(&body)
	writeFindServiceByDescriptorFunc(&body)

	for _, ci := range commandables {
		gc := &genContext{
			iface:         ci.iface,
			ifaceAlias:    ci.importAlias,
			importAliases: importAliases,
			aliasCounts:   aliasCounts,
		}
		writeInterfaceGroup(&body, ci, gc)
	}

	// Now write the full file: header + imports + body.
	var buf bytes.Buffer
	buf.WriteString("// Code generated by spec2cli. DO NOT EDIT.\n\n")
	buf.WriteString("//go:build linux\n\n")
	buf.WriteString("package main\n\n")

	writeImports(&buf, importAliases, needs)
	buf.Write(body.Bytes())

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return fmt.Errorf("formatting commands_gen.go: %w\n\nsource:\n%s", err, buf.String())
	}

	return os.WriteFile(filepath.Join(outDir, "commands_gen.go"), formatted, 0o644)
}

func writeImports(
	buf *bytes.Buffer,
	importAliases map[string]string,
	needs needsTracker,
) {
	buf.WriteString("import (\n")
	buf.WriteString("\t\"context\"\n")
	if needs.hex {
		buf.WriteString("\t\"encoding/hex\"\n")
	}
	if needs.json {
		buf.WriteString("\t\"encoding/json\"\n")
	}
	buf.WriteString("\t\"fmt\"\n")
	buf.WriteString("\t\"os\"\n")
	if needs.strconv {
		buf.WriteString("\t\"strconv\"\n")
	}
	if needs.strings {
		buf.WriteString("\t\"strings\"\n")
	}
	buf.WriteString("\n")
	buf.WriteString("\t\"github.com/spf13/cobra\"\n")
	buf.WriteString("\t\"github.com/xaionaro-go/binder/binder\"\n")
	buf.WriteString("\t\"github.com/xaionaro-go/binder/servicemanager\"\n")

	type importEntry struct {
		path  string
		alias string
	}
	var imports []importEntry
	for path, alias := range importAliases {
		if path == "github.com/xaionaro-go/binder/binder" || path == modulePath {
			continue
		}
		imports = append(imports, importEntry{path: path, alias: alias})
	}
	sort.Slice(imports, func(i, j int) bool {
		return imports[i].path < imports[j].path
	})

	if len(imports) > 0 {
		buf.WriteString("\n")
	}
	for _, imp := range imports {
		pkgBase := filepath.Base(imp.path)
		if imp.alias == pkgBase {
			fmt.Fprintf(buf, "\t%q\n", imp.path)
		} else {
			fmt.Fprintf(buf, "\t%s %q\n", imp.alias, imp.path)
		}
	}
	buf.WriteString(")\n\n")
}

func writeAddGeneratedCommands(
	buf *bytes.Buffer,
	commandables []commandableIface,
) {
	buf.WriteString("func addGeneratedCommands(root *cobra.Command) {\n")
	for _, ci := range commandables {
		fmt.Fprintf(buf, "\troot.AddCommand(%s())\n", cmdGroupFuncName(ci.iface))
	}
	buf.WriteString("}\n\n")
}

func writeKnownServiceNamesMap(
	buf *bytes.Buffer,
) {
	keys := make([]string, 0, len(knownServiceNames))
	for k := range knownServiceNames {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	buf.WriteString("// knownServiceNames maps AIDL descriptors to well-known Android\n")
	buf.WriteString("// ServiceManager names, allowing fast lookup without enumeration.\n")
	buf.WriteString("var knownServiceNames = map[string]string{\n")
	for _, k := range keys {
		fmt.Fprintf(buf, "\t%q: %q,\n", k, knownServiceNames[k])
	}
	buf.WriteString("}\n\n")
}

func writeFindServiceByDescriptorFunc(
	buf *bytes.Buffer,
) {
	buf.WriteString(`func findServiceByDescriptor(
	ctx context.Context,
	conn *Conn,
	descriptor string,
) (binder.IBinder, error) {
	// Try the static map of well-known service names first to avoid
	// slow enumeration of all registered services.
	if name, ok := knownServiceNames[descriptor]; ok {
		svc, err := conn.SM.CheckService(ctx, servicemanager.ServiceName(name))
		if err == nil && svc != nil {
			return svc, nil
		}
	}

	// Fall back to enumeration.
	services, err := conn.SM.ListServices(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing services: %w", err)
	}

	for _, name := range services {
		svc, err := conn.SM.CheckService(ctx, name)
		if err != nil || svc == nil {
			continue
		}
		desc := queryDescriptor(ctx, svc)
		if desc == descriptor {
			return svc, nil
		}
	}

	return nil, fmt.Errorf("no service with descriptor %q found", descriptor)
}

`)
}

func writeInterfaceGroup(
	buf *bytes.Buffer,
	ci commandableIface,
	gc *genContext,
) {
	iface := ci.iface
	groupFunc := cmdGroupFuncName(iface)

	fmt.Fprintf(buf, "func %s() *cobra.Command {\n", groupFunc)
	fmt.Fprintf(buf, "\tcmd := &cobra.Command{\n")
	fmt.Fprintf(buf, "\t\tUse:   %q,\n", iface.Descriptor)
	fmt.Fprintf(buf, "\t\tShort: %q,\n", iface.Descriptor+" methods")
	fmt.Fprintf(buf, "\t}\n\n")

	for _, m := range ci.commandMethods {
		fmt.Fprintf(buf, "\tcmd.AddCommand(%s())\n", cmdMethodFuncName(iface, m))
	}

	buf.WriteString("\n\treturn cmd\n}\n\n")

	for _, m := range ci.commandMethods {
		writeMethodCmd(buf, iface, ci.importAlias, m, gc)
	}
}

func writeMethodCmd(
	buf *bytes.Buffer,
	iface *interfaceInfo,
	alias string,
	m methodInfo,
	gc *genContext,
) {
	funcName := cmdMethodFuncName(iface, m)
	kebabName := camelToKebab(m.Name)
	shortDesc := buildShortDesc(m)

	qualifier := alias

	fmt.Fprintf(buf, "func %s() *cobra.Command {\n", funcName)

	buf.WriteString("\tcmd := &cobra.Command{\n")
	fmt.Fprintf(buf, "\t\tUse:   %q,\n", kebabName)
	fmt.Fprintf(buf, "\t\tShort: %q,\n", shortDesc)

	buf.WriteString("\t\tRunE: func(cmd *cobra.Command, args []string) error {\n")
	buf.WriteString("\t\t\tctx := context.Background()\n\n")
	buf.WriteString("\t\t\tconn, err := OpenConn(ctx, cmd)\n")
	buf.WriteString("\t\t\tif err != nil {\n")
	buf.WriteString("\t\t\t\treturn err\n")
	buf.WriteString("\t\t\t}\n")
	buf.WriteString("\t\t\tdefer conn.Close(ctx)\n\n")

	buf.WriteString("\t\t\tserviceName, _ := cmd.Flags().GetString(\"service-name\")\n")
	buf.WriteString("\t\t\tvar svc binder.IBinder\n")
	buf.WriteString("\t\t\tif serviceName != \"\" {\n")
	buf.WriteString("\t\t\t\tsvc, err = conn.GetService(ctx, serviceName)\n")
	buf.WriteString("\t\t\t} else {\n")
	fmt.Fprintf(buf, "\t\t\t\tsvc, err = findServiceByDescriptor(ctx, conn, %q)\n", iface.Descriptor)
	buf.WriteString("\t\t\t}\n")
	buf.WriteString("\t\t\tif err != nil {\n")
	buf.WriteString("\t\t\t\treturn err\n")
	buf.WriteString("\t\t\t}\n\n")

	fmt.Fprintf(buf, "\t\t\tsvcProxy := %s.%s(svc)\n\n", qualifier, iface.ProxyConstructor)

	for _, p := range m.Params {
		writeParamExtraction(buf, p, gc)
	}

	if m.ReturnType != "" {
		fmt.Fprintf(buf, "\t\t\tresult, err := svcProxy.%s(ctx", m.Name)
	} else {
		fmt.Fprintf(buf, "\t\t\terr = svcProxy.%s(ctx", m.Name)
	}
	for _, p := range m.Params {
		fmt.Fprintf(buf, ", %s", paramVarName(p))
	}
	buf.WriteString(")\n")
	buf.WriteString("\t\t\tif err != nil {\n")
	buf.WriteString("\t\t\t\treturn err\n")
	buf.WriteString("\t\t\t}\n\n")

	buf.WriteString("\t\t\tmode, _ := cmd.Root().PersistentFlags().GetString(\"format\")\n")
	buf.WriteString("\t\t\tf := NewFormatter(mode, os.Stdout)\n")
	if m.ReturnType != "" {
		buf.WriteString("\t\t\tf.Value(\"result\", result)\n")
	} else {
		buf.WriteString("\t\t\tf.Value(\"status\", \"ok\")\n")
	}

	buf.WriteString("\t\t\treturn nil\n")
	buf.WriteString("\t\t},\n")
	buf.WriteString("\t}\n\n")

	buf.WriteString("\tcmd.Flags().String(\"service-name\", \"\", \"ServiceManager name to use instead of descriptor discovery\")\n")

	for _, p := range m.Params {
		writeParamFlag(buf, p, gc)
	}

	buf.WriteString("\n\treturn cmd\n")
	buf.WriteString("}\n\n")
}

// ---- Per-param flag registration ----

func writeParamFlag(
	buf *bytes.Buffer,
	p paramInfo,
	gc *genContext,
) {
	kind := classifyType(p.Type, gc.iface)
	flagName := p.Name

	switch kind {
	case kindPrimitive:
		fi := primitiveTypes[p.Type]
		fmt.Fprintf(buf, "\tcmd.Flags().%s(%q, %s, %q)\n",
			fi.FlagMethod, flagName, fi.ZeroVal, p.Name+" ("+p.Type+")")
		fmt.Fprintf(buf, "\t_ = cmd.MarkFlagRequired(%q)\n", flagName)

	case kindPrimitiveArray:
		desc := p.Name
		switch p.Type {
		case "[]byte":
			desc += " (hex bytes)"
		case "[]string":
			desc += " (comma-separated)"
		default:
			desc += " (comma-separated " + p.Type[2:] + ")"
		}
		fmt.Fprintf(buf, "\tcmd.Flags().String(%q, \"\", %q)\n", flagName, desc)
		fmt.Fprintf(buf, "\t_ = cmd.MarkFlagRequired(%q)\n", flagName)

	case kindStruct:
		key := resolveTypeKey(p.Type, gc.iface.ImportPath)
		si := knownStructs[key]
		if si != nil {
			writeStructFlags(buf, flagName, si, gc, 0)
		} else {
			fmt.Fprintf(buf, "\tcmd.Flags().String(%q, \"\", %q)\n", flagName, p.Name+" (JSON)")
			fmt.Fprintf(buf, "\t_ = cmd.MarkFlagRequired(%q)\n", flagName)
		}

	case kindEnum:
		fmt.Fprintf(buf, "\tcmd.Flags().Int32(%q, 0, %q)\n", flagName, p.Name+" ("+p.Type+" enum)")
		fmt.Fprintf(buf, "\t_ = cmd.MarkFlagRequired(%q)\n", flagName)

	case kindBinderIBinder:
		fmt.Fprintf(buf, "\tcmd.Flags().String(%q, \"\", %q)\n", flagName, p.Name+" (service name)")
		fmt.Fprintf(buf, "\t_ = cmd.MarkFlagRequired(%q)\n", flagName)

	case kindInterface:
		fmt.Fprintf(buf, "\tcmd.Flags().String(%q, \"\", %q)\n", flagName, p.Name+" (JSON)")
		fmt.Fprintf(buf, "\t_ = cmd.MarkFlagRequired(%q)\n", flagName)

	case kindInterfaceType:
		fmt.Fprintf(buf, "\tcmd.Flags().String(%q, \"\", %q)\n", flagName, p.Name+" (service name for "+p.Type+")")
		fmt.Fprintf(buf, "\t_ = cmd.MarkFlagRequired(%q)\n", flagName)

	case kindNullable:
		inner := p.Type[1:]
		innerKind := classifyType(inner, gc.iface)
		switch innerKind {
		case kindStruct:
			key := resolveTypeKey(inner, gc.iface.ImportPath)
			si := knownStructs[key]
			if si != nil {
				writeStructFlags(buf, flagName, si, gc, 0)
			} else {
				fmt.Fprintf(buf, "\tcmd.Flags().String(%q, \"\", %q)\n", flagName, p.Name+" (JSON, optional)")
			}
		default:
			fmt.Fprintf(buf, "\tcmd.Flags().String(%q, \"\", %q)\n", flagName, p.Name+" (optional, JSON)")
		}

	case kindNullablePrimitive:
		inner := p.Type[1:]
		fmt.Fprintf(buf, "\tcmd.Flags().String(%q, \"\", %q)\n", flagName, p.Name+" (optional "+inner+")")

	case kindMap, kindComplexArray, kindJSONFallback:
		fmt.Fprintf(buf, "\tcmd.Flags().String(%q, \"\", %q)\n", flagName, p.Name+" (JSON "+p.Type+")")
		fmt.Fprintf(buf, "\t_ = cmd.MarkFlagRequired(%q)\n", flagName)
	}
}

func writeStructFlags(
	buf *bytes.Buffer,
	prefix string,
	si *structInfo,
	gc *genContext,
	depth int,
) {
	if depth > 3 {
		fmt.Fprintf(buf, "\tcmd.Flags().String(%q, \"\", %q)\n", prefix, prefix+" (JSON)")
		return
	}

	for _, f := range si.Fields {
		flagName := prefix + "-" + camelToKebab(f.Name)
		fKind := classifyFieldType(f.Type, si)

		switch fKind {
		case kindPrimitive:
			fi := primitiveTypes[f.Type]
			fmt.Fprintf(buf, "\tcmd.Flags().%s(%q, %s, %q)\n",
				fi.FlagMethod, flagName, fi.ZeroVal, prefix+"."+f.Name+" ("+f.Type+")")

		case kindPrimitiveArray:
			desc := prefix + "." + f.Name
			switch f.Type {
			case "[]byte":
				desc += " (hex bytes)"
			case "[]string":
				desc += " (comma-separated)"
			default:
				desc += " (comma-separated " + f.Type[2:] + ")"
			}
			fmt.Fprintf(buf, "\tcmd.Flags().String(%q, \"\", %q)\n", flagName, desc)

		case kindEnum:
			fmt.Fprintf(buf, "\tcmd.Flags().Int32(%q, 0, %q)\n", flagName, prefix+"."+f.Name+" (enum)")

		case kindStruct:
			key := resolveTypeKey(f.Type, si.ImportPath)
			innerSI := knownStructs[key]
			if innerSI != nil && depth < 3 {
				writeStructFlags(buf, flagName, innerSI, gc, depth+1)
			} else {
				fmt.Fprintf(buf, "\tcmd.Flags().String(%q, \"\", %q)\n", flagName, flagName+" (JSON)")
			}

		default:
			fmt.Fprintf(buf, "\tcmd.Flags().String(%q, \"\", %q)\n", flagName, prefix+"."+f.Name+" (JSON)")
		}
	}
}

// ---- Per-param value extraction ----

func writeParamExtraction(
	buf *bytes.Buffer,
	p paramInfo,
	gc *genContext,
) {
	kind := classifyType(p.Type, gc.iface)
	flagName := p.Name
	varName := paramVarName(p)

	switch kind {
	case kindPrimitive:
		fi := primitiveTypes[p.Type]
		fmt.Fprintf(buf, "\t\t\t%s, err := cmd.Flags().%s(%q)\n", varName, fi.GetMethod, flagName)
		buf.WriteString("\t\t\tif err != nil {\n")
		fmt.Fprintf(buf, "\t\t\t\treturn fmt.Errorf(\"reading flag %s: %%w\", err)\n", flagName)
		buf.WriteString("\t\t\t}\n\n")

	case kindPrimitiveArray:
		writePrimitiveArrayExtraction(buf, p.Type, flagName, varName)

	case kindStruct:
		writeStructExtraction(buf, p.Type, flagName, varName, gc, 0)

	case kindEnum:
		writeEnumExtraction(buf, p.Type, flagName, varName, gc)

	case kindBinderIBinder:
		writeBinderExtraction(buf, flagName, varName)

	case kindInterface:
		writeInterfaceExtraction(buf, flagName, varName)

	case kindInterfaceType:
		writeInterfaceTypeExtraction(buf, p.Type, flagName, varName, gc)

	case kindNullable:
		writeNullableExtraction(buf, p.Type, flagName, varName, gc)

	case kindNullablePrimitive:
		writeNullablePrimitiveExtraction(buf, p.Type, flagName, varName)

	case kindMap, kindComplexArray, kindJSONFallback:
		writeJSONFallbackExtraction(buf, p.Type, flagName, varName, gc)
	}
}

func writePrimitiveArrayExtraction(
	buf *bytes.Buffer,
	typeStr string,
	flagName string,
	varName string,
) {
	switch typeStr {
	case "[]byte":
		fmt.Fprintf(buf, "\t\t\t%sHex, err := cmd.Flags().GetString(%q)\n", varName, flagName)
		buf.WriteString("\t\t\tif err != nil {\n")
		fmt.Fprintf(buf, "\t\t\t\treturn fmt.Errorf(\"reading flag %s: %%w\", err)\n", flagName)
		buf.WriteString("\t\t\t}\n")
		fmt.Fprintf(buf, "\t\t\t%s, err := hex.DecodeString(%sHex)\n", varName, varName)
		buf.WriteString("\t\t\tif err != nil {\n")
		fmt.Fprintf(buf, "\t\t\t\treturn fmt.Errorf(\"invalid hex for %s: %%w\", err)\n", flagName)
		buf.WriteString("\t\t\t}\n\n")

	case "[]string":
		fmt.Fprintf(buf, "\t\t\t%sStr, err := cmd.Flags().GetString(%q)\n", varName, flagName)
		buf.WriteString("\t\t\tif err != nil {\n")
		fmt.Fprintf(buf, "\t\t\t\treturn fmt.Errorf(\"reading flag %s: %%w\", err)\n", flagName)
		buf.WriteString("\t\t\t}\n")
		fmt.Fprintf(buf, "\t\t\tvar %s []string\n", varName)
		fmt.Fprintf(buf, "\t\t\tif %sStr != \"\" {\n", varName)
		fmt.Fprintf(buf, "\t\t\t\t%s = strings.Split(%sStr, \",\")\n", varName, varName)
		buf.WriteString("\t\t\t}\n\n")

	default:
		elemType := typeStr[2:]
		parseFunc, bitSize := elemParseInfo(elemType)

		fmt.Fprintf(buf, "\t\t\t%sStr, err := cmd.Flags().GetString(%q)\n", varName, flagName)
		buf.WriteString("\t\t\tif err != nil {\n")
		fmt.Fprintf(buf, "\t\t\t\treturn fmt.Errorf(\"reading flag %s: %%w\", err)\n", flagName)
		buf.WriteString("\t\t\t}\n")
		fmt.Fprintf(buf, "\t\t\tvar %s %s\n", varName, typeStr)
		fmt.Fprintf(buf, "\t\t\tif %sStr != \"\" {\n", varName)
		fmt.Fprintf(buf, "\t\t\t\tfor _, _s := range strings.Split(%sStr, \",\") {\n", varName)

		switch elemType {
		case "bool":
			buf.WriteString("\t\t\t\t\t_v, _err := strconv.ParseBool(strings.TrimSpace(_s))\n")
		case "float32", "float64":
			fmt.Fprintf(buf, "\t\t\t\t\t_v, _err := strconv.%s(strings.TrimSpace(_s), %d)\n", parseFunc, bitSize)
		default:
			fmt.Fprintf(buf, "\t\t\t\t\t_v, _err := strconv.%s(strings.TrimSpace(_s), 0, %d)\n", parseFunc, bitSize)
		}

		buf.WriteString("\t\t\t\t\tif _err != nil {\n")
		fmt.Fprintf(buf, "\t\t\t\t\t\treturn fmt.Errorf(\"invalid %s in %s: %%w\", _err)\n", elemType, flagName)
		buf.WriteString("\t\t\t\t\t}\n")
		fmt.Fprintf(buf, "\t\t\t\t\t%s = append(%s, %s(_v))\n", varName, varName, elemType)
		buf.WriteString("\t\t\t\t}\n")
		buf.WriteString("\t\t\t}\n\n")
	}
}

func elemParseInfo(
	elemType string,
) (string, int) {
	switch elemType {
	case "int32":
		return "ParseInt", 32
	case "int64":
		return "ParseInt", 64
	case "float32":
		return "ParseFloat", 32
	case "float64":
		return "ParseFloat", 64
	case "bool":
		return "ParseBool", 0
	default:
		return "ParseInt", 64
	}
}

func writeStructExtraction(
	buf *bytes.Buffer,
	typeStr string,
	prefix string,
	varName string,
	gc *genContext,
	depth int,
) {
	key := resolveTypeKey(typeStr, gc.iface.ImportPath)
	si := knownStructs[key]
	if si == nil || depth > 3 {
		writeJSONFallbackExtraction(buf, typeStr, prefix, varName, gc)
		return
	}

	qualifiedType := gc.resolveGoType(typeStr)
	fmt.Fprintf(buf, "\t\t\tvar %s %s\n", varName, qualifiedType)

	for _, f := range si.Fields {
		flagName := prefix + "-" + camelToKebab(f.Name)
		fieldAccess := varName + "." + f.Name
		writeStructFieldExtraction(buf, f, flagName, fieldAccess, si, gc, depth)
	}
	buf.WriteString("\n")
}

func writeStructFieldExtraction(
	buf *bytes.Buffer,
	f structField,
	flagName string,
	fieldAccess string,
	parentSI *structInfo,
	gc *genContext,
	depth int,
) {
	fKind := classifyFieldType(f.Type, parentSI)

	switch fKind {
	case kindPrimitive:
		fi := primitiveTypes[f.Type]
		fmt.Fprintf(buf, "\t\t\t%s, _ = cmd.Flags().%s(%q)\n", fieldAccess, fi.GetMethod, flagName)

	case kindPrimitiveArray:
		writeStructFieldPrimitiveArray(buf, f.Type, flagName, fieldAccess)

	case kindEnum:
		tmpVar := "_" + sanitizeVarName(strings.ReplaceAll(flagName, "-", "_"))
		fmt.Fprintf(buf, "\t\t\t%s, _ := cmd.Flags().GetInt32(%q)\n", tmpVar, flagName)
		qualType := gc.resolveFieldGoType(f.Type, parentSI)
		fmt.Fprintf(buf, "\t\t\t%s = %s(%s)\n", fieldAccess, qualType, tmpVar)

	case kindStruct:
		key := resolveTypeKey(f.Type, parentSI.ImportPath)
		innerSI := knownStructs[key]
		if innerSI != nil && depth < 3 {
			for _, innerF := range innerSI.Fields {
				innerFlagName := flagName + "-" + camelToKebab(innerF.Name)
				innerFieldAccess := fieldAccess + "." + innerF.Name
				writeStructFieldExtraction(buf, innerF, innerFlagName, innerFieldAccess, innerSI, gc, depth+1)
			}
		} else {
			writeStructFieldJSON(buf, flagName, fieldAccess)
		}

	default:
		writeStructFieldJSON(buf, flagName, fieldAccess)
	}
}

func writeStructFieldPrimitiveArray(
	buf *bytes.Buffer,
	typeStr string,
	flagName string,
	fieldAccess string,
) {
	tmpVar := "_" + sanitizeVarName(strings.ReplaceAll(flagName, "-", "_"))

	switch typeStr {
	case "[]byte":
		fmt.Fprintf(buf, "\t\t\tif %s, _ := cmd.Flags().GetString(%q); %s != \"\" {\n", tmpVar, flagName, tmpVar)
		fmt.Fprintf(buf, "\t\t\t\t_decoded, _err := hex.DecodeString(%s)\n", tmpVar)
		buf.WriteString("\t\t\t\tif _err != nil {\n")
		fmt.Fprintf(buf, "\t\t\t\t\treturn fmt.Errorf(\"invalid hex for %s: %%w\", _err)\n", flagName)
		buf.WriteString("\t\t\t\t}\n")
		fmt.Fprintf(buf, "\t\t\t\t%s = _decoded\n", fieldAccess)
		buf.WriteString("\t\t\t}\n")

	case "[]string":
		fmt.Fprintf(buf, "\t\t\tif %s, _ := cmd.Flags().GetString(%q); %s != \"\" {\n", tmpVar, flagName, tmpVar)
		fmt.Fprintf(buf, "\t\t\t\t%s = strings.Split(%s, \",\")\n", fieldAccess, tmpVar)
		buf.WriteString("\t\t\t}\n")

	default:
		elemType := typeStr[2:]
		parseFunc, bitSize := elemParseInfo(elemType)

		fmt.Fprintf(buf, "\t\t\tif %s, _ := cmd.Flags().GetString(%q); %s != \"\" {\n", tmpVar, flagName, tmpVar)
		fmt.Fprintf(buf, "\t\t\t\tfor _, _s := range strings.Split(%s, \",\") {\n", tmpVar)

		switch elemType {
		case "bool":
			buf.WriteString("\t\t\t\t\t_v, _err := strconv.ParseBool(strings.TrimSpace(_s))\n")
		case "float32", "float64":
			fmt.Fprintf(buf, "\t\t\t\t\t_v, _err := strconv.%s(strings.TrimSpace(_s), %d)\n", parseFunc, bitSize)
		default:
			fmt.Fprintf(buf, "\t\t\t\t\t_v, _err := strconv.%s(strings.TrimSpace(_s), 0, %d)\n", parseFunc, bitSize)
		}

		buf.WriteString("\t\t\t\t\tif _err != nil {\n")
		fmt.Fprintf(buf, "\t\t\t\t\t\treturn fmt.Errorf(\"invalid %s in %s: %%w\", _err)\n", elemType, flagName)
		buf.WriteString("\t\t\t\t\t}\n")
		fmt.Fprintf(buf, "\t\t\t\t\t%s = append(%s, %s(_v))\n", fieldAccess, fieldAccess, elemType)
		buf.WriteString("\t\t\t\t}\n")
		buf.WriteString("\t\t\t}\n")
	}
}

func writeStructFieldJSON(
	buf *bytes.Buffer,
	flagName string,
	fieldAccess string,
) {
	tmpVar := "_" + sanitizeVarName(strings.ReplaceAll(flagName, "-", "_"))
	fmt.Fprintf(buf, "\t\t\tif %s, _ := cmd.Flags().GetString(%q); %s != \"\" {\n", tmpVar, flagName, tmpVar)
	fmt.Fprintf(buf, "\t\t\t\tif _err := json.Unmarshal([]byte(%s), &%s); _err != nil {\n", tmpVar, fieldAccess)
	fmt.Fprintf(buf, "\t\t\t\t\treturn fmt.Errorf(\"invalid JSON for %s: %%w\", _err)\n", flagName)
	buf.WriteString("\t\t\t\t}\n")
	buf.WriteString("\t\t\t}\n")
}

func writeEnumExtraction(
	buf *bytes.Buffer,
	typeStr string,
	flagName string,
	varName string,
	gc *genContext,
) {
	tmpVar := varName + "Raw"
	fmt.Fprintf(buf, "\t\t\t%s, err := cmd.Flags().GetInt32(%q)\n", tmpVar, flagName)
	buf.WriteString("\t\t\tif err != nil {\n")
	fmt.Fprintf(buf, "\t\t\t\treturn fmt.Errorf(\"reading flag %s: %%w\", err)\n", flagName)
	buf.WriteString("\t\t\t}\n")

	qualType := gc.resolveGoType(typeStr)
	fmt.Fprintf(buf, "\t\t\t%s := %s(%s)\n\n", varName, qualType, tmpVar)
}

func writeBinderExtraction(
	buf *bytes.Buffer,
	flagName string,
	varName string,
) {
	fmt.Fprintf(buf, "\t\t\t%sName, err := cmd.Flags().GetString(%q)\n", varName, flagName)
	buf.WriteString("\t\t\tif err != nil {\n")
	fmt.Fprintf(buf, "\t\t\t\treturn fmt.Errorf(\"reading flag %s: %%w\", err)\n", flagName)
	buf.WriteString("\t\t\t}\n")
	fmt.Fprintf(buf, "\t\t\t%s, err := conn.GetService(ctx, %sName)\n", varName, varName)
	buf.WriteString("\t\t\tif err != nil {\n")
	fmt.Fprintf(buf, "\t\t\t\treturn fmt.Errorf(\"resolving binder %%q: %%w\", %sName, err)\n", varName)
	buf.WriteString("\t\t\t}\n\n")
}

func writeInterfaceExtraction(
	buf *bytes.Buffer,
	flagName string,
	varName string,
) {
	fmt.Fprintf(buf, "\t\t\t%sJSON, err := cmd.Flags().GetString(%q)\n", varName, flagName)
	buf.WriteString("\t\t\tif err != nil {\n")
	fmt.Fprintf(buf, "\t\t\t\treturn fmt.Errorf(\"reading flag %s: %%w\", err)\n", flagName)
	buf.WriteString("\t\t\t}\n")
	fmt.Fprintf(buf, "\t\t\tvar %s interface{}\n", varName)
	fmt.Fprintf(buf, "\t\t\tif %sJSON != \"\" {\n", varName)
	fmt.Fprintf(buf, "\t\t\t\tif err := json.Unmarshal([]byte(%sJSON), &%s); err != nil {\n", varName, varName)
	fmt.Fprintf(buf, "\t\t\t\t\treturn fmt.Errorf(\"invalid JSON for %s: %%w\", err)\n", flagName)
	buf.WriteString("\t\t\t\t}\n")
	buf.WriteString("\t\t\t}\n\n")
}

func writeInterfaceTypeExtraction(
	buf *bytes.Buffer,
	typeStr string,
	flagName string,
	varName string,
	gc *genContext,
) {
	qualType := gc.resolveGoType(typeStr)

	bareName := typeStr
	proxyPkg := gc.ifaceAlias
	if strings.Contains(typeStr, ".") {
		parts := strings.SplitN(typeStr, ".", 2)
		bareName = parts[1]
		_ = proxyPkg // keep using interface's alias for the proxy lookup
	}

	proxyConstructor := "New" + bareName[1:] + "Proxy"

	fmt.Fprintf(buf, "\t\t\t%sName, err := cmd.Flags().GetString(%q)\n", varName, flagName)
	buf.WriteString("\t\t\tif err != nil {\n")
	fmt.Fprintf(buf, "\t\t\t\treturn fmt.Errorf(\"reading flag %s: %%w\", err)\n", flagName)
	buf.WriteString("\t\t\t}\n")
	fmt.Fprintf(buf, "\t\t\t%sBinder, err := conn.GetService(ctx, %sName)\n", varName, varName)
	buf.WriteString("\t\t\tif err != nil {\n")
	fmt.Fprintf(buf, "\t\t\t\treturn fmt.Errorf(\"resolving service %%q for %s: %%w\", %sName, err)\n", flagName, varName)
	buf.WriteString("\t\t\t}\n")
	fmt.Fprintf(buf, "\t\t\tvar %s %s = %s.%s(%sBinder)\n\n", varName, qualType, proxyPkg, proxyConstructor, varName)
}

func writeNullableExtraction(
	buf *bytes.Buffer,
	typeStr string,
	flagName string,
	varName string,
	gc *genContext,
) {
	inner := typeStr[1:]
	innerKind := classifyType(inner, gc.iface)

	switch innerKind {
	case kindStruct:
		key := resolveTypeKey(inner, gc.iface.ImportPath)
		si := knownStructs[key]
		if si == nil {
			writeJSONFallbackExtraction(buf, typeStr, flagName, varName, gc)
			return
		}
		qualifiedType := gc.resolveGoType(inner)
		innerVar := varName + "Val"
		fmt.Fprintf(buf, "\t\t\tvar %s %s\n", innerVar, qualifiedType)
		for _, f := range si.Fields {
			fFlagName := flagName + "-" + camelToKebab(f.Name)
			fFieldAccess := innerVar + "." + f.Name
			writeStructFieldExtraction(buf, f, fFlagName, fFieldAccess, si, gc, 0)
		}
		fmt.Fprintf(buf, "\t\t\t%s := &%s\n\n", varName, innerVar)

	case kindEnum:
		fmt.Fprintf(buf, "\t\t\t%sStr, _ := cmd.Flags().GetString(%q)\n", varName, flagName)
		qualType := gc.resolveGoType(inner)
		fmt.Fprintf(buf, "\t\t\tvar %s *%s\n", varName, qualType)
		fmt.Fprintf(buf, "\t\t\tif %sStr != \"\" {\n", varName)
		fmt.Fprintf(buf, "\t\t\t\t_v, _err := strconv.ParseInt(%sStr, 0, 32)\n", varName)
		buf.WriteString("\t\t\t\tif _err != nil {\n")
		fmt.Fprintf(buf, "\t\t\t\t\treturn fmt.Errorf(\"invalid value for %s: %%w\", _err)\n", flagName)
		buf.WriteString("\t\t\t\t}\n")
		fmt.Fprintf(buf, "\t\t\t\t_typed := %s(_v)\n", qualType)
		fmt.Fprintf(buf, "\t\t\t\t%s = &_typed\n", varName)
		buf.WriteString("\t\t\t}\n\n")

	default:
		writeJSONFallbackExtraction(buf, typeStr, flagName, varName, gc)
	}
}

func writeNullablePrimitiveExtraction(
	buf *bytes.Buffer,
	typeStr string,
	flagName string,
	varName string,
) {
	inner := typeStr[1:]

	fmt.Fprintf(buf, "\t\t\t%sStr, _ := cmd.Flags().GetString(%q)\n", varName, flagName)
	fmt.Fprintf(buf, "\t\t\tvar %s *%s\n", varName, inner)
	fmt.Fprintf(buf, "\t\t\tif %sStr != \"\" {\n", varName)

	switch inner {
	case "string":
		fmt.Fprintf(buf, "\t\t\t\t_v := %sStr\n", varName)
	case "bool":
		fmt.Fprintf(buf, "\t\t\t\t_v, _err := strconv.ParseBool(%sStr)\n", varName)
		buf.WriteString("\t\t\t\tif _err != nil {\n")
		fmt.Fprintf(buf, "\t\t\t\t\treturn fmt.Errorf(\"invalid bool for %s: %%w\", _err)\n", flagName)
		buf.WriteString("\t\t\t\t}\n")
	case "int32":
		fmt.Fprintf(buf, "\t\t\t\t_v64, _err := strconv.ParseInt(%sStr, 0, 32)\n", varName)
		buf.WriteString("\t\t\t\tif _err != nil {\n")
		fmt.Fprintf(buf, "\t\t\t\t\treturn fmt.Errorf(\"invalid int32 for %s: %%w\", _err)\n", flagName)
		buf.WriteString("\t\t\t\t}\n")
		buf.WriteString("\t\t\t\t_v := int32(_v64)\n")
	case "int64":
		fmt.Fprintf(buf, "\t\t\t\t_v, _err := strconv.ParseInt(%sStr, 0, 64)\n", varName)
		buf.WriteString("\t\t\t\tif _err != nil {\n")
		fmt.Fprintf(buf, "\t\t\t\t\treturn fmt.Errorf(\"invalid int64 for %s: %%w\", _err)\n", flagName)
		buf.WriteString("\t\t\t\t}\n")
	case "float32":
		fmt.Fprintf(buf, "\t\t\t\t_v64, _err := strconv.ParseFloat(%sStr, 32)\n", varName)
		buf.WriteString("\t\t\t\tif _err != nil {\n")
		fmt.Fprintf(buf, "\t\t\t\t\treturn fmt.Errorf(\"invalid float32 for %s: %%w\", _err)\n", flagName)
		buf.WriteString("\t\t\t\t}\n")
		buf.WriteString("\t\t\t\t_v := float32(_v64)\n")
	case "float64":
		fmt.Fprintf(buf, "\t\t\t\t_v, _err := strconv.ParseFloat(%sStr, 64)\n", varName)
		buf.WriteString("\t\t\t\tif _err != nil {\n")
		fmt.Fprintf(buf, "\t\t\t\t\treturn fmt.Errorf(\"invalid float64 for %s: %%w\", _err)\n", flagName)
		buf.WriteString("\t\t\t\t}\n")
	case "byte":
		fmt.Fprintf(buf, "\t\t\t\t_v64, _err := strconv.ParseUint(%sStr, 0, 8)\n", varName)
		buf.WriteString("\t\t\t\tif _err != nil {\n")
		fmt.Fprintf(buf, "\t\t\t\t\treturn fmt.Errorf(\"invalid byte for %s: %%w\", _err)\n", flagName)
		buf.WriteString("\t\t\t\t}\n")
		buf.WriteString("\t\t\t\t_v := byte(_v64)\n")
	case "uint16":
		fmt.Fprintf(buf, "\t\t\t\t_v64, _err := strconv.ParseUint(%sStr, 0, 16)\n", varName)
		buf.WriteString("\t\t\t\tif _err != nil {\n")
		fmt.Fprintf(buf, "\t\t\t\t\treturn fmt.Errorf(\"invalid uint16 for %s: %%w\", _err)\n", flagName)
		buf.WriteString("\t\t\t\t}\n")
		buf.WriteString("\t\t\t\t_v := uint16(_v64)\n")
	}

	fmt.Fprintf(buf, "\t\t\t\t%s = &_v\n", varName)
	buf.WriteString("\t\t\t}\n\n")
}

func writeJSONFallbackExtraction(
	buf *bytes.Buffer,
	typeStr string,
	flagName string,
	varName string,
	gc *genContext,
) {
	qualType := gc.resolveGoType(typeStr)

	fmt.Fprintf(buf, "\t\t\t%sJSON, err := cmd.Flags().GetString(%q)\n", varName, flagName)
	buf.WriteString("\t\t\tif err != nil {\n")
	fmt.Fprintf(buf, "\t\t\t\treturn fmt.Errorf(\"reading flag %s: %%w\", err)\n", flagName)
	buf.WriteString("\t\t\t}\n")
	fmt.Fprintf(buf, "\t\t\tvar %s %s\n", varName, qualType)
	fmt.Fprintf(buf, "\t\t\tif %sJSON != \"\" {\n", varName)
	fmt.Fprintf(buf, "\t\t\t\tif err := json.Unmarshal([]byte(%sJSON), &%s); err != nil {\n", varName, varName)
	fmt.Fprintf(buf, "\t\t\t\t\treturn fmt.Errorf(\"invalid JSON for %s: %%w\", err)\n", flagName)
	buf.WriteString("\t\t\t\t}\n")
	buf.WriteString("\t\t\t}\n\n")
}

// ---- Helpers ----

func paramVarName(
	p paramInfo,
) string {
	return "flag" + capitalize(sanitizeVarName(p.Name))
}

func buildShortDesc(
	m methodInfo,
) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Call %s(", m.Name)
	for i, p := range m.Params {
		if i > 0 {
			sb.WriteString(", ")
		}
		fmt.Fprintf(&sb, "%s %s", p.Name, p.Type)
	}
	sb.WriteString(")")
	if m.ReturnType != "" {
		fmt.Fprintf(&sb, " -> %s", m.ReturnType)
	}
	return sb.String()
}

func sanitizeVarName(
	name string,
) string {
	return strings.TrimRight(name, "_")
}

func capitalize(
	s string,
) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func uniqueAlias(
	pkgName string,
	counts map[string]int,
) string {
	counts[pkgName]++
	if counts[pkgName] == 1 {
		return pkgName
	}
	return fmt.Sprintf("%s%d", pkgName, counts[pkgName])
}

func cmdGroupFuncName(
	iface *interfaceInfo,
) string {
	return "newCmd" + descriptorToIdent(iface.Descriptor)
}

func cmdMethodFuncName(
	iface *interfaceInfo,
	m methodInfo,
) string {
	return "newCmd" + descriptorToIdent(iface.Descriptor) + "_" + m.Name
}

func descriptorToIdent(
	descriptor string,
) string {
	parts := strings.Split(descriptor, ".")
	var sb strings.Builder
	for _, p := range parts {
		if len(p) == 0 {
			continue
		}
		sb.WriteString(strings.ToUpper(p[:1]))
		sb.WriteString(p[1:])
	}
	return sb.String()
}

func camelToKebab(
	s string,
) string {
	var b strings.Builder
	for i, r := range s {
		if unicode.IsUpper(r) && i > 0 {
			b.WriteByte('-')
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}
