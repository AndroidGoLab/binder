// Command genaidlcli scans generated AIDL proxy Go files and produces
// registry_gen.go and commands_gen.go for the aidlcli tool.
package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

const (
	modulePath = "github.com/xaionaro-go/aidl"
	outputDir  = "tools/cmd/aidlcli"
)

// scanDirs lists the directories (relative to module root) that contain
// generated proxy Go files.
var scanDirs = []string{
	"android",
	"com",
	"src",
	"fuzztest",
	"libgui_test_server",
}

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

// primitiveArrayElemTypes lists the element types that can appear in
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

// interfaceInfo holds parsed metadata for one AIDL interface.
type interfaceInfo struct {
	Descriptor       string
	ProxyConstructor string // e.g. "NewActivityManagerProxy"
	ProxyType        string // e.g. "ActivityManagerProxy"
	ImportPath       string // e.g. "github.com/xaionaro-go/aidl/android/app"
	PkgName          string // e.g. "app"
	Methods          []methodInfo
	// fileImports maps local package name -> import path for the source file.
	// Used to resolve cross-package type references like "common.Rect".
	FileImports map[string]string
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

// structField describes one field inside a scanned struct type.
type structField struct {
	Name string
	Type string
}

// structInfo holds parsed struct metadata from generated code.
type structInfo struct {
	Fields     []structField
	ImportPath string
	PkgName    string
	// fileImports from the file this struct was defined in, for
	// resolving cross-package field types.
	FileImports map[string]string
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
// Populated by scanTypes before command generation.
var knownStructs map[string]*structInfo

// knownEnums maps "importPath:TypeName" -> true.
var knownEnums map[string]bool

func main() {
	rootDir, err := findModuleRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Phase 1: scan struct and enum types from generated files.
	knownStructs, knownEnums = scanTypes(rootDir)
	fmt.Printf("Scanned struct types: %d\n", len(knownStructs))
	fmt.Printf("Scanned enum types: %d\n", len(knownEnums))

	// Phase 2: scan interfaces.
	interfaces := scanAllInterfaces(rootDir)
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

	fmt.Printf("Scanned interfaces: %d\n", len(interfaces))
	fmt.Printf("Total methods: %d\n", totalMethods)
	fmt.Printf("Generated commands for %d/%d methods (%.1f%%)\n", commandableMethods, totalMethods, pct)

	outPath := filepath.Join(rootDir, outputDir)

	if err := writeRegistryGen(outPath, interfaces); err != nil {
		fmt.Fprintf(os.Stderr, "error writing registry_gen.go: %v\n", err)
		os.Exit(1)
	}

	if err := writeCommandsGen(outPath, interfaces); err != nil {
		fmt.Fprintf(os.Stderr, "error writing commands_gen.go: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Generated registry_gen.go and commands_gen.go")
}

// findModuleRoot walks upward from the current directory to find go.mod.
func findModuleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("cannot find go.mod in any parent directory")
		}
		dir = parent
	}
}

// ---- Phase 1: Type scanning ----

func scanTypes(rootDir string) (map[string]*structInfo, map[string]bool) {
	structs := map[string]*structInfo{}
	enums := map[string]bool{}

	walkGenerated(rootDir, func(path string, f *ast.File) {
		relDir, err := filepath.Rel(rootDir, filepath.Dir(path))
		if err != nil {
			return
		}
		var importPath, pkgName string
		if relDir == "." {
			importPath = modulePath
			pkgName = f.Name.Name
		} else {
			importPath = modulePath + "/" + filepath.ToSlash(relDir)
			pkgName = f.Name.Name
		}

		fileImports := extractFileImports(f)

		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, spec := range gd.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				key := importPath + ":" + ts.Name.Name
				switch t := ts.Type.(type) {
				case *ast.StructType:
					si := &structInfo{
						ImportPath:  importPath,
						PkgName:     pkgName,
						FileImports: fileImports,
					}
					if t.Fields != nil {
						for _, field := range t.Fields.List {
							typeStr := exprToString(field.Type)
							for _, name := range field.Names {
								si.Fields = append(si.Fields, structField{
									Name: name.Name,
									Type: typeStr,
								})
							}
						}
					}
					structs[key] = si
				case *ast.Ident:
					switch t.Name {
					case "int32", "int64", "byte":
						enums[key] = true
					}
				}
			}
		}
	})

	return structs, enums
}

// extractFileImports returns a map of local package name -> import path
// from the file's import declarations.
func extractFileImports(f *ast.File) map[string]string {
	result := map[string]string{}
	for _, imp := range f.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		var localName string
		if imp.Name != nil {
			localName = imp.Name.Name
		} else {
			localName = filepath.Base(path)
		}
		result[localName] = path
	}
	return result
}

func walkGenerated(rootDir string, fn func(path string, f *ast.File)) {
	visit := func(path string) {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return
		}
		if !isGenerated(f) {
			return
		}
		fn(path, f)
	}

	for _, dir := range scanDirs {
		absDir := filepath.Join(rootDir, dir)
		if info, err := os.Stat(absDir); err != nil || !info.IsDir() {
			continue
		}
		filepath.Walk(absDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
				return nil
			}
			visit(path)
			return nil
		})
	}

	entries, err := os.ReadDir(rootDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		visit(filepath.Join(rootDir, entry.Name()))
	}
}

// ---- Type resolution ----

// resolveTypeKey resolves a type string to its knownStructs/knownEnums
// key ("importPath:TypeName") using the file's import context.
// For unqualified types, uses ifaceImportPath as the package.
// For qualified types like "pkg.Type", looks up pkg in fileImports.
func resolveTypeKey(typeStr string, ifaceImportPath string, fileImports map[string]string) string {
	// Strip pointer/slice prefixes to get the bare type.
	bare := typeStr
	for strings.HasPrefix(bare, "*") || strings.HasPrefix(bare, "[]") {
		if strings.HasPrefix(bare, "*") {
			bare = bare[1:]
		} else {
			bare = bare[2:]
		}
	}

	if strings.Contains(bare, ".") {
		parts := strings.SplitN(bare, ".", 2)
		pkgAlias := parts[0]
		typeName := parts[1]
		if importPath, ok := fileImports[pkgAlias]; ok {
			return importPath + ":" + typeName
		}
		// Can't resolve -- return best guess.
		return pkgAlias + ":" + typeName
	}

	return ifaceImportPath + ":" + bare
}

// lookupStruct looks up a struct by resolving the type string using
// the provided import context.
func lookupStruct(typeStr string, ifaceImportPath string, fileImports map[string]string) *structInfo {
	key := resolveTypeKey(typeStr, ifaceImportPath, fileImports)
	return knownStructs[key]
}

// lookupEnum checks if a type string resolves to a known enum.
func lookupEnum(typeStr string, ifaceImportPath string, fileImports map[string]string) bool {
	key := resolveTypeKey(typeStr, ifaceImportPath, fileImports)
	return knownEnums[key]
}

// ---- Phase 2: Interface scanning ----

func scanAllInterfaces(rootDir string) []*interfaceInfo {
	var result []*interfaceInfo

	for _, dir := range scanDirs {
		absDir := filepath.Join(rootDir, dir)
		if info, err := os.Stat(absDir); err != nil || !info.IsDir() {
			continue
		}
		filepath.Walk(absDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
				return nil
			}
			if iface := parseFile(rootDir, path); iface != nil {
				result = append(result, iface)
			}
			return nil
		})
	}

	entries, err := os.ReadDir(rootDir)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
				continue
			}
			path := filepath.Join(rootDir, entry.Name())
			if iface := parseFile(rootDir, path); iface != nil {
				result = append(result, iface)
			}
		}
	}

	return result
}

func parseFile(rootDir, path string) *interfaceInfo {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil
	}

	if !isGenerated(f) {
		return nil
	}

	descriptor := findDescriptor(f)
	if descriptor == "" {
		return nil
	}

	proxyConstructor, proxyType := findProxyConstructor(f)
	if proxyConstructor == "" {
		return nil
	}

	relDir, err := filepath.Rel(rootDir, filepath.Dir(path))
	if err != nil {
		return nil
	}

	var importPath, pkgName string
	if relDir == "." {
		importPath = modulePath
		pkgName = f.Name.Name
	} else {
		importPath = modulePath + "/" + filepath.ToSlash(relDir)
		pkgName = f.Name.Name
	}

	methods := extractProxyMethods(f, proxyType)

	return &interfaceInfo{
		Descriptor:       descriptor,
		ProxyConstructor: proxyConstructor,
		ProxyType:        proxyType,
		ImportPath:       importPath,
		PkgName:          pkgName,
		Methods:          methods,
		FileImports:      extractFileImports(f),
	}
}

func isGenerated(f *ast.File) bool {
	for _, cg := range f.Comments {
		for _, c := range cg.List {
			if strings.Contains(c.Text, "Code generated") {
				return true
			}
		}
	}
	return false
}

func findDescriptor(f *ast.File) string {
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok || len(vs.Names) == 0 || len(vs.Values) == 0 {
				continue
			}
			if !strings.HasPrefix(vs.Names[0].Name, "Descriptor") {
				continue
			}
			lit, ok := vs.Values[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				continue
			}
			return strings.Trim(lit.Value, `"`)
		}
	}
	return ""
}

func findProxyConstructor(f *ast.File) (string, string) {
	for _, decl := range f.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Recv != nil {
			continue
		}
		name := fd.Name.Name
		if !strings.HasPrefix(name, "New") || !strings.HasSuffix(name, "Proxy") {
			continue
		}
		params := fd.Type.Params
		if params == nil || len(params.List) != 1 {
			continue
		}
		if !isBinderIBinder(params.List[0].Type) {
			continue
		}
		proxyType := strings.TrimPrefix(name, "New")
		return name, proxyType
	}
	return "", ""
}

func isBinderIBinder(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	return ident.Name == "binder" && sel.Sel.Name == "IBinder"
}

func extractProxyMethods(f *ast.File, proxyType string) []methodInfo {
	var methods []methodInfo
	for _, decl := range f.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Recv == nil {
			continue
		}
		if fd.Name.Name == "AsBinder" {
			continue
		}
		if !receiverIs(fd, proxyType) {
			continue
		}

		params := fd.Type.Params.List
		if len(params) == 0 || !isContextContext(params[0].Type) {
			continue
		}

		m := methodInfo{Name: fd.Name.Name}

		for _, p := range params[1:] {
			typeStr := exprToString(p.Type)
			for _, n := range p.Names {
				m.Params = append(m.Params, paramInfo{Name: n.Name, Type: typeStr})
			}
		}

		m.ReturnType = extractReturnType(fd.Type.Results)
		methods = append(methods, m)
	}
	return methods
}

func receiverIs(fd *ast.FuncDecl, typeName string) bool {
	if fd.Recv == nil || len(fd.Recv.List) == 0 {
		return false
	}
	star, ok := fd.Recv.List[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	ident, ok := star.X.(*ast.Ident)
	if !ok {
		return false
	}
	return ident.Name == typeName
}

func isContextContext(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	return ident.Name == "context" && sel.Sel.Name == "Context"
}

func exprToString(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return exprToString(e.X) + "." + e.Sel.Name
	case *ast.StarExpr:
		return "*" + exprToString(e.X)
	case *ast.ArrayType:
		if e.Len == nil {
			return "[]" + exprToString(e.Elt)
		}
		return "[" + exprToString(e.Len) + "]" + exprToString(e.Elt)
	case *ast.MapType:
		return "map[" + exprToString(e.Key) + "]" + exprToString(e.Value)
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.BasicLit:
		return e.Value
	case *ast.Ellipsis:
		return "..." + exprToString(e.Elt)
	default:
		return "UNKNOWN"
	}
}

func extractReturnType(results *ast.FieldList) string {
	if results == nil {
		return ""
	}
	for _, r := range results.List {
		typeStr := exprToString(r.Type)
		if typeStr == "error" {
			continue
		}
		return typeStr
	}
	return ""
}

// ---- Type classification ----

// classifyType determines how a parameter type should be handled.
// Uses the interface's import context for cross-package type resolution.
func classifyType(typeStr string, iface *interfaceInfo) typeKind {
	if _, ok := primitiveTypes[typeStr]; ok {
		return kindPrimitive
	}

	// Nullable pointer wrapping.
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

	// Primitive arrays.
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

	// Array of complex types.
	if strings.HasPrefix(typeStr, "[]") {
		return kindComplexArray
	}

	// Resolve type using import context.
	if lookupEnum(typeStr, iface.ImportPath, iface.FileImports) {
		return kindEnum
	}

	if lookupStruct(typeStr, iface.ImportPath, iface.FileImports) != nil {
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

func isAIDLInterfaceName(name string) bool {
	if len(name) < 2 {
		return false
	}
	return name[0] == 'I' && name[1] >= 'A' && name[1] <= 'Z'
}

func allParamsSupported(m methodInfo, iface *interfaceInfo) bool {
	for _, p := range m.Params {
		if classifyType(p.Type, iface) == kindUnsupported {
			return false
		}
	}
	return true
}

// ---- Import alias resolution for generated code ----

// genContext holds the state needed during code generation for resolving
// type names into the correct import alias in the generated file.
type genContext struct {
	iface         *interfaceInfo
	ifaceAlias    string              // import alias for the interface's package
	importAliases map[string]string   // importPath -> alias in generated code
	aliasCounts   map[string]int      // for generating unique aliases
}

// resolveGoType converts a type string from the source file into a
// fully-qualified Go type expression using the import aliases in the
// generated code.
func (gc *genContext) resolveGoType(typeStr string) string {
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
		localPkg := parts[0]
		typeName := parts[1]

		// Look up the actual import path from the source file.
		importPath, ok := gc.iface.FileImports[localPkg]
		if !ok {
			// Can't resolve; use as-is (will likely fail to compile,
			// but this shouldn't happen for well-formed generated code).
			return typeStr
		}

		// Ensure this import path has an alias in the generated code.
		alias := gc.ensureImport(importPath)
		return alias + "." + typeName
	}

	// Unqualified type in the same package as the interface.
	return gc.ifaceAlias + "." + typeStr
}

// ensureImport makes sure an import path has an alias in the generated
// code and returns that alias.
func (gc *genContext) ensureImport(importPath string) string {
	if importPath == "github.com/xaionaro-go/aidl/binder" {
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

func (n *needsTracker) methodNeeds(m methodInfo, iface *interfaceInfo) {
	for _, p := range m.Params {
		n.paramNeeds(p.Type, iface)
	}
}

func (n *needsTracker) paramNeeds(typeStr string, iface *interfaceInfo) {
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
		si := lookupStruct(typeStr, iface.ImportPath, iface.FileImports)
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

func (n *needsTracker) structFieldsNeeds(si *structInfo, depth int) {
	if depth > 3 {
		n.json = true
		return
	}
	for _, f := range si.Fields {
		n.fieldNeeds(f, si, depth)
	}
}

func (n *needsTracker) fieldNeeds(f structField, parentSI *structInfo, depth int) {
	// For struct fields, we use the struct's own file imports to resolve types.
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
		innerSI := lookupStruct(f.Type, parentSI.ImportPath, parentSI.FileImports)
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

// classifyFieldType classifies a struct field's type using the struct's
// own import context (not the interface's).
func classifyFieldType(typeStr string, si *structInfo) typeKind {
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

	if lookupEnum(typeStr, si.ImportPath, si.FileImports) {
		return kindEnum
	}
	if lookupStruct(typeStr, si.ImportPath, si.FileImports) != nil {
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

// ---- Generation: registry_gen.go ----

func writeRegistryGen(outDir string, interfaces []*interfaceInfo) error {
	var buf bytes.Buffer
	buf.WriteString("// Code generated by genaidlcli. DO NOT EDIT.\n\n")
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

	return os.WriteFile(filepath.Join(outDir, "registry_gen.go"), formatted, 0644)
}

// ---- Generation: commands_gen.go ----

type commandableIface struct {
	iface          *interfaceInfo
	importAlias    string
	commandMethods []methodInfo
}

func writeCommandsGen(outDir string, interfaces []*interfaceInfo) error {
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
	buf.WriteString("// Code generated by genaidlcli. DO NOT EDIT.\n\n")
	buf.WriteString("//go:build linux\n\n")
	buf.WriteString("package main\n\n")

	writeImports(&buf, importAliases, needs)
	buf.Write(body.Bytes())

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return fmt.Errorf("formatting commands_gen.go: %w\n\nsource:\n%s", err, buf.String())
	}

	return os.WriteFile(filepath.Join(outDir, "commands_gen.go"), formatted, 0644)
}


// resolveFieldGoType resolves a struct field's type to a Go expression
// using the struct's own file imports (not the interface's).
func (gc *genContext) resolveFieldGoType(typeStr string, parentSI *structInfo) string {
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
		localPkg := parts[0]
		typeName := parts[1]
		if importPath, ok := parentSI.FileImports[localPkg]; ok {
			alias := gc.ensureImport(importPath)
			return alias + "." + typeName
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

func writeImports(buf *bytes.Buffer, importAliases map[string]string, needs needsTracker) {
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
	buf.WriteString("\t\"github.com/xaionaro-go/aidl/binder\"\n")

	type importEntry struct {
		path  string
		alias string
	}
	var imports []importEntry
	for path, alias := range importAliases {
		if path == "github.com/xaionaro-go/aidl/binder" || path == modulePath {
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

func writeAddGeneratedCommands(buf *bytes.Buffer, commandables []commandableIface) {
	buf.WriteString("func addGeneratedCommands(root *cobra.Command) {\n")
	for _, ci := range commandables {
		fmt.Fprintf(buf, "\troot.AddCommand(%s())\n", cmdGroupFuncName(ci.iface))
	}
	buf.WriteString("}\n\n")
}

func writeFindServiceByDescriptorFunc(buf *bytes.Buffer) {
	buf.WriteString(`func findServiceByDescriptor(
	ctx context.Context,
	conn *Conn,
	descriptor string,
) (binder.IBinder, error) {
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

func writeInterfaceGroup(buf *bytes.Buffer, ci commandableIface, gc *genContext) {
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
	hasParams := len(m.Params) > 0

	qualifier := alias

	fmt.Fprintf(buf, "func %s() *cobra.Command {\n", funcName)

	if hasParams {
		buf.WriteString("\tcmd := &cobra.Command{\n")
	} else {
		buf.WriteString("\treturn &cobra.Command{\n")
	}
	fmt.Fprintf(buf, "\t\tUse:   %q,\n", kebabName)
	fmt.Fprintf(buf, "\t\tShort: %q,\n", shortDesc)

	buf.WriteString("\t\tRunE: func(cmd *cobra.Command, args []string) error {\n")
	buf.WriteString("\t\t\tctx := context.Background()\n\n")
	buf.WriteString("\t\t\tconn, err := OpenConn(ctx, cmd)\n")
	buf.WriteString("\t\t\tif err != nil {\n")
	buf.WriteString("\t\t\t\treturn err\n")
	buf.WriteString("\t\t\t}\n")
	buf.WriteString("\t\t\tdefer conn.Close(ctx)\n\n")

	fmt.Fprintf(buf, "\t\t\tsvc, err := findServiceByDescriptor(ctx, conn, %q)\n", iface.Descriptor)
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

	if hasParams {
		buf.WriteString("\t}\n\n")
		for _, p := range m.Params {
			writeParamFlag(buf, p, gc)
		}
		buf.WriteString("\n\treturn cmd\n")
	} else {
		buf.WriteString("\t}\n")
	}

	buf.WriteString("}\n\n")
}

// ---- Per-param flag registration ----

func writeParamFlag(buf *bytes.Buffer, p paramInfo, gc *genContext) {
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
		si := lookupStruct(p.Type, gc.iface.ImportPath, gc.iface.FileImports)
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
			si := lookupStruct(inner, gc.iface.ImportPath, gc.iface.FileImports)
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

func writeStructFlags(buf *bytes.Buffer, prefix string, si *structInfo, gc *genContext, depth int) {
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
			innerSI := lookupStruct(f.Type, si.ImportPath, si.FileImports)
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

func writeParamExtraction(buf *bytes.Buffer, p paramInfo, gc *genContext) {
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

func writePrimitiveArrayExtraction(buf *bytes.Buffer, typeStr, flagName, varName string) {
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

func elemParseInfo(elemType string) (string, int) {
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
	typeStr, prefix, varName string,
	gc *genContext,
	depth int,
) {
	si := lookupStruct(typeStr, gc.iface.ImportPath, gc.iface.FileImports)
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
	flagName, fieldAccess string,
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
		innerSI := lookupStruct(f.Type, parentSI.ImportPath, parentSI.FileImports)
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

func writeStructFieldPrimitiveArray(buf *bytes.Buffer, typeStr, flagName, fieldAccess string) {
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

func writeStructFieldJSON(buf *bytes.Buffer, flagName, fieldAccess string) {
	tmpVar := "_" + sanitizeVarName(strings.ReplaceAll(flagName, "-", "_"))
	fmt.Fprintf(buf, "\t\t\tif %s, _ := cmd.Flags().GetString(%q); %s != \"\" {\n", tmpVar, flagName, tmpVar)
	fmt.Fprintf(buf, "\t\t\t\tif _err := json.Unmarshal([]byte(%s), &%s); _err != nil {\n", tmpVar, fieldAccess)
	fmt.Fprintf(buf, "\t\t\t\t\treturn fmt.Errorf(\"invalid JSON for %s: %%w\", _err)\n", flagName)
	buf.WriteString("\t\t\t\t}\n")
	buf.WriteString("\t\t\t}\n")
}

func writeEnumExtraction(
	buf *bytes.Buffer,
	typeStr, flagName, varName string,
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

func writeBinderExtraction(buf *bytes.Buffer, flagName, varName string) {
	fmt.Fprintf(buf, "\t\t\t%sName, err := cmd.Flags().GetString(%q)\n", varName, flagName)
	buf.WriteString("\t\t\tif err != nil {\n")
	fmt.Fprintf(buf, "\t\t\t\treturn fmt.Errorf(\"reading flag %s: %%w\", err)\n", flagName)
	buf.WriteString("\t\t\t}\n")
	fmt.Fprintf(buf, "\t\t\t%s, err := conn.GetService(ctx, %sName)\n", varName, varName)
	buf.WriteString("\t\t\tif err != nil {\n")
	fmt.Fprintf(buf, "\t\t\t\treturn fmt.Errorf(\"resolving binder %%q: %%w\", %sName, err)\n", varName)
	buf.WriteString("\t\t\t}\n\n")
}

func writeInterfaceExtraction(buf *bytes.Buffer, flagName, varName string) {
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
	typeStr, flagName, varName string,
	gc *genContext,
) {
	qualType := gc.resolveGoType(typeStr)

	bareName := typeStr
	proxyPkg := gc.ifaceAlias
	if strings.Contains(typeStr, ".") {
		parts := strings.SplitN(typeStr, ".", 2)
		localPkg := parts[0]
		bareName = parts[1]
		// Resolve the package alias.
		if importPath, ok := gc.iface.FileImports[localPkg]; ok {
			proxyPkg = gc.ensureImport(importPath)
		}
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
	typeStr, flagName, varName string,
	gc *genContext,
) {
	inner := typeStr[1:]
	innerKind := classifyType(inner, gc.iface)

	switch innerKind {
	case kindStruct:
		si := lookupStruct(inner, gc.iface.ImportPath, gc.iface.FileImports)
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
	typeStr, flagName, varName string,
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
	typeStr, flagName, varName string,
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

func paramVarName(p paramInfo) string {
	return "flag" + capitalize(sanitizeVarName(p.Name))
}

func buildShortDesc(m methodInfo) string {
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

func sanitizeVarName(name string) string {
	return strings.TrimRight(name, "_")
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func uniqueAlias(pkgName string, counts map[string]int) string {
	counts[pkgName]++
	if counts[pkgName] == 1 {
		return pkgName
	}
	return fmt.Sprintf("%s%d", pkgName, counts[pkgName])
}

func cmdGroupFuncName(iface *interfaceInfo) string {
	return "newCmd" + descriptorToIdent(iface.Descriptor)
}

func cmdMethodFuncName(iface *interfaceInfo, m methodInfo) string {
	return "newCmd" + descriptorToIdent(iface.Descriptor) + "_" + m.Name
}

func descriptorToIdent(descriptor string) string {
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

func camelToKebab(s string) string {
	var b strings.Builder
	for i, r := range s {
		if unicode.IsUpper(r) && i > 0 {
			b.WriteByte('-')
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}
