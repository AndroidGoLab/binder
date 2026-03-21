package codegen

import (
	"strings"

	"github.com/xaionaro-go/binder/tools/pkg/parser"
	"github.com/xaionaro-go/binder/tools/pkg/resolver"
)

const goModulePath = "github.com/xaionaro-go/binder"

// resolveTypeRef converts a TypeSpecifier to a Go type string using the
// TypeRefResolver if available, falling back to AIDLTypeToGo otherwise.
func resolveTypeRef(
	typeRef *TypeRefResolver,
	ts *parser.TypeSpecifier,
) string {
	if typeRef != nil {
		return typeRef.GoTypeRef(ts)
	}
	return AIDLTypeToGo(ts)
}

// TypeRefResolver resolves AIDL type references to qualified Go type strings,
// adding import statements to the GoFile when a type is from a different package.
type TypeRefResolver struct {
	Registry   *resolver.TypeRegistry
	CurrentPkg string // current AIDL package (e.g., "android.hardware.audio.common")
	GoFile     *GoFile
	// AliasMap caches the Go import alias assigned per AIDL package to avoid
	// collisions when two different packages share the same last segment.
	AliasMap map[string]string
	// UsedAliases tracks aliases already assigned to detect collisions.
	UsedAliases map[string]bool
	// ReservedNames holds identifiers that must not be used as import aliases
	// because they appear as parameter names in method signatures. An alias
	// matching a parameter would shadow the import within that method body.
	ReservedNames map[string]bool
	// ImportGraph is used to detect import cycles. When set, cross-package
	// type references that would create cycles are replaced with any.
	ImportGraph *ImportGraph
	// CycleBreaks tracks qualified names that were resolved to any
	// due to import cycles.
	CycleBreaks map[string]bool
	// CycleTypeCallback is called when a type is redirected to a "types"
	// sub-package to break an import cycle.
	CycleTypeCallback func(qualifiedName, typesPkg string)
	// ResolvedTypes caches the Go type string for each AIDL type name to
	// ensure consistent resolution across multiple calls (avoiding
	// non-determinism from map iteration in the type registry).
	ResolvedTypes map[string]string
}

// NewTypeRefResolver creates a resolver for type references in the given AIDL package.
func NewTypeRefResolver(
	registry *resolver.TypeRegistry,
	currentPkg string,
	goFile *GoFile,
) *TypeRefResolver {
	return &TypeRefResolver{
		Registry:      registry,
		CurrentPkg:    currentPkg,
		GoFile:        goFile,
		AliasMap:      make(map[string]string),
		UsedAliases:   make(map[string]bool),
		ReservedNames: make(map[string]bool),
		CycleBreaks:   make(map[string]bool),
		ResolvedTypes: make(map[string]string),
	}
}

// GoTypeRef converts an AIDL TypeSpecifier to a Go type string, qualifying
// cross-package references and adding imports to the GoFile as needed.
func (r *TypeRefResolver) GoTypeRef(ts *parser.TypeSpecifier) string {
	if ts == nil {
		return ""
	}

	goType := r.goTypeRefInner(ts)

	if hasAnnotation(ts.Annots, "nullable") && goType != "" && goType != "string" {
		if goType[0] != '*' && goType[0] != '[' && !strings.HasPrefix(goType, "map[") {
			// Don't add pointer for interface types — Go interfaces
			// are inherently nullable (nil-able). This covers both
			// AIDL interface definitions (checked via registry) and
			// the built-in IBinder type (a Go interface not in the
			// registry).
			if !r.isGoInterfaceType(ts.Name) {
				goType = "*" + goType
			}
		}
	}

	return goType
}

// goTypeRefInner converts a TypeSpecifier without handling @nullable.
func (r *TypeRefResolver) goTypeRefInner(ts *parser.TypeSpecifier) string {
	if mapped, ok := aidlPrimitiveToGo[ts.Name]; ok {
		base := mapped
		if ts.IsArray {
			return "[]" + base
		}
		return base
	}

	switch ts.Name {
	case "List":
		elem := "any"
		if len(ts.TypeArgs) > 0 {
			elem = r.GoTypeRef(ts.TypeArgs[0])
		}
		return "[]" + elem

	case "Map":
		key := "any"
		val := "any"
		if len(ts.TypeArgs) >= 2 {
			key = r.GoTypeRef(ts.TypeArgs[0])
			val = r.GoTypeRef(ts.TypeArgs[1])
		}
		return "map[" + key + "]" + val
	}

	// User-defined type. Try to resolve via the registry.
	goName := r.resolveUserType(ts.Name)
	if goName == "" {
		goName = "any"
	}
	if ts.IsArray {
		return "[]" + goName
	}
	return goName
}

// resolveUserType resolves a user-defined AIDL type name to a Go type string.
// If the type is from a different package, it adds the import and returns
// a qualified name (e.g., "common.AudioUsage"). Results are cached to ensure
// consistent resolution across multiple calls for the same type name.
func (r *TypeRefResolver) resolveUserType(aidlName string) string {
	if r.Registry == nil {
		return aidlDottedNameToGo(aidlName)
	}

	// Return cached result if available, ensuring the same AIDL type name
	// always resolves to the same Go type within a single file generation.
	if cached, ok := r.ResolvedTypes[aidlName]; ok {
		return cached
	}

	result := r.resolveUserTypeUncached(aidlName)
	r.ResolvedTypes[aidlName] = result
	return result
}

// resolveUserTypeUncached performs the actual type resolution without caching.
func (r *TypeRefResolver) resolveUserTypeUncached(aidlName string) string {
	// Try fully qualified lookup first (e.g., "android.os.Bundle").
	if qualifiedName, ok := r.tryResolve(aidlName); ok {
		return r.qualifiedGoRef(qualifiedName, aidlName)
	}

	// Try short name lookup (e.g., "AudioUsage" -> "android.media.audio.common.AudioUsage").
	if qualifiedName, _, ok := r.Registry.LookupQualifiedByShortName(aidlName); ok {
		return r.qualifiedGoRef(qualifiedName, aidlName)
	}

	// For dotted names like "ActivityManager.RunningTaskInfo", try resolving
	// the first segment and appending the rest as a nested type.
	if strings.Contains(aidlName, ".") {
		if qualifiedName, ok := r.resolveNestedType(aidlName); ok {
			return r.qualifiedGoRef(qualifiedName, aidlName)
		}
	}

	// Unknown type: fall back to any to avoid compile errors.
	return "any"
}

// resolveNestedType attempts to resolve a dotted AIDL type name by looking up
// the first segment as a type and treating the rest as nested type segments.
// E.g., "ActivityManager.RunningTaskInfo" -> look up "ActivityManager" to find
// "android.app.ActivityManager", then derive "android.app.ActivityManager.RunningTaskInfo".
func (r *TypeRefResolver) resolveNestedType(aidlName string) (string, bool) {
	dotIdx := strings.IndexByte(aidlName, '.')
	if dotIdx < 0 {
		return "", false
	}

	firstPart := aidlName[:dotIdx]
	rest := aidlName[dotIdx+1:]

	// Try short name lookup for the first segment.
	if parentQualified, _, ok := r.Registry.LookupQualifiedByShortName(firstPart); ok {
		candidate := parentQualified + "." + rest
		if _, ok := r.Registry.Lookup(candidate); ok {
			return candidate, true
		}
	}

	// Try current package.
	if r.CurrentPkg != "" {
		candidate := r.CurrentPkg + "." + aidlName
		if _, ok := r.Registry.Lookup(candidate); ok {
			return candidate, true
		}
	}

	return "", false
}

// tryResolve attempts a fully qualified lookup and returns the qualified name.
func (r *TypeRefResolver) tryResolve(name string) (string, bool) {
	if _, ok := r.Registry.Lookup(name); ok {
		return name, true
	}

	// For dotted names that might be nested types, try prepending the current package.
	if r.CurrentPkg != "" && !strings.Contains(name, ".") {
		candidate := r.CurrentPkg + "." + name
		if _, ok := r.Registry.Lookup(candidate); ok {
			return candidate, true
		}
	}

	return "", false
}

// qualifiedGoRef returns the Go type reference for a fully qualified AIDL name,
// adding an import if the type is from a different package.
func (r *TypeRefResolver) qualifiedGoRef(
	qualifiedName string,
	originalName string,
) string {
	// Check if the definition is a forward-declared parcelable (no fields,
	// has cpp_header). These produce no generated code, so use any.
	if r.isForwardDeclared(qualifiedName) {
		return "any"
	}

	typePkg, goTypeName := r.splitQualifiedName(qualifiedName)

	// Same package: no qualifier needed.
	if typePkg == r.CurrentPkg {
		return goTypeName
	}

	// Check if importing this package would create an import cycle.
	// Redirect to a "types" sub-package instead of falling back to
	// any. The types-only dependency graph is acyclic (cycles
	// exist only in interface method references), so types sub-packages
	// can always be safely imported.
	//
	// Skip cycle checking when the current package is already a .types
	// sub-package: these use the strict SCC graph which is already
	// acyclic, so no further redirection is needed. Without this guard,
	// a .types package would redirect to foo.types.types which doesn't exist.
	if r.ImportGraph != nil &&
		!strings.HasSuffix(r.CurrentPkg, ".types") &&
		r.ImportGraph.WouldCauseCycle(r.CurrentPkg, typePkg) {
		// Redirect to a "types" sub-package. For non-interface types
		// (parcelables, enums, unions), the full definition goes to the
		// sub-package. For interface types, only the Go interface
		// definition goes to the sub-package; the proxy/stub stays in
		// the original package.
		if !r.isForwardDeclared(qualifiedName) {
			typesPkg := typePkg + ".types"
			alias := r.ensureImport(typesPkg)
			if r.CycleTypeCallback != nil {
				r.CycleTypeCallback(qualifiedName, typesPkg)
			}
			return alias + "." + goTypeName
		}
		r.CycleBreaks[qualifiedName] = true
		return "any"
	}

	// Different package: add import and qualify.
	alias := r.ensureImport(typePkg)
	return alias + "." + goTypeName
}

// isForwardDeclared returns true if the definition is a forward-declared
// parcelable that maps to a native C++/NDK/Rust type and won't produce
// generated Go code.
//
// A parcelable is forward-declared if it has no fields, no constants, no
// nested types, AND has a CppHeader, NdkHeader, or RustType annotation
// (indicating it is implemented in a native language, not AIDL).
//
// Java-only parcelables (no fields, no native header) are NOT considered
// forward-declared: they generate empty Go structs.
func (r *TypeRefResolver) isForwardDeclared(qualifiedName string) bool {
	if r.Registry == nil {
		return false
	}
	def, ok := r.Registry.Lookup(qualifiedName)
	if !ok {
		return false
	}
	parcDecl, ok := def.(*parser.ParcelableDecl)
	if !ok {
		return false
	}
	if len(parcDecl.Fields) > 0 || len(parcDecl.Constants) > 0 || len(parcDecl.NestedTypes) > 0 {
		return false
	}
	return parcDecl.CppHeader != "" || parcDecl.NdkHeader != "" || parcDecl.RustType != ""
}

// isUnresolvableType returns true if the given AIDL type name would resolve
// to any because it's unknown or forward-declared. This check does
// not add any imports as a side effect.
func (r *TypeRefResolver) isUnresolvableType(aidlName string) bool {
	if r == nil || r.Registry == nil {
		return false
	}

	// Skip primitives and known types.
	if _, ok := aidlPrimitiveToGo[aidlName]; ok {
		return false
	}
	if aidlName == "List" || aidlName == "Map" {
		return false
	}

	// Check if the type can be resolved via the registry.
	_, found := r.Registry.Lookup(aidlName)
	var qualifiedName string
	switch {
	case found:
		qualifiedName = aidlName
	case r.CurrentPkg != "":
		candidate := r.CurrentPkg + "." + aidlName
		if _, ok := r.Registry.Lookup(candidate); ok {
			qualifiedName = candidate
		}
	}
	if qualifiedName == "" {
		if qn, _, ok := r.Registry.LookupQualifiedByShortName(aidlName); ok {
			qualifiedName = qn
		}
	}

	if qualifiedName == "" {
		// Type not found in registry at all.
		return true
	}

	// Check if the resolved type is forward-declared.
	return r.isForwardDeclared(qualifiedName)
}

// isGoInterfaceType returns true if the AIDL type name corresponds to a
// Go interface type — either an AIDL interface definition in the registry,
// or the built-in IBinder type (which is a Go interface but not an AIDL
// interface definition).
func (r *TypeRefResolver) isGoInterfaceType(aidlName string) bool {
	if aidlName == "IBinder" {
		return true
	}
	return r.isInterfaceType(aidlName)
}

// resolveQualifiedName resolves an AIDL type name to a fully qualified
// registry key. It tries direct lookup first, then prepends the current
// package for unqualified names. Returns "" if not found.
func (r *TypeRefResolver) resolveQualifiedName(aidlName string) string {
	if _, ok := r.Registry.Lookup(aidlName); ok {
		return aidlName
	}

	// Only prepend the current package for unqualified names.
	// A dotted name like "android.os.IRemoteCallback" is already
	// fully qualified; prepending CurrentPkg would produce a wrong
	// key like "android.location.android.os.IRemoteCallback".
	if r.CurrentPkg != "" && !strings.Contains(aidlName, ".") {
		candidate := r.CurrentPkg + "." + aidlName
		if _, ok := r.Registry.Lookup(candidate); ok {
			return candidate
		}
	}

	return ""
}

// isInterfaceType returns true if the AIDL type resolves to an interface.
func (r *TypeRefResolver) isInterfaceType(aidlName string) bool {
	if r.Registry == nil {
		return false
	}
	// Try to resolve the type name via sequential fallback.
	qn := r.resolveQualifiedName(aidlName)
	if qn == "" {
		if resolved, _, ok := r.Registry.LookupQualifiedByShortName(aidlName); ok {
			qn = resolved
		}
	}
	if qn == "" {
		return false
	}
	def, ok := r.Registry.Lookup(qn)
	if !ok {
		return false
	}
	_, isIface := def.(*parser.InterfaceDecl)
	return isIface
}



// IsCycleBroken returns true if the given AIDL type name was resolved
// to any due to an import cycle.
func (r *TypeRefResolver) IsCycleBroken(aidlName string) bool {
	if r == nil || r.ImportGraph == nil {
		return false
	}

	// Resolve the type to its package, then check if importing it
	// from the current package would cause a cycle.
	targetPkg := r.resolveTypePkg(aidlName)
	if targetPkg == "" || targetPkg == r.CurrentPkg {
		return false
	}

	return r.ImportGraph.WouldCauseCycle(r.CurrentPkg, targetPkg)
}

// resolveTypePkg resolves an AIDL type name to its package.
func (r *TypeRefResolver) resolveTypePkg(aidlName string) string {
	if r.Registry == nil {
		return ""
	}

	// Try fully qualified lookup.
	if def, ok := r.Registry.Lookup(aidlName); ok {
		return packageFromDef(aidlName, def.GetName())
	}

	// Try current package + name.
	if r.CurrentPkg != "" {
		candidate := r.CurrentPkg + "." + aidlName
		if def, ok := r.Registry.Lookup(candidate); ok {
			return packageFromDef(candidate, def.GetName())
		}
	}

	// Try short name lookup.
	if qualifiedName, _, ok := r.Registry.LookupQualifiedByShortName(aidlName); ok {
		if def, ok := r.Registry.Lookup(qualifiedName); ok {
			return packageFromDef(qualifiedName, def.GetName())
		}
	}

	return ""
}

// splitQualifiedName splits a fully qualified AIDL name into its package and
// Go type name by looking up the definition in the registry to determine the
// correct boundary between package and type name.
func (r *TypeRefResolver) splitQualifiedName(qualifiedName string) (string, string) {
	if def, ok := r.Registry.Lookup(qualifiedName); ok {
		defName := def.GetName()
		pkg := packageFromDef(qualifiedName, defName)
		return pkg, aidlDottedNameToGo(defName)
	}

	// Fallback: split at the last dot.
	pkg := packageFromQualified(qualifiedName)
	typePart := qualifiedName
	if pkg != "" {
		typePart = qualifiedName[len(pkg)+1:]
	}
	return pkg, aidlDottedNameToGo(typePart)
}

// ensureImport adds the import for the given AIDL package to the GoFile
// and returns the alias to use for qualifying types from that package.
func (r *TypeRefResolver) ensureImport(aidlPkg string) string {
	if alias, ok := r.AliasMap[aidlPkg]; ok {
		return alias
	}

	goImportPath := goModulePath + "/" + AIDLToGoPackage(aidlPkg)
	alias := r.pickAlias(aidlPkg, goImportPath)

	r.AliasMap[aidlPkg] = alias
	r.UsedAliases[alias] = true
	r.GoFile.AddImport(goImportPath, alias)

	return alias
}

// pickAlias generates a unique import alias for the given AIDL package.
// It uses the last segment of the package name, disambiguating with
// progressively more segments if needed. It also avoids collisions with
// type names defined in the current AIDL package (which would make the
// import alias shadow the local type declaration).
func (r *TypeRefResolver) pickAlias(
	aidlPkg string,
	goImportPath string,
) string {
	segments := strings.Split(aidlPkg, ".")

	// Try last segment first.
	candidate := segments[len(segments)-1]
	if r.isValidAlias(candidate) {
		return candidate
	}

	// Build progressively longer aliases from the right.
	// "android.media.audio.common" -> "audioCommon", "mediaAudioCommon", etc.
	for i := len(segments) - 2; i >= 0; i-- {
		prefix := segments[i]
		candidate = prefix + strings.ToUpper(candidate[:1]) + candidate[1:]
		if r.isValidAlias(candidate) {
			return candidate
		}
	}

	// Last resort: use full path-derived alias.
	candidate = strings.ReplaceAll(aidlPkg, ".", "_")
	if r.isValidAlias(candidate) {
		return candidate
	}

	// Add suffix to avoid collision.
	return candidate + "Pkg"
}

// ReserveNames marks identifiers that must not be used as import aliases.
// Call this before resolving any types to prevent aliases from colliding
// with method parameter names in the generated file.
func (r *TypeRefResolver) ReserveNames(names []string) {
	for _, name := range names {
		r.ReservedNames[name] = true
	}
}

// isValidAlias checks that a candidate import alias does not collide with
// the current Go package name, an already-used alias, a reserved identifier
// (e.g. a method parameter name), or a type name defined in the current
// AIDL package.
func (r *TypeRefResolver) isValidAlias(candidate string) bool {
	if r.UsedAliases[candidate] || candidate == r.GoFile.Pkg {
		return false
	}
	if r.ReservedNames[candidate] {
		return false
	}
	return !r.collidesWithLocalType(candidate)
}

// collidesWithLocalType returns true if the candidate import alias would
// collide with a Go type name defined in the current AIDL package.
// For example, if the current package defines a type "RefreshRateRanges",
// using "RefreshRateRanges" as an import alias would cause a compilation error.
func (r *TypeRefResolver) collidesWithLocalType(candidate string) bool {
	if r.Registry == nil || r.CurrentPkg == "" {
		return false
	}

	allDefs := r.Registry.All()
	prefix := r.CurrentPkg + "."
	for qualifiedName, def := range allDefs {
		if !strings.HasPrefix(qualifiedName, prefix) {
			continue
		}

		// Only check definitions that are direct children of the current
		// package (not deeply nested ones from sub-packages).
		defName := def.GetName()
		pkg := packageFromDef(qualifiedName, defName)
		if pkg != r.CurrentPkg {
			continue
		}

		goName := AIDLToGoName(defName)
		if goName == candidate {
			return true
		}
	}

	return false
}
