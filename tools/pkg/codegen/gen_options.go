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
}

// GenOption configures code generation.
type GenOption func(*GenOptions)

// WithRegistry sets the type registry for looking up type definitions
// during code generation. This enables correct marshaling of enum types
// (as integers) and interface types (as IBinder).
func WithRegistry(
	registry *resolver.TypeRegistry,
) GenOption {
	return func(opts *GenOptions) {
		opts.Registry = registry
	}
}

// WithCurrentPkg sets the AIDL package of the type being generated.
// This enables cross-package type references to be qualified correctly.
func WithCurrentPkg(
	pkg string,
) GenOption {
	return func(opts *GenOptions) {
		opts.CurrentPkg = pkg
	}
}

// WithImportGraph sets the import graph for cycle detection.
// When set, cross-package type references that would create import
// cycles are replaced with interface{} to break the cycle.
func WithImportGraph(
	graph *ImportGraph,
) GenOption {
	return func(opts *GenOptions) {
		opts.ImportGraph = graph
	}
}

func applyGenOptions(options []GenOption) GenOptions {
	var opts GenOptions
	for _, o := range options {
		o(&opts)
	}
	return opts
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
// checking if the type was cycle-broken or resolved to interface{}.
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
		// If the type would resolve to interface{} (unknown, forward-declared,
		// or cycle-broken), it cannot be marshaled/unmarshaled.
		if typeRef.isUnresolvableType(ts.Name) {
			return opaqueTypeMarshalInfo
		}
		// Additional check: see what the type actually resolves to in Go.
		// If it resolves to interface{}, skip marshaling regardless of
		// why it was resolved that way.
		resolved := typeRef.GoTypeRef(ts)
		if isOpaqueGoType(resolved) {
			return opaqueTypeMarshalInfo
		}
	}
	return marshalForType(ts, opts)
}

// isOpaqueGoType returns true if the Go type string indicates an opaque
// type that cannot be marshaled (e.g. interface{}, *interface{}).
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
	return t == "interface{}"
}

// opaqueTypeMarshalInfo is the marshal info for types that were resolved
// to interface{} due to import cycles. These types cannot be marshaled/
// unmarshaled directly, so the expressions are empty (causing the
// generators to skip these fields).
var opaqueTypeMarshalInfo = MarshalInfo{}

// newTypeRefResolver creates a TypeRefResolver from the options and a GoFile.
// If no registry is available, returns nil.
func (opts GenOptions) newTypeRefResolver(goFile *GoFile) *TypeRefResolver {
	if opts.Registry == nil {
		return nil
	}
	r := NewTypeRefResolver(opts.Registry, opts.CurrentPkg, goFile)
	r.ImportGraph = opts.ImportGraph
	return r
}
