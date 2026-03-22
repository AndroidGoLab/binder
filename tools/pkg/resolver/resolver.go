package resolver

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/AndroidGoLab/binder/tools/pkg/parser"
)

// Resolver resolves AIDL imports and builds a TypeRegistry.
//
// Resolver is NOT safe for concurrent use. While the underlying TypeRegistry
// is mutex-protected, the Resolver's own resolved map has no synchronization.
// Callers must ensure that only one goroutine calls Resolver methods at a time.
type Resolver struct {
	SearchPaths    []string
	Registry       *TypeRegistry
	Resolved       map[string]bool
	SkipUnresolved bool
}

// New creates a Resolver that searches the given paths for AIDL files.
func New(
	searchPaths []string,
) *Resolver {
	return &Resolver{
		SearchPaths: searchPaths,
		Registry:    NewTypeRegistry(),
		Resolved:    make(map[string]bool),
	}
}

// SetSkipUnresolved configures whether the resolver silently skips
// imports that cannot be found in any search path. When true, missing
// imports are ignored instead of causing an error.
func (r *Resolver) SetSkipUnresolved(
	skip bool,
) {
	r.SkipUnresolved = skip
}

// ResolveFile parses an AIDL file and transitively resolves all its imports.
func (r *Resolver) ResolveFile(
	filename string,
) (_err error) {
	absPath, err := filepath.Abs(filename)
	if err != nil {
		return fmt.Errorf("resolver: abs path: %w", err)
	}

	if r.Resolved[absPath] {
		return nil
	}
	r.Resolved[absPath] = true

	doc, err := parser.ParseFile(filename)
	if err != nil {
		return fmt.Errorf("resolver: parsing %s: %w", filename, err)
	}

	return r.resolveDocument(doc, filename)
}

// ResolveDocument registers definitions from an already-parsed document
// and transitively resolves all its imports.
func (r *Resolver) ResolveDocument(
	doc *parser.Document,
	filename string,
) (_err error) {
	absPath, err := filepath.Abs(filename)
	if err != nil {
		return fmt.Errorf("resolver: abs path: %w", err)
	}

	if r.Resolved[absPath] {
		return nil
	}
	r.Resolved[absPath] = true

	return r.resolveDocument(doc, filename)
}

func (r *Resolver) resolveDocument(
	doc *parser.Document,
	filename string,
) error {
	pkg := ""
	if doc.Package != nil {
		pkg = doc.Package.Name
	}

	for _, def := range doc.Definitions {
		qualifiedName := def.GetName()
		if pkg != "" {
			qualifiedName = pkg + "." + def.GetName()
		}
		r.Registry.Register(qualifiedName, def)
		r.registerNestedTypes(qualifiedName, def)
	}

	for _, imp := range doc.Imports {
		if err := r.resolveImport(imp.Name); err != nil {
			return fmt.Errorf("resolver: resolving import %q from %s: %w", imp.Name, filename, err)
		}
	}

	return nil
}

// registerNestedTypes recursively registers nested type definitions
// with dot-separated qualified names (e.g. "pkg.Outer.Inner").
func (r *Resolver) registerNestedTypes(
	parentName string,
	def parser.Definition,
) {
	var nested []parser.Definition
	switch d := def.(type) {
	case *parser.ParcelableDecl:
		nested = d.NestedTypes
	case *parser.InterfaceDecl:
		nested = d.NestedTypes
	case *parser.UnionDecl:
		nested = d.NestedTypes
	}

	for _, nd := range nested {
		qualifiedName := parentName + "." + nd.GetName()
		r.Registry.Register(qualifiedName, nd)
		r.registerNestedTypes(qualifiedName, nd)
	}
}

// resolveImport finds and resolves an AIDL file by its qualified name.
func (r *Resolver) resolveImport(
	qualifiedName string,
) error {
	if _, ok := r.Registry.Lookup(qualifiedName); ok {
		return nil
	}

	// Convert qualified name to file path: android.os.IServiceManager -> android/os/IServiceManager.aidl
	relPath := strings.ReplaceAll(qualifiedName, ".", "/") + ".aidl"

	for _, searchPath := range r.SearchPaths {
		fullPath := filepath.Join(searchPath, relPath)
		if _, err := os.Stat(fullPath); err == nil {
			absPath, _ := filepath.Abs(fullPath)
			if r.Resolved[absPath] {
				return nil // already being resolved (circular import)
			}
			return r.ResolveFile(fullPath)
		}
	}

	if r.SkipUnresolved {
		return nil
	}
	return fmt.Errorf("cannot find AIDL file for %q in search paths", qualifiedName)
}
