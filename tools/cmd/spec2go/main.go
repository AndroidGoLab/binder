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

	"github.com/xaionaro-go/binder/binder"
	"github.com/xaionaro-go/binder/tools/pkg/codegen"
	"github.com/xaionaro-go/binder/tools/pkg/parser"
	"github.com/xaionaro-go/binder/tools/pkg/resolver"
	"github.com/xaionaro-go/binder/tools/pkg/spec"
)

func main() {
	specsDir := flag.String("specs", "specs/", "Directory containing spec YAML files")
	outputDir := flag.String("output", ".", "Output directory for generated Go files")
	smokeTests := flag.Bool("smoke-tests", false, "Generate smoke tests")
	codesOutput := flag.String("codes-output", "", "Output path for codes_gen.go")
	defaultAPI := flag.Int("default-api", 36, "Default API level")
	flag.Parse()

	if err := run(
		*specsDir,
		*outputDir,
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

	// Convert specs to parser AST and register in a type registry.
	registry := resolver.NewTypeRegistry()
	for _, ps := range specs {
		registerPackageSpec(registry, ps)
	}

	allDefs := registry.All()
	fmt.Fprintf(os.Stderr, "Registered %d definitions\n", len(allDefs))

	// Create a resolver wrapping the pre-populated registry.
	r := newRegistryResolver(registry)

	// Generate Go code.
	fmt.Fprintf(os.Stderr, "Generating Go code into %s...\n", outputDir)
	gen := codegen.NewGenerator(r, outputDir)
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
	}

	if smokeTests {
		fmt.Fprintf(os.Stderr, "Generating smoke tests...\n")
		if err := gen.GenerateAllSmokeTests(); err != nil {
			smokeErrors := strings.Split(err.Error(), "\n")
			fmt.Fprintf(os.Stderr, "Smoke test generation completed with %d errors (skipped)\n", len(smokeErrors))
		}
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

// registerPackageSpec converts all definitions in a PackageSpec to parser
// AST types and registers them in the type registry.
func registerPackageSpec(
	registry *resolver.TypeRegistry,
	ps *spec.PackageSpec,
) {
	for _, iface := range ps.Interfaces {
		qualifiedName := ps.AIDLPackage + "." + iface.Name
		def := convertInterfaceToAST(iface)
		registry.Register(qualifiedName, def)
	}

	for _, parc := range ps.Parcelables {
		qualifiedName := ps.AIDLPackage + "." + parc.Name
		def := convertParcelableToAST(parc)
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
		ParamName: p.Name,
		Type:      convertTypeRefToAST(p.Type),
		Annots:    convertAnnotationNamesToAST(p.Annotations),
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

// convertParcelableToAST converts a ParcelableSpec back to a parser.ParcelableDecl.
func convertParcelableToAST(
	parc spec.ParcelableSpec,
) *parser.ParcelableDecl {
	decl := &parser.ParcelableDecl{
		ParcName: parc.Name,
		Annots:   convertAnnotationNamesToAST(parc.Annotations),
	}

	for _, f := range parc.Fields {
		decl.Fields = append(decl.Fields, convertFieldToAST(f))
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
			idx := strings.LastIndex(value, padded)
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
	cIdx := strings.Index(rest, " : ")
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
	buf.WriteString("import \"github.com/xaionaro-go/binder/binder\"\n\n")

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
