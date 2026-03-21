package codegen

import (
	"strings"
	"unicode"

	"github.com/xaionaro-go/binder/tools/pkg/parser"
)

// aidlPrimitiveToGo maps AIDL primitive type names to Go type strings.
var aidlPrimitiveToGo = map[string]string{
	"void":                 "",
	"boolean":              "bool",
	"byte":                 "byte",
	"char":                 "uint16",
	"int":                  "int32",
	"long":                 "int64",
	"float":                "float32",
	"double":               "float64",
	"String":               "string",
	"CharSequence":         "string",
	"IBinder":              "binder.IBinder",
	"ParcelFileDescriptor": "int32",
	"FileDescriptor":       "int32",
}

// AIDLToGoName converts an AIDL identifier to a Go-exported name.
// camelCase -> PascalCase (e.g., "getService" -> "GetService")
// SCREAMING_SNAKE -> PascalCase (e.g., "FIRST_CALL_TRANSACTION" -> "FirstCallTransaction")
// Dotted names (nested types) -> flattened PascalCase (e.g., "Foo.Bar" -> "FooBar")
// already PascalCase -> kept as-is
func AIDLToGoName(name string) string {
	if name == "" {
		return ""
	}

	// Handle dotted names (nested types like "ParentType.NestedType").
	if strings.Contains(name, ".") {
		return aidlDottedNameToGo(name)
	}

	if isScreamingSnake(name) {
		return ScreamingSnakeToPascal(name)
	}

	// camelCase or PascalCase: ensure first letter is uppercase.
	runes := []rune(name)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

// AIDLToGoPackage converts an AIDL package name to a Go package path segment.
// "android.os" -> "android/os"
// "com.android.internal.foo" -> "com/android/internal_/foo"
//
// The "internal" segment is renamed to "internal_" because Go restricts
// imports of packages under "internal/" directories to their parent tree.
func AIDLToGoPackage(pkg string) string {
	segments := strings.Split(pkg, ".")
	for i, seg := range segments {
		if seg == "internal" {
			segments[i] = "internal_"
		}
	}
	return strings.Join(segments, "/")
}

// AIDLToGoFileName converts an AIDL type name to a Go file name.
// "IServiceManager" -> "iservicemanager.go"
func AIDLToGoFileName(name string) string {
	return strings.ToLower(name) + ".go"
}

// AIDLTypeToGo converts an AIDL TypeSpecifier to a Go type string.
// Handles primitives, String, IBinder, List<T>, Map<K,V>, arrays, @nullable.
func AIDLTypeToGo(ts *parser.TypeSpecifier) string {
	if ts == nil {
		return ""
	}

	goType := aidlTypeToGoInner(ts)

	if hasAnnotation(ts.Annots, "nullable") && goType != "" && goType != "string" {
		if goType[0] != '*' && goType[0] != '[' && !strings.HasPrefix(goType, "map[") {
			goType = "*" + goType
		}
	}

	return goType
}

// aidlTypeToGoInner converts an AIDL TypeSpecifier without handling @nullable.
func aidlTypeToGoInner(ts *parser.TypeSpecifier) string {
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
			elem = AIDLTypeToGo(ts.TypeArgs[0])
		}
		return "[]" + elem

	case "Map":
		key := "any"
		val := "any"
		if len(ts.TypeArgs) >= 2 {
			key = AIDLTypeToGo(ts.TypeArgs[0])
			val = AIDLTypeToGo(ts.TypeArgs[1])
		}
		return "map[" + key + "]" + val
	}

	// User-defined or unrecognized type. Dotted names (e.g.,
	// "ActivityManager.RunningTaskInfo") represent nested types in AIDL;
	// flatten them by converting each segment to PascalCase and joining.
	goName := aidlDottedNameToGo(ts.Name)
	if ts.IsArray {
		return "[]" + goName
	}
	return goName
}

// aidlDottedNameToGo converts a potentially dotted AIDL type name to a Go identifier.
// Dotted names like "ActivityManager.RunningTaskInfo" represent nested types and are
// flattened by converting each segment to PascalCase: "ActivityManagerRunningTaskInfo".
// Non-dotted names are handled as before via AIDLToGoName.
func aidlDottedNameToGo(name string) string {
	if !strings.Contains(name, ".") {
		return AIDLToGoName(name)
	}

	segments := strings.Split(name, ".")
	var b strings.Builder
	for _, seg := range segments {
		b.WriteString(AIDLToGoName(seg))
	}
	return b.String()
}

// isScreamingSnake returns true if the name consists only of uppercase letters,
// digits, and underscores, and contains at least one underscore.
func isScreamingSnake(name string) bool {
	hasUnderscore := false
	for _, r := range name {
		switch {
		case r == '_':
			hasUnderscore = true
		case unicode.IsUpper(r) || unicode.IsDigit(r):
			// ok
		default:
			return false
		}
	}
	return hasUnderscore
}

// ScreamingSnakeToPascal converts "FIRST_CALL_TRANSACTION" to "FirstCallTransaction".
func ScreamingSnakeToPascal(name string) string {
	parts := strings.Split(name, "_")
	var b strings.Builder
	for _, part := range parts {
		if part == "" {
			continue
		}
		b.WriteString(strings.ToUpper(part[:1]))
		b.WriteString(strings.ToLower(part[1:]))
	}
	return b.String()
}

// aidlIntSuffixes lists AIDL typed integer suffixes to strip during codegen.
// These include unsigned (u8, u16, u32, u64), signed (i8, i16, i32, i64),
// and Java-style long (L, l) suffixes.
var aidlIntSuffixes = []string{
	"u64", "u32", "u16", "u8",
	"i64", "i32", "i16", "i8",
	"L", "l",
}

// stripAIDLIntSuffix removes AIDL typed integer suffixes (e.g., u8, u32, L)
// from an integer literal value so it is valid Go.
func stripAIDLIntSuffix(value string) string {
	for _, suffix := range aidlIntSuffixes {
		if strings.HasSuffix(value, suffix) {
			return value[:len(value)-len(suffix)]
		}
	}
	return value
}

// stripAIDLFloatSuffix removes Java/AIDL float suffixes (f, F, d, D) from
// a floating-point literal value so it is valid Go.
func stripAIDLFloatSuffix(value string) string {
	if len(value) == 0 {
		return value
	}
	last := value[len(value)-1]
	if last == 'f' || last == 'F' || last == 'd' || last == 'D' {
		stripped := value[:len(value)-1]
		// Ensure the result is still a valid number (not just "0" without ".").
		// "0f" -> "0.0", not just "0".
		if !strings.Contains(stripped, ".") && !strings.ContainsAny(stripped, "eE") {
			stripped += ".0"
		}
		return stripped
	}
	return value
}

// hasAnnotation checks if the annotation list contains an annotation with the given name.
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

