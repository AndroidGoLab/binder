package codegen

import (
	"github.com/xaionaro-go/binder/tools/pkg/parser"
	"github.com/xaionaro-go/binder/tools/pkg/resolver"
)

// GenOptions holds optional parameters for code generation functions.
type GenOptions struct {
	Registry    *resolver.TypeRegistry
	CurrentPkg  string // AIDL package of the type being generated
	ImportGraph *ImportGraph
	// CycleTypeCallback is called when a type is redirected to a "types"
	// sub-package to break an import cycle. The callback receives the
	// qualified AIDL name and the types sub-package AIDL name.
	CycleTypeCallback func(qualifiedName, typesPkg string)
}

// marshalForType returns the MarshalInfo for a type, using the registry
// from options if available.
func marshalForType(
	ts *parser.TypeSpecifier,
	opts GenOptions,
) MarshalInfo {
	return MarshalForTypeWithRegistry(ts, opts.Registry, opts.CurrentPkg)
}

// marshalForTypeWithCycleCheck returns the MarshalInfo for a type,
// checking if the type was cycle-broken or resolved to any.
// Such types cannot be marshaled/unmarshaled, so the expressions are empty.
func marshalForTypeWithCycleCheck(
	ts *parser.TypeSpecifier,
	opts GenOptions,
	typeRef *TypeRefResolver,
) MarshalInfo {
	if typeRef != nil {
		if typeRef.IsCycleBroken(ts.Name) {
			return opaqueTypeMarshalInfo
		}
		// If the type would resolve to any (unknown, forward-declared,
		// or cycle-broken), it cannot be marshaled/unmarshaled.
		if typeRef.isUnresolvableType(ts.Name) {
			return opaqueTypeMarshalInfo
		}
		// Additional check: see what the type actually resolves to in Go.
		// If it resolves to any, skip marshaling regardless of
		// why it was resolved that way.
		resolved := typeRef.GoTypeRef(ts)
		if isOpaqueGoType(resolved) {
			return opaqueTypeMarshalInfo
		}
	}
	return marshalForType(ts, opts)
}

// isOpaqueGoType returns true if the Go type string indicates an opaque
// type that cannot be marshaled (e.g. any, *any).
func isOpaqueGoType(goType string) bool {
	// Strip pointer and slice prefixes.
	t := goType
	for len(t) > 0 && (t[0] == '*' || t[0] == '[') {
		if t[0] == '[' {
			t = t[1:]
			if len(t) > 0 && t[0] == ']' {
				t = t[1:]
			}
			continue
		}
		t = t[1:]
	}
	return t == "any"
}

// opaqueTypeMarshalInfo is the marshal info for types that were resolved
// to any due to import cycles. These types cannot be marshaled/
// unmarshaled directly, so the expressions are empty (causing the
// generators to skip these fields).
var opaqueTypeMarshalInfo = MarshalInfo{}

// recursiveFields returns the set of struct fields that need pointer types
// to break recursive type definitions within the same package.
func (opts GenOptions) recursiveFields() *recursiveFieldSet {
	return detectRecursiveTypes(opts.Registry, opts.CurrentPkg)
}

// newTypeRefResolver creates a TypeRefResolver from the options and a GoFile.
// If no registry is available, returns nil.
func (opts GenOptions) newTypeRefResolver(goFile *GoFile) *TypeRefResolver {
	if opts.Registry == nil {
		return nil
	}
	r := NewTypeRefResolver(opts.Registry, opts.CurrentPkg, goFile)
	r.ImportGraph = opts.ImportGraph
	r.CycleTypeCallback = opts.CycleTypeCallback
	return r
}
