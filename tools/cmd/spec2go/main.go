// Command spec2go generates Go source code from YAML spec files.
//
// It reads spec files produced by aidl2spec, converts them back to the
// parser AST types, and feeds them through the existing codegen.Generator
// to produce identical Go output without requiring AIDL source files.
//
// Usage:
//
//	spec2go -specs specs/ -output . [-smoke-tests] [-codes-output binder/versionaware/codes_gen.go] [-default-api 36]
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

	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/tools/pkg/codegen"
	"github.com/AndroidGoLab/binder/tools/pkg/parser"
	"github.com/AndroidGoLab/binder/tools/pkg/resolver"
	"github.com/AndroidGoLab/binder/tools/pkg/spec"
)

func main() {
	specsDir := flag.String("specs", "specs/", "Directory containing spec YAML files")
	outputDir := flag.String("output", ".", "Output directory for generated Go files")
	nativeImplsDir := flag.String("native-impls", "", "Directory containing hand-written native parcelable implementations")
	smokeTests := flag.Bool("smoke-tests", false, "Generate smoke tests")
	codesOutput := flag.String("codes-output", "", "Output path for codes_gen.go")
	defaultAPI := flag.Int("default-api", 36, "Default API level")
	flag.Parse()

	if err := run(
		*specsDir,
		*outputDir,
		*nativeImplsDir,
		*smokeTests,
		*codesOutput,
		*defaultAPI,
	); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(
	specsDir string,
	outputDir string,
	nativeImplsDir string,
	smokeTests bool,
	codesOutput string,
	defaultAPI int,
) error {
	fmt.Fprintf(os.Stderr, "Reading specs from %s...\n", specsDir)
	specs, err := spec.ReadAllSpecs(specsDir)
	if err != nil {
		return fmt.Errorf("reading specs: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Read %d package specs\n", len(specs))

	// Build an index of parcelable short names → AIDL qualified names
	// so that typed_object fields can be resolved to concrete Go types.
	parcelableIndex := buildParcelableIndex(specs)

	// Convert specs to parser AST and register in a type registry.
	registry := resolver.NewTypeRegistry()
	for _, ps := range specs {
		registerPackageSpec(registry, ps, parcelableIndex)
	}

	allDefs := registry.All()
	fmt.Fprintf(os.Stderr, "Registered %d definitions\n", len(allDefs))

	// Create a resolver wrapping the pre-populated registry.
	r := newRegistryResolver(registry)

	// Generate Go code.
	fmt.Fprintf(os.Stderr, "Generating Go code into %s...\n", outputDir)
	gen := codegen.NewGenerator(r, outputDir)
	gen.NativeImplsDir = nativeImplsDir
	gen.SetSkipErrors(true)
	if err := gen.GenerateAll(); err != nil {
		// errors.Join returns an error implementing Unwrap() []error.
		// Use comma-ok to avoid panicking if the error type changes.
		multi, ok := err.(interface{ Unwrap() []error })
		if !ok {
			return fmt.Errorf("codegen failed: %w", err)
		}
		joinedErrs := multi.Unwrap()
		fmt.Fprintf(os.Stderr, "Codegen completed with %d definition errors (skipped)\n", len(joinedErrs))
		for _, e := range joinedErrs {
			fmt.Fprintf(os.Stderr, "  error: %v\n", e)
		}
		return fmt.Errorf("codegen produced %d definition errors", len(joinedErrs))
	}

	if smokeTests {
		fmt.Fprintf(os.Stderr, "Generating smoke tests...\n")
		if err := gen.GenerateAllSmokeTests(); err != nil {
			smokeErrors := strings.Split(err.Error(), "\n")
			fmt.Fprintf(os.Stderr, "Smoke test generation completed with %d errors (skipped)\n", len(smokeErrors))
		}
	}

	// Generate service name constants and accessor files from the
	// servicemanager spec, if present.
	if err := generateServiceNamesFile(specs, outputDir); err != nil {
		return fmt.Errorf("generating service names: %w", err)
	}
	if err := generateAccessorFiles(specs, outputDir); err != nil {
		return fmt.Errorf("generating accessor files: %w", err)
	}

	if err := generateJavaConstantsFiles(specs, outputDir); err != nil {
		return fmt.Errorf("generating java constants: %w", err)
	}

	genCount, err := countGoFiles(outputDir)
	if err != nil {
		return fmt.Errorf("counting Go files: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Generated %d Go files\n", genCount)

	// Generate codes_gen.go from version_codes in interface specs.
	if codesOutput == "" {
		return nil
	}

	return generateCodesFile(specs, codesOutput, defaultAPI)
}

// parcelableIndex maps a short parcelable name (e.g., "WorkSource") to its
// AIDL qualified name (e.g., "android.os.WorkSource"). When multiple
// parcelables share the same short name, none is stored (ambiguous).
type parcelableIndex map[string]string

// buildParcelableIndex scans all specs and builds a short name → qualified
// name index for parcelables that have a JavaWireFormat (i.e., are concrete
// types with marshal/unmarshal support).
func buildParcelableIndex(
	specs map[string]*spec.PackageSpec,
) parcelableIndex {
	idx := parcelableIndex{}
	for _, ps := range specs {
		for _, parc := range ps.Parcelables {
			shortName := parc.Name
			qualifiedName := ps.AIDLPackage + "." + parc.Name
			if existing, ok := idx[shortName]; ok {
				if existing != qualifiedName {
					// Ambiguous: multiple parcelables with same short name.
					idx[shortName] = ""
				}
			} else {
				idx[shortName] = qualifiedName
			}
		}
	}
	return idx
}

// resolveDelegateName tries common name transformations to find a
// parcelable matching the delegate field name. For example, a field
// named "Locales" may correspond to the parcelable "LocaleList".
func resolveDelegateName(fieldName string, parcIdx parcelableIndex) string {
	// Try: strip trailing 's' (plurals) and append "List".
	// "Locales" → "Locale" + "List" → "LocaleList"
	if len(fieldName) > 1 && fieldName[len(fieldName)-1] == 's' {
		candidate := fieldName[:len(fieldName)-1] + "List"
		if q := parcIdx[candidate]; q != "" {
			return q
		}
	}
	// Try: append "List" directly.
	// "Foo" → "FooList"
	if q := parcIdx[fieldName+"List"]; q != "" {
		return q
	}
	// Try: append "Handle" — e.g. "User" → "UserHandle".
	if q := parcIdx[fieldName+"Handle"]; q != "" {
		return q
	}
	return ""
}

// delegateFieldName extracts a clean field name from a java_wire_format
// delegate name. Names like "This.notification" become "Notification".
// Names without a "This." prefix are returned as-is.
func delegateFieldName(name string) string {
	if strings.HasPrefix(name, "This.") {
		suffix := name[len("This."):]
		if len(suffix) > 0 {
			return strings.ToUpper(suffix[:1]) + suffix[1:]
		}
	}
	return name
}

// isNullCheckCondition returns true if the condition is a Java-style null
// check like "this.tag!=null", "this.mInstanceId!=null", or "mFoo!=null".
// Both "this."-prefixed and bare field references are accepted since
// different Java classes use different conventions.
func isNullCheckCondition(cond string) bool {
	return strings.HasSuffix(cond, "!=null")
}

// mergeNullFlagPairs preprocesses JavaWireFormat fields to merge the
// "flag + conditional data" pattern used for nullable fields in Java
// writeToParcel(). The pattern is:
//
//	{ name: "1", write_method: int32, condition: "this.xxx!=null" }   ← flag
//	{ name: FieldName, write_method: ..., condition: "this.xxx!=null" } ← data
//
// This merges into a single field with condition "_null_flag", meaning
// the codegen should emit: read int32 flag, then conditionally read/write data.
func mergeNullFlagPairs(fields []spec.JavaWireField) []spec.JavaWireField {
	var result []spec.JavaWireField
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		// Detect flag field: numeric literal int32 with null-check condition.
		if isNumericLiteral(f.Name) && f.WriteMethod == "int32" &&
			isNullCheckCondition(f.Condition) &&
			i+1 < len(fields) && fields[i+1].Condition == f.Condition {
			// Merge: skip the flag field, mark the data field with _null_flag.
			i++
			merged := fields[i]
			merged.Condition = "_null_flag"
			result = append(result, merged)
			continue
		}
		result = append(result, f)
	}
	return result
}

// registerPackageSpec converts all definitions in a PackageSpec to parser
// AST types and registers them in the type registry.
func registerPackageSpec(
	registry *resolver.TypeRegistry,
	ps *spec.PackageSpec,
	parcIdx parcelableIndex,
) {
	for _, iface := range ps.Interfaces {
		qualifiedName := ps.AIDLPackage + "." + iface.Name
		def := convertInterfaceToAST(iface)
		registry.Register(qualifiedName, def)
	}

	for _, parc := range ps.Parcelables {
		qualifiedName := ps.AIDLPackage + "." + parc.Name
		def := convertParcelableToAST(parc, ps.AIDLPackage, parcIdx)
		registry.Register(qualifiedName, def)
	}

	for _, enum := range ps.Enums {
		qualifiedName := ps.AIDLPackage + "." + enum.Name
		def := convertEnumToAST(enum)
		registry.Register(qualifiedName, def)
	}

	for _, union := range ps.Unions {
		qualifiedName := ps.AIDLPackage + "." + union.Name
		def := convertUnionToAST(union)
		registry.Register(qualifiedName, def)
	}
}

// convertInterfaceToAST converts an InterfaceSpec back to a parser.InterfaceDecl.
func convertInterfaceToAST(
	iface spec.InterfaceSpec,
) *parser.InterfaceDecl {
	decl := &parser.InterfaceDecl{
		IntfName: iface.Name,
		Oneway:   iface.Oneway,
		Annots:   convertAnnotationNamesToAST(iface.Annotations),
	}

	for _, m := range iface.Methods {
		decl.Methods = append(decl.Methods, convertMethodToAST(m))
	}

	for _, c := range iface.Constants {
		decl.Constants = append(decl.Constants, convertConstantToAST(c))
	}

	return decl
}

// convertMethodToAST converts a MethodSpec back to a parser.MethodDecl.
func convertMethodToAST(
	m spec.MethodSpec,
) *parser.MethodDecl {
	decl := &parser.MethodDecl{
		MethodName: m.Name,
		Oneway:     m.Oneway,
		// Set TransactionID = offset + 1 so ComputeTransactionCodes produces
		// the correct offset. TransactionID is 1-based in the parser; 0 means
		// auto-assign. By making every method "explicit", the counter resets
		// correctly for each method.
		TransactionID: m.TransactionCodeOffset + 1,
		Annots:        convertAnnotationNamesToAST(m.Annotations),
	}

	if m.ReturnType.Name != "" {
		decl.ReturnType = convertTypeRefToAST(m.ReturnType)
	}

	for _, p := range m.Params {
		decl.Params = append(decl.Params, convertParamToAST(p))
	}

	return decl
}

// convertParamToAST converts a ParamSpec back to a parser.ParamDecl.
func convertParamToAST(
	p spec.ParamSpec,
) *parser.ParamDecl {
	decl := &parser.ParamDecl{
		ParamName:   p.Name,
		Type:        convertTypeRefToAST(p.Type),
		Annots:      convertAnnotationNamesToAST(p.Annotations),
		MinAPILevel: p.MinAPILevel,
	}

	switch p.Direction {
	case spec.DirectionIn:
		decl.Direction = parser.DirectionIn
	case spec.DirectionOut:
		decl.Direction = parser.DirectionOut
	case spec.DirectionInOut:
		decl.Direction = parser.DirectionInOut
	default:
		decl.Direction = parser.DirectionNone
	}

	return decl
}

// convertTypeRefToAST converts a TypeRef back to a parser.TypeSpecifier.
func convertTypeRefToAST(
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

	// Restore type-level annotations beyond @nullable.
	for _, name := range tr.Annotations {
		ts.Annots = append(ts.Annots, &parser.Annotation{Name: name})
	}

	for _, arg := range tr.TypeArgs {
		ts.TypeArgs = append(ts.TypeArgs, convertTypeRefToAST(arg))
	}

	return ts
}

// javaWireTypeToAIDL maps JavaWireField.WriteMethod values to the AIDL type
// name used by the parser/codegen (matching marshalPrimitiveMap keys).
var javaWireTypeToAIDL = map[string]string{
	"bool":     "boolean",
	"int32":    "int",
	"int64":    "long",
	"float32":  "float",
	"float64":  "double",
	"string8":  "String",  // UTF-8
	"string16": "String",  // UTF-16
}

// isValidJavaWireFieldName returns true if the name is a simple identifier
// suitable for use as a Go struct field (no dots, parens, or Java-isms).
// isNumericLiteral returns true if the string is a decimal integer literal
// (optionally negative), e.g., "0", "-1", "42".
func isNumericLiteral(s string) bool {
	if len(s) == 0 {
		return false
	}
	start := 0
	if s[0] == '-' {
		start = 1
	}
	if start >= len(s) {
		return false
	}
	for i := start; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func isValidJavaWireFieldName(name string) bool {
	if len(name) == 0 {
		return false
	}
	for i, r := range name {
		if i == 0 {
			if !unicode.IsLetter(r) && r != '_' {
				return false
			}
		} else {
			if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
				return false
			}
		}
	}
	return true
}

// isResolvableJavaWireCondition returns true if the condition is a simple
// "FieldName & number" bitmask pattern that can be translated to Go.
func isResolvableJavaWireCondition(condition string) bool {
	if condition == "" {
		return true
	}
	if condition == "_null_flag" {
		return true
	}
	parts := strings.SplitN(condition, " & ", 2)
	if len(parts) != 2 {
		return false
	}
	return isValidJavaWireFieldName(parts[0])
}

// convertParcelableToAST converts a ParcelableSpec back to a parser.ParcelableDecl.
// currentPkg is the AIDL package of the parcelable being converted.
// parcIdx maps short parcelable names to AIDL qualified names, used
// to resolve typed_object fields to concrete struct fields.
func convertParcelableToAST(
	parc spec.ParcelableSpec,
	currentPkg string,
	parcIdx parcelableIndex,
) *parser.ParcelableDecl {
	decl := &parser.ParcelableDecl{
		ParcName:         parc.Name,
		Annots:           convertAnnotationNamesToAST(parc.Annotations),
		NativeParcelable: parc.NativeParcelable,
	}

	for _, f := range parc.Fields {
		decl.Fields = append(decl.Fields, convertFieldToAST(f))
	}

	// When a bare AIDL parcelable has no fields but does have a Java wire
	// format extracted from writeToParcel(), synthesize struct fields from
	// the wire format and store the wire layout for custom marshal/unmarshal.
	if len(decl.Fields) == 0 && len(parc.JavaWireFormat) > 0 {
		var wireFields []parser.JavaWireField
		seenNames := map[string]int{} // track Go field names for dedup
		jwfList := mergeNullFlagPairs(parc.JavaWireFormat)
		for _, jwf := range jwfList {
			// Strip "This." prefix from field names to get clean Go
			// identifiers. Java writeToParcel often references fields
			// as "this.field" → spec records them as "This.field".
			cleanName := delegateFieldName(jwf.Name)
			if cleanName != jwf.Name {
				jwf.Name = cleanName
			}

			// Check if this field has a valid name and resolvable condition.
			validName := isValidJavaWireFieldName(jwf.Name)
			resolvableCond := isResolvableJavaWireCondition(jwf.Condition)
			_, knownType := javaWireTypeToAIDL[jwf.WriteMethod]

			// For typed_object/delegate fields with a valid name, check if
			// the field name corresponds to a known parcelable in the specs.
			// If found, store the AIDL qualified name in GoType so the
			// codegen can generate a struct field with proper marshal/unmarshal.
			//
			// The struct field is NOT added to decl.Fields here — it's
			// generated directly by the codegen from JavaWireField.GoType.
			// This avoids adding import graph edges that could destabilize
			// the cycle-breaking back-edge computation for unrelated packages.
			if (jwf.WriteMethod == "typed_object" || jwf.WriteMethod == "delegate") && validName {
				qualifiedName := parcIdx[jwf.Name]
				if qualifiedName == "" && jwf.DelegateType != "" {
					qualifiedName = parcIdx[jwf.DelegateType]
				}
				if qualifiedName == "" && jwf.WriteMethod == "delegate" {
					qualifiedName = resolveDelegateName(jwf.Name, parcIdx)
				}
				if qualifiedName != "" {
					goName := codegen.AIDLToGoName(jwf.Name)
					seenNames[goName]++
					dedupName := jwf.Name
					if seenNames[goName] > 1 {
						dedupName = fmt.Sprintf("%s%d", jwf.Name, seenNames[goName])
					}

					wireFields = append(wireFields, parser.JavaWireField{
						Name:        dedupName,
						WriteMethod: jwf.WriteMethod, // preserve typed_object vs delegate
						GoType:      qualifiedName,
						Condition:   jwf.Condition,
					})
					continue
				}
			}

			// Treat fields with invalid names, unresolvable conditions,
			// or non-primitive types as opaque in the wire format.
			// For non-primitive types (bundle, typed_object, etc.), preserve
			// the specific write_method for correct null marker generation.
			// For primitive types with unresolvable conditions (e.g., a bool
			// field guarded by "hasGainmap()"), use the specific write_method
			// but clear the condition — the codegen can't translate Java
			// method calls to Go, and the field has no corresponding struct field.
			if !validName || !resolvableCond || !knownType {
				wm := jwf.WriteMethod
				// Fields with unresolvable conditions or invalid names that
				// are known primitive types need careful handling:
				//
				// - Invalid name + known type (e.g., "IsExternal?1:0" int32):
				//   keep the original write_method so the codegen reads and
				//   discards the correct number of bytes. Previously this
				//   converted to "typed_object", causing bool-as-int fields
				//   to be treated as null indicators, aborting deserialization
				//   when the value is 1.
				//
				// - Valid name + unresolvable condition + known type: convert
				//   to typed_object (these have struct fields that aren't
				//   conditionally writable from Go).
				//
				// - Unknown type: preserve the write_method as-is (bundle,
				//   typed_object, opaque).
				if knownType && !isNumericLiteral(jwf.Name) && validName {
					wm = "typed_object"
				}
				wireFields = append(wireFields, parser.JavaWireField{
					Name:        jwf.Name,
					WriteMethod: wm,
				})
				continue
			}

			// Deduplicate Go field names by appending a numeric suffix.
			goName := codegen.AIDLToGoName(jwf.Name)
			seenNames[goName]++
			dedupName := jwf.Name
			if seenNames[goName] > 1 {
				dedupName = fmt.Sprintf("%s%d", jwf.Name, seenNames[goName])
			}

			wireFields = append(wireFields, parser.JavaWireField{
				Name:        dedupName,
				WriteMethod: jwf.WriteMethod,
				Condition:   jwf.Condition,
			})

			aidlType := javaWireTypeToAIDL[jwf.WriteMethod]
			decl.Fields = append(decl.Fields, &parser.FieldDecl{
				FieldName: dedupName,
				Type:      &parser.TypeSpecifier{Name: aidlType},
			})
		}
		decl.JavaWireFormat = wireFields
	}

	for _, c := range parc.Constants {
		decl.Constants = append(decl.Constants, convertConstantToAST(c))
	}

	return decl
}

// convertFieldToAST converts a FieldSpec back to a parser.FieldDecl.
func convertFieldToAST(
	f spec.FieldSpec,
) *parser.FieldDecl {
	decl := &parser.FieldDecl{
		FieldName: f.Name,
		Type:      convertTypeRefToAST(f.Type),
		Annots:    convertAnnotationNamesToAST(f.Annotations),
	}

	if f.DefaultValue != "" {
		decl.DefaultValue = parseConstExpr(f.DefaultValue)
	}

	return decl
}

// convertEnumToAST converts an EnumSpec back to a parser.EnumDecl.
func convertEnumToAST(
	enum spec.EnumSpec,
) *parser.EnumDecl {
	decl := &parser.EnumDecl{
		EnumName: enum.Name,
		Annots:   convertAnnotationNamesToAST(enum.Annotations),
	}

	if enum.BackingType != "" {
		decl.BackingType = &parser.TypeSpecifier{Name: enum.BackingType}
	}

	for _, e := range enum.Values {
		enumerator := &parser.Enumerator{
			Name: e.Name,
		}
		if e.Value != "" {
			enumerator.Value = parseConstExpr(e.Value)
		}
		decl.Enumerators = append(decl.Enumerators, enumerator)
	}

	return decl
}

// convertUnionToAST converts a UnionSpec back to a parser.UnionDecl.
func convertUnionToAST(
	union spec.UnionSpec,
) *parser.UnionDecl {
	decl := &parser.UnionDecl{
		UnionName: union.Name,
		Annots:    convertAnnotationNamesToAST(union.Annotations),
	}

	for _, f := range union.Fields {
		decl.Fields = append(decl.Fields, convertFieldToAST(f))
	}

	for _, c := range union.Constants {
		decl.Constants = append(decl.Constants, convertConstantToAST(c))
	}

	return decl
}

// convertConstantToAST converts a ConstantSpec back to a parser.ConstantDecl.
func convertConstantToAST(
	c spec.ConstantSpec,
) *parser.ConstantDecl {
	decl := &parser.ConstantDecl{
		ConstName: c.Name,
	}

	if c.Type != "" {
		decl.Type = &parser.TypeSpecifier{Name: c.Type}
	}

	if c.Value != "" {
		decl.Value = parseConstExpr(c.Value)
	}

	return decl
}

// convertAnnotationNamesToAST converts annotation name strings back to
// parser.Annotation values.
func convertAnnotationNamesToAST(
	names []string,
) []*parser.Annotation {
	if len(names) == 0 {
		return nil
	}

	annots := make([]*parser.Annotation, 0, len(names))
	for _, name := range names {
		annots = append(annots, &parser.Annotation{Name: name})
	}
	return annots
}

// parseConstExpr parses a string constant expression back to a parser.ConstExpr.
// This handles the string representations produced by aidl2spec's constExprToString.
func parseConstExpr(
	value string,
) parser.ConstExpr {
	value = strings.TrimSpace(value)

	if value == "" {
		return nil
	}

	// Boolean literals.
	switch value {
	case "true":
		return &parser.BoolLiteral{Value: true}
	case "false":
		return &parser.BoolLiteral{Value: false}
	case "null":
		return &parser.NullLiteral{}
	}

	// String literals: "..."
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		return &parser.StringLiteralExpr{Value: value[1 : len(value)-1]}
	}

	// Char literals: '...'
	if len(value) >= 2 && value[0] == '\'' && value[len(value)-1] == '\'' {
		return &parser.CharLiteralExpr{Value: value[1 : len(value)-1]}
	}

	// Try to parse as integer (decimal, hex, octal, binary).
	// Accept optional AIDL suffixes (L, u32, etc.) by checking the
	// stripped value, but keep the original for the literal.
	if looksLikeInteger(value) {
		return &parser.IntegerLiteral{Value: value}
	}

	// Parenthesized expressions: (...)
	if len(value) >= 2 && value[0] == '(' && value[len(value)-1] == ')' {
		// Verify the parens are balanced (the closing paren matches the opening one).
		if findMatchingParen(value) == len(value)-1 {
			return parseConstExpr(value[1 : len(value)-1])
		}
	}

	// Unary operators: -, +, ~, !
	if len(value) > 1 {
		switch value[0] {
		case '-':
			inner := value[1:]
			if looksLikeInteger(inner) {
				return &parser.UnaryExpr{
					Op:      parser.TokenMinus,
					Operand: &parser.IntegerLiteral{Value: inner},
				}
			}
			if looksLikeFloat(inner) {
				return &parser.UnaryExpr{
					Op:      parser.TokenMinus,
					Operand: &parser.FloatLiteral{Value: inner},
				}
			}
			return &parser.UnaryExpr{
				Op:      parser.TokenMinus,
				Operand: parseConstExpr(inner),
			}
		case '+':
			return &parser.UnaryExpr{
				Op:      parser.TokenPlus,
				Operand: parseConstExpr(value[1:]),
			}
		case '~':
			return &parser.UnaryExpr{
				Op:      parser.TokenTilde,
				Operand: parseConstExpr(value[1:]),
			}
		case '!':
			return &parser.UnaryExpr{
				Op:      parser.TokenBang,
				Operand: parseConstExpr(value[1:]),
			}
		}
	}

	// Float literals.
	if looksLikeFloat(value) {
		return &parser.FloatLiteral{Value: value}
	}

	// Binary expressions: "left OP right" — try to parse.
	if expr := tryParseBinaryExpr(value); expr != nil {
		return expr
	}

	// Ternary expressions: "cond ? then : else"
	if expr := tryParseTernaryExpr(value); expr != nil {
		return expr
	}

	// Everything else is an identifier reference.
	return &parser.IdentExpr{Name: value}
}

// looksLikeInteger returns true if the value looks like an integer literal
// (decimal, hex, octal, or binary), optionally with AIDL type suffixes.
func looksLikeInteger(
	value string,
) bool {
	if len(value) == 0 {
		return false
	}

	s := value

	// Strip known AIDL integer suffixes.
	for _, suffix := range []string{"u64", "u32", "u16", "u8", "i64", "i32", "i16", "i8", "L", "l"} {
		if strings.HasSuffix(s, suffix) {
			s = s[:len(s)-len(suffix)]
			break
		}
	}

	if len(s) == 0 {
		return false
	}

	// Hex: 0x...
	if len(s) > 2 && s[0] == '0' && (s[1] == 'x' || s[1] == 'X') {
		for _, c := range s[2:] {
			if !isHexDigit(c) {
				return false
			}
		}
		return len(s) > 2
	}

	// Binary: 0b...
	if len(s) > 2 && s[0] == '0' && (s[1] == 'b' || s[1] == 'B') {
		for _, c := range s[2:] {
			if c != '0' && c != '1' {
				return false
			}
		}
		return len(s) > 2
	}

	// Decimal digits.
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func isHexDigit(c rune) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// looksLikeFloat returns true if the value looks like a floating-point literal.
// This includes values with a dot, exponent, or Java/AIDL float suffix
// (e.g., "1.0", "1e5", "1f", "1.5d"), as well as hex floats (e.g., "0x1.Ap+3").
func looksLikeFloat(
	value string,
) bool {
	if len(value) == 0 {
		return false
	}

	// Hex float: 0x<hex>[.<hex>]p[+-]<dec>[fFdD]
	if len(value) > 2 && value[0] == '0' && (value[1] == 'x' || value[1] == 'X') {
		return looksLikeHexFloat(value)
	}

	s := value
	hasSuffix := false
	// Strip Java/AIDL float suffixes.
	last := s[len(s)-1]
	if last == 'f' || last == 'F' || last == 'd' || last == 'D' {
		s = s[:len(s)-1]
		hasSuffix = true
	}

	hasDot := false
	hasE := false
	hasDigit := false
	for i, c := range s {
		switch {
		case c >= '0' && c <= '9':
			hasDigit = true
		case c == '.':
			if hasDot || hasE {
				return false
			}
			hasDot = true
		case c == 'e' || c == 'E':
			if hasE || !hasDigit {
				return false
			}
			hasE = true
		case c == '+' || c == '-':
			if i == 0 || (s[i-1] != 'e' && s[i-1] != 'E') {
				return false
			}
		default:
			return false
		}
	}
	// A float needs digits plus at least one float indicator: dot, exponent, or suffix.
	return hasDigit && (hasDot || hasE || hasSuffix)
}

// looksLikeHexFloat returns true if the value looks like a hex float literal
// (e.g., "0x1.Ap+3", "0xABp-2f"). Hex floats require a binary exponent (p/P).
func looksLikeHexFloat(
	value string,
) bool {
	s := value[2:] // skip "0x"/"0X"
	if len(s) == 0 {
		return false
	}

	// Strip optional float suffix.
	last := s[len(s)-1]
	if last == 'f' || last == 'F' || last == 'd' || last == 'D' {
		s = s[:len(s)-1]
	}

	// Must contain 'p' or 'P' (binary exponent).
	pIdx := strings.IndexAny(s, "pP")
	if pIdx < 0 {
		return false
	}

	mantissa := s[:pIdx]
	exponent := s[pIdx+1:]

	// Mantissa: hex digits with optional dot.
	hasHexDigit := false
	for _, c := range mantissa {
		switch {
		case isHexDigit(c):
			hasHexDigit = true
		case c == '.':
			// allow dot
		default:
			return false
		}
	}
	if !hasHexDigit {
		return false
	}

	// Exponent: optional sign, then decimal digits.
	if len(exponent) == 0 {
		return false
	}
	if exponent[0] == '+' || exponent[0] == '-' {
		exponent = exponent[1:]
	}
	if len(exponent) == 0 {
		return false
	}
	for _, c := range exponent {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// binaryOpGroup is a set of operators at the same precedence level.
// Within a group, the rightmost match is used so that left-to-right
// associativity is preserved (e.g., "a - b + c" → "(a - b) + c").
type binaryOpGroup struct {
	Ops []struct {
		Symbol string
		Token  parser.TokenKind
	}
}

// binaryOpGroups lists operator groups from lowest to highest precedence.
// Multi-character operators must appear before single-character operators
// that are their prefixes within the same group (e.g., ">=" before ">").
var binaryOpGroups = []binaryOpGroup{
	{Ops: []struct {
		Symbol string
		Token  parser.TokenKind
	}{
		{"||", parser.TokenPipePipe},
	}},
	{Ops: []struct {
		Symbol string
		Token  parser.TokenKind
	}{
		{"&&", parser.TokenAmpAmp},
	}},
	{Ops: []struct {
		Symbol string
		Token  parser.TokenKind
	}{
		{"|", parser.TokenPipe},
	}},
	{Ops: []struct {
		Symbol string
		Token  parser.TokenKind
	}{
		{"^", parser.TokenCaret},
	}},
	{Ops: []struct {
		Symbol string
		Token  parser.TokenKind
	}{
		{"&", parser.TokenAmp},
	}},
	{Ops: []struct {
		Symbol string
		Token  parser.TokenKind
	}{
		{"==", parser.TokenEqEq},
		{"!=", parser.TokenBangEq},
	}},
	{Ops: []struct {
		Symbol string
		Token  parser.TokenKind
	}{
		{"<=", parser.TokenLessEq},
		{">=", parser.TokenGreaterEq},
		{"<", parser.TokenLAngle},
		{">", parser.TokenRAngle},
	}},
	{Ops: []struct {
		Symbol string
		Token  parser.TokenKind
	}{
		{"<<", parser.TokenLShift},
		{">>", parser.TokenRShift},
	}},
	{Ops: []struct {
		Symbol string
		Token  parser.TokenKind
	}{
		{"+", parser.TokenPlus},
		{"-", parser.TokenMinus},
	}},
	{Ops: []struct {
		Symbol string
		Token  parser.TokenKind
	}{
		{"*", parser.TokenStar},
		{"/", parser.TokenSlash},
		{"%", parser.TokenPercent},
	}},
}

// tryParseBinaryExpr attempts to parse "left OP right" binary expressions.
// Returns nil if the value does not look like a binary expression.
//
// Operators are tried from lowest to highest precedence. Within each
// precedence group, the rightmost match of any operator in the group is
// used, which produces a left-associative parse tree.
func tryParseBinaryExpr(
	value string,
) parser.ConstExpr {
	for _, group := range binaryOpGroups {
		bestIdx := -1
		bestLen := 0
		var bestToken parser.TokenKind

		for _, op := range group.Ops {
			padded := " " + op.Symbol + " "
			idx := lastIndexAtDepthZero(value, padded)
			if idx < 0 {
				continue
			}
			if idx > bestIdx {
				bestIdx = idx
				bestLen = len(padded)
				bestToken = op.Token
			}
		}

		if bestIdx < 0 {
			continue
		}

		left := strings.TrimSpace(value[:bestIdx])
		right := strings.TrimSpace(value[bestIdx+bestLen:])
		if left == "" || right == "" {
			continue
		}

		return &parser.BinaryExpr{
			Op:    bestToken,
			Left:  parseConstExpr(left),
			Right: parseConstExpr(right),
		}
	}
	return nil
}

// lastIndexAtDepthZero returns the last position of needle in s
// that occurs at parenthesis depth 0. Returns -1 if not found.
func lastIndexAtDepthZero(
	s string,
	needle string,
) int {
	best := -1
	depth := 0
	for i := 0; i <= len(s)-len(needle); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
		}
		if depth == 0 && s[i:i+len(needle)] == needle {
			best = i
		}
	}
	return best
}

// findMatchingParen returns the index of the closing ')' that matches
// the opening '(' at index 0. Returns -1 if not found.
func findMatchingParen(
	s string,
) int {
	depth := 0
	for i, c := range s {
		switch c {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// tryParseTernaryExpr attempts to parse "cond ? then : else".
func tryParseTernaryExpr(
	value string,
) parser.ConstExpr {
	qIdx := strings.Index(value, " ? ")
	if qIdx < 0 {
		return nil
	}

	rest := value[qIdx+3:]
	cIdx := lastIndexAtDepthZero(rest, " : ")
	if cIdx < 0 {
		return nil
	}

	cond := strings.TrimSpace(value[:qIdx])
	then := strings.TrimSpace(rest[:cIdx])
	elseExpr := strings.TrimSpace(rest[cIdx+3:])

	if cond == "" || then == "" || elseExpr == "" {
		return nil
	}

	return &parser.TernaryExpr{
		Cond: parseConstExpr(cond),
		Then: parseConstExpr(then),
		Else: parseConstExpr(elseExpr),
	}
}

// newRegistryResolver creates a resolver.Resolver whose type registry is
// pre-populated. The resolver has no search paths (specs replace AIDL files).
func newRegistryResolver(
	registry *resolver.TypeRegistry,
) *resolver.Resolver {
	r := resolver.New(nil)
	// Transfer all definitions from our registry to the resolver's registry.
	for qualifiedName, def := range registry.All() {
		r.Registry.Register(qualifiedName, def)
	}
	return r
}

// generateServiceNamesFile generates servicemanager/service_names_gen.go from
// the Services field of the servicemanager package spec. Each ServiceMapping
// produces a typed constant: ConstantName (SCREAMING_SNAKE) is converted to
// PascalCase, and the value is the service_name string.
func generateServiceNamesFile(
	specs map[string]*spec.PackageSpec,
	outputDir string,
) error {
	smSpec := specs["servicemanager"]
	if smSpec == nil || len(smSpec.Services) == 0 {
		return nil
	}

	// Build sorted constant entries.
	type constEntry struct {
		GoName      string
		ServiceName string
	}

	entries := make([]constEntry, 0, len(smSpec.Services))
	for _, svc := range smSpec.Services {
		goName := codegen.ScreamingSnakeToPascal(svc.ConstantName)
		entries = append(entries, constEntry{
			GoName:      goName,
			ServiceName: svc.ServiceName,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].GoName < entries[j].GoName
	})

	// Find the longest GoName for column alignment.
	maxLen := 0
	for _, e := range entries {
		if len(e.GoName) > maxLen {
			maxLen = len(e.GoName)
		}
	}

	var buf bytes.Buffer
	buf.WriteString("// Code generated by spec2go. DO NOT EDIT.\n\n")
	buf.WriteString("package servicemanager\n\n")
	buf.WriteString("// Service name constants extracted from android.content.Context.\n")
	buf.WriteString("const (\n")

	for _, e := range entries {
		padding := strings.Repeat(" ", maxLen-len(e.GoName))
		fmt.Fprintf(&buf, "\t%s%s ServiceName = %q\n", e.GoName, padding, e.ServiceName)
	}

	buf.WriteString(")\n")

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return fmt.Errorf("formatting service names: %w\n\nRaw source:\n%s", err, buf.String())
	}

	outPath := filepath.Join(outputDir, "servicemanager", "service_names_gen.go")
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}
	if err := os.WriteFile(outPath, formatted, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", outPath, err)
	}

	fmt.Fprintf(os.Stderr, "Wrote %s (%d constants)\n", outPath, len(entries))
	return nil
}

// generateAccessorFiles generates get_*.go files for services whose AIDL
// interface has a generated proxy in the output directory. Each file contains
// a GetXxxManager function that calls ServiceManager.GetService and wraps
// the result in the typed proxy constructor.
func generateAccessorFiles(
	specs map[string]*spec.PackageSpec,
	outputDir string,
) error {
	smSpec := specs["servicemanager"]
	if smSpec == nil || len(smSpec.Services) == 0 {
		return nil
	}

	// Build a descriptor -> InterfaceSpec index across all specs so we can
	// look up the interface definition for each service's descriptor.
	type ifaceInfo struct {
		GoPackage     string
		InterfaceName string
	}
	descriptorIndex := map[string]ifaceInfo{}
	for _, ps := range specs {
		for _, iface := range ps.Interfaces {
			desc := ps.AIDLPackage + "." + iface.Name
			descriptorIndex[desc] = ifaceInfo{
				GoPackage:     ps.GoPackage,
				InterfaceName: iface.Name,
			}
		}
	}

	// Track emitted (goPackage, funcName) pairs to detect collisions when
	// multiple services map to the same interface in the same package.
	emittedFuncs := map[string]bool{}

	count := 0
	for _, svc := range smSpec.Services {
		info, ok := descriptorIndex[svc.Descriptor]
		if !ok {
			continue
		}

		interfaceGoName := codegen.AIDLToGoName(info.InterfaceName)
		proxyName := deriveProxyName(interfaceGoName)
		constructorName := "New" + proxyName

		// Verify the proxy constructor exists in the output directory by
		// checking that the generated interface file is present.
		goFileName := codegen.AIDLToGoFileName(info.InterfaceName)
		ifaceFilePath := filepath.Join(outputDir, info.GoPackage, goFileName)
		if _, err := os.Stat(ifaceFilePath); err != nil {
			continue
		}

		// Determine the Go package name (last segment of the package path).
		goPkg := filepath.Base(info.GoPackage)

		// Build the constant name for the servicemanager constant.
		constName := codegen.ScreamingSnakeToPascal(svc.ConstantName)

		// The base name without Proxy suffix is used for the Get function.
		baseName := interfaceGoName
		if strings.HasPrefix(interfaceGoName, "I") && len(interfaceGoName) > 1 {
			baseName = interfaceGoName[1:]
		}
		funcName := "Get" + baseName

		// If a function with this name was already emitted in this package,
		// derive the name from the service constant instead to avoid collision.
		funcKey := info.GoPackage + ":" + funcName
		if emittedFuncs[funcKey] {
			funcName = "Get" + constName
			funcKey = info.GoPackage + ":" + funcName
		}
		emittedFuncs[funcKey] = true

		var buf bytes.Buffer
		buf.WriteString("// Code generated by spec2go. DO NOT EDIT.\n\n")
		fmt.Fprintf(&buf, "package %s\n\n", goPkg)
		buf.WriteString("import (\n")
		buf.WriteString("\t\"context\"\n")
		buf.WriteString("\t\"fmt\"\n\n")
		buf.WriteString("\t\"github.com/AndroidGoLab/binder/servicemanager\"\n")
		buf.WriteString(")\n\n")
		fmt.Fprintf(&buf, "// %s retrieves the %s service and returns a typed proxy.\n", funcName, constName)
		fmt.Fprintf(&buf, "func %s(\n", funcName)
		buf.WriteString("\tctx context.Context,\n")
		buf.WriteString("\tsm *servicemanager.ServiceManager,\n")
		fmt.Fprintf(&buf, ") (*%s, error) {\n", proxyName)
		fmt.Fprintf(&buf, "\tsvc, err := sm.GetService(ctx, servicemanager.%s)\n", constName)
		buf.WriteString("\tif err != nil {\n")
		fmt.Fprintf(&buf, "\t\treturn nil, fmt.Errorf(\"%s: %%w\", err)\n", funcName)
		buf.WriteString("\t}\n")
		fmt.Fprintf(&buf, "\treturn %s(svc), nil\n", constructorName)
		buf.WriteString("}\n")

		formatted, fmtErr := format.Source(buf.Bytes())
		if fmtErr != nil {
			return fmt.Errorf("formatting accessor for %s: %w\n\nRaw source:\n%s", svc.ServiceName, fmtErr, buf.String())
		}

		outPath := filepath.Join(outputDir, info.GoPackage, "get_"+svc.ServiceName+".go")
		if err := os.WriteFile(outPath, formatted, 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", outPath, err)
		}
		count++
	}

	fmt.Fprintf(os.Stderr, "Generated %d accessor files\n", count)
	return nil
}

// deriveProxyName derives a proxy struct name from an interface name.
// IFoo -> FooProxy, Foo -> FooProxy. This mirrors the codegen convention.
func deriveProxyName(interfaceName string) string {
	base := interfaceName
	if len(interfaceName) >= 2 && interfaceName[0] == 'I' && interfaceName[1] >= 'A' && interfaceName[1] <= 'Z' {
		base = interfaceName[1:]
	}
	return base + "Proxy"
}

// generateCodesFile builds codes_gen.go from version_codes embedded in
// interface specs.
func generateCodesFile(
	specs map[string]*spec.PackageSpec,
	codesOutput string,
	defaultAPI int,
) error {
	fmt.Fprintf(os.Stderr, "Generating codes_gen.go...\n")

	allTables := map[string]map[string]map[string]binder.TransactionCode{}
	apiRevisions := map[int][]string{}

	// Collect version codes from all interface specs.
	for _, ps := range specs {
		for _, iface := range ps.Interfaces {
			if len(iface.VersionCodes) == 0 {
				continue
			}

			for versionID, methodOffsets := range iface.VersionCodes {
				if allTables[versionID] == nil {
					allTables[versionID] = map[string]map[string]binder.TransactionCode{}
				}

				methods := map[string]binder.TransactionCode{}
				for methodName, offset := range methodOffsets {
					methods[methodName] = binder.FirstCallTransaction + binder.TransactionCode(offset)
				}
				allTables[versionID][iface.Descriptor] = methods
			}
		}
	}

	if len(allTables) == 0 {
		fmt.Fprintf(os.Stderr, "No version codes found in specs, skipping codes_gen.go\n")
		return nil
	}

	// Build apiRevisions from the version IDs. Version IDs have the
	// format "36.local" or "36.r4". Group by API level and sort.
	apiLevelSet := map[int]map[string]bool{}
	for versionID := range allTables {
		dotIdx := strings.IndexByte(versionID, '.')
		if dotIdx < 0 {
			continue
		}
		levelStr := versionID[:dotIdx]
		level := 0
		for _, c := range levelStr {
			if c < '0' || c > '9' {
				level = -1
				break
			}
			level = level*10 + int(c-'0')
		}
		if level < 0 {
			continue
		}

		if apiLevelSet[level] == nil {
			apiLevelSet[level] = map[string]bool{}
		}
		apiLevelSet[level][versionID] = true
	}

	apiLevels := make([]int, 0, len(apiLevelSet))
	for level := range apiLevelSet {
		apiLevels = append(apiLevels, level)
	}
	sort.Ints(apiLevels)

	// Ensure defaultAPI is in the list.
	hasDefault := false
	for _, level := range apiLevels {
		if level == defaultAPI {
			hasDefault = true
			break
		}
	}
	if !hasDefault {
		apiLevels = append(apiLevels, defaultAPI)
		sort.Ints(apiLevels)
	}

	for level, vids := range apiLevelSet {
		sorted := sortedKeys(vids)
		// Store revisions latest-first for probing.
		reversed := make([]string, len(sorted))
		for i, v := range sorted {
			reversed[len(sorted)-1-i] = v
		}
		apiRevisions[level] = reversed
	}

	src, err := generateSource(defaultAPI, allTables, apiRevisions, apiLevels)
	if err != nil {
		return fmt.Errorf("generating source: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(codesOutput), 0o755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}
	if err := os.WriteFile(codesOutput, src, 0o644); err != nil {
		return fmt.Errorf("writing output: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Wrote %s (%d bytes)\n", codesOutput, len(src))
	return nil
}

// generateSource produces the Go source for codes_gen.go.
func generateSource(
	defaultAPI int,
	allTables map[string]map[string]map[string]binder.TransactionCode,
	apiRevisions map[int][]string,
	apiLevels []int,
) ([]byte, error) {
	var buf bytes.Buffer

	buf.WriteString("// Code generated by spec2go. DO NOT EDIT.\n\n")
	buf.WriteString("package versionaware\n\n")
	buf.WriteString("import \"github.com/AndroidGoLab/binder/binder\"\n\n")

	fmt.Fprintf(&buf, "func init() {\n")
	fmt.Fprintf(&buf, "\tDefaultAPILevel = %d\n", defaultAPI)

	versionIDs := sortedKeys(allTables)
	buf.WriteString("\tTables = MultiVersionTable{\n")

	for _, vid := range versionIDs {
		table := allTables[vid]
		fmt.Fprintf(&buf, "\t\t%q: VersionTable{\n", vid)

		descriptors := sortedKeys(table)
		for _, desc := range descriptors {
			methods := table[desc]
			if len(methods) == 0 {
				continue
			}

			fmt.Fprintf(&buf, "\t\t\t%q: {\n", desc)

			methodNames := sortedKeys(methods)
			for _, name := range methodNames {
				code := methods[name]
				offset := code - binder.FirstCallTransaction
				fmt.Fprintf(&buf, "\t\t\t\t%q: binder.FirstCallTransaction + %d,\n", name, offset)
			}

			buf.WriteString("\t\t\t},\n")
		}

		buf.WriteString("\t\t},\n")
	}

	buf.WriteString("\t}\n")

	buf.WriteString("\tRevisions = APIRevisions{\n")
	for _, level := range apiLevels {
		revs := apiRevisions[level]
		if len(revs) == 0 {
			continue
		}
		fmt.Fprintf(&buf, "\t\t%d: {", level)
		for i, rev := range revs {
			if i > 0 {
				buf.WriteString(", ")
			}
			fmt.Fprintf(&buf, "%q", rev)
		}
		buf.WriteString("},\n")
	}
	buf.WriteString("\t}\n")

	buf.WriteString("}\n")

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("formatting generated code: %w\n\nRaw source:\n%s", err, buf.String())
	}
	return formatted, nil
}

// countGoFiles counts .go files in the output directory tree.
func countGoFiles(
	dir string,
) (int, error) {
	count := 0
	err := filepath.Walk(dir, func(
		path string,
		info os.FileInfo,
		err error,
	) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && strings.HasSuffix(path, ".go") {
			count++
		}
		return nil
	})
	return count, err
}

// sortedKeys returns the keys of a map sorted alphabetically.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// generateJavaConstantsFiles generates Go source files for typed string
// constants extracted from Java sources (e.g., LocationProvider constants).
func generateJavaConstantsFiles(
	specs map[string]*spec.PackageSpec,
	outputDir string,
) error {
	totalGroups := 0
	for _, ps := range specs {
		if len(ps.JavaConstants) == 0 {
			continue
		}

		goPkg := ps.GoPackage
		if goPkg == "" {
			continue
		}

		// Derive the short package name (last segment of the Go package path).
		pkgParts := strings.Split(goPkg, "/")
		shortPkg := pkgParts[len(pkgParts)-1]

		for _, group := range ps.JavaConstants {
			if len(group.Values) == 0 {
				continue
			}

			var buf bytes.Buffer
			buf.WriteString("// Code generated by spec2go. DO NOT EDIT.\n\n")
			fmt.Fprintf(&buf, "package %s\n\n", shortPkg)

			// Type declaration.
			fmt.Fprintf(&buf, "// %s is a typed string constant.\n", group.GoType)
			fmt.Fprintf(&buf, "type %s = string\n\n", group.GoType)

			// Constants.
			buf.WriteString("const (\n")

			// Find max name length for alignment.
			maxLen := 0
			for _, v := range group.Values {
				if len(v.Name) > maxLen {
					maxLen = len(v.Name)
				}
			}

			for _, v := range group.Values {
				padding := strings.Repeat(" ", maxLen-len(v.Name))
				fmt.Fprintf(&buf, "\t%s%s %s = %q\n", v.Name, padding, group.GoType, v.Value)
			}

			buf.WriteString(")\n")

			formatted, err := format.Source(buf.Bytes())
			if err != nil {
				return fmt.Errorf("formatting %s constants: %w\n\nRaw source:\n%s", group.GoType, err, buf.String())
			}

			fileName := strings.ToLower(group.GoType) + "_constants_gen.go"
			outDir := filepath.Join(outputDir, goPkg)
			if err := os.MkdirAll(outDir, 0o755); err != nil {
				return fmt.Errorf("creating directory %s: %w", outDir, err)
			}
			outPath := filepath.Join(outDir, fileName)
			if err := os.WriteFile(outPath, formatted, 0o644); err != nil {
				return fmt.Errorf("writing %s: %w", outPath, err)
			}

			totalGroups++
		}
	}

	if totalGroups > 0 {
		fmt.Fprintf(os.Stderr, "Generated %d Java constant files\n", totalGroups)
	}
	return nil
}
