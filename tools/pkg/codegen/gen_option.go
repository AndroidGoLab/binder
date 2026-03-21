package codegen

import "github.com/xaionaro-go/binder/tools/pkg/resolver"

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
// cycles are replaced with any to break the cycle.
func WithImportGraph(
	graph *ImportGraph,
) GenOption {
	return func(opts *GenOptions) {
		opts.ImportGraph = graph
	}
}

// WithCycleTypeCallback sets a callback for types redirected to sub-packages.
func WithCycleTypeCallback(
	cb func(qualifiedName, typesPkg string),
) GenOption {
	return func(opts *GenOptions) {
		opts.CycleTypeCallback = cb
	}
}

func applyGenOptions(options []GenOption) GenOptions {
	var opts GenOptions
	for _, o := range options {
		o(&opts)
	}
	return opts
}
