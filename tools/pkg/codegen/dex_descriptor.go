package codegen

import (
	"strings"

	"github.com/AndroidGoLab/binder/tools/pkg/parser"
	"github.com/AndroidGoLab/binder/tools/pkg/resolver"
)

// aidlPrimitiveToDex maps AIDL primitive type names to their DEX
// type descriptors. These are the same as Java primitive descriptors.
var aidlPrimitiveToDex = map[string]string{
	"boolean": "Z",
	"byte":    "B",
	"char":    "C",
	"short":   "S",
	"int":     "I",
	"long":    "J",
	"float":   "F",
	"double":  "D",
	"void":    "V",
}

// aidlKnownToDex maps well-known AIDL types (that don't need registry
// lookup) to their DEX type descriptors.
var aidlKnownToDex = map[string]string{
	"String":               "Ljava/lang/String;",
	"CharSequence":         "Ljava/lang/CharSequence;",
	"IBinder":              "Landroid/os/IBinder;",
	"ParcelFileDescriptor": "Landroid/os/ParcelFileDescriptor;",
	"FileDescriptor":       "Ljava/io/FileDescriptor;",
}

// AIDLTypeToDexDescriptor converts an AIDL type specifier to its DEX
// type descriptor string. Uses the type registry to resolve short names
// to fully qualified names.
//
// Returns empty string if the type cannot be resolved.
func AIDLTypeToDexDescriptor(
	ts *parser.TypeSpecifier,
	registry *resolver.TypeRegistry,
	currentPkg string,
) string {
	if ts == nil {
		return ""
	}

	name := ts.Name

	// Array types: prepend "[" to the element descriptor.
	if ts.IsArray {
		elemTS := &parser.TypeSpecifier{
			Name:     ts.Name,
			TypeArgs: ts.TypeArgs,
		}
		elemDesc := AIDLTypeToDexDescriptor(elemTS, registry, currentPkg)
		if elemDesc == "" {
			return ""
		}
		return "[" + elemDesc
	}

	// List<T> maps to Ljava/util/List; in DEX (generic erasure).
	if name == "List" {
		return "Ljava/util/List;"
	}

	// Map<K,V> maps to Ljava/util/Map;.
	if name == "Map" {
		return "Ljava/util/Map;"
	}

	// Primitives.
	if desc, ok := aidlPrimitiveToDex[name]; ok {
		return desc
	}

	// Well-known types.
	if desc, ok := aidlKnownToDex[name]; ok {
		return desc
	}

	// Try to resolve via the type registry.
	qualifiedName := resolveQualifiedName(name, registry, currentPkg)
	if qualifiedName == "" {
		return ""
	}

	// Convert dot-separated qualified name to DEX descriptor:
	// "android.content.AttributionSource" → "Landroid/content/AttributionSource;"
	return "L" + strings.ReplaceAll(qualifiedName, ".", "/") + ";"
}

// resolveQualifiedName resolves a type name to its fully qualified
// AIDL name. If the name already contains dots, it is assumed to be
// fully qualified. Otherwise, the registry is consulted.
func resolveQualifiedName(
	name string,
	registry *resolver.TypeRegistry,
	currentPkg string,
) string {
	// Already fully qualified.
	if strings.Contains(name, ".") {
		return name
	}

	if registry == nil {
		return ""
	}

	// Try current package first: "android.bluetooth" + "." + "IBluetoothGattCallback".
	if currentPkg != "" {
		candidate := currentPkg + "." + name
		if _, ok := registry.Lookup(candidate); ok {
			return candidate
		}
	}

	// Fall back to short-name lookup.
	qualifiedName, _, ok := registry.LookupQualifiedByShortName(name)
	if ok {
		return qualifiedName
	}

	return ""
}
