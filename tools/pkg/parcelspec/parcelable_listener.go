package parcelspec

import (
	"strconv"
	"strings"
	"unicode"

	"github.com/AndroidGoLab/binder/tools/pkg/javaparser"
)

// parcelableListener walks a Java AST and extracts ParcelableSpec
// from classes that contain writeToParcel methods.
type parcelableListener struct {
	javaparser.BaseJavaParserListener

	packageName string
	specs       []ParcelableSpec

	// Per-class state, reset on each EnterClassDeclaration.
	className string

	// constants maps constant names (e.g. "HAS_ALTITUDE_MASK") to their
	// integer values, extracted from static final int fields.
	constants map[string]int64

	// hasMethods maps hasXxx() method names to their mask constant names.
	// For example: "hasAltitude" -> "HAS_ALTITUDE_MASK".
	hasMethods map[string]string

	// fieldTypes maps field names (PascalCase, m-prefix stripped) to their
	// declared Java type names. Used to resolve delegate types when the
	// field name differs from the type name (e.g., mSmallIcon → Icon).
	fieldTypes map[string]string

	// classDepth tracks nesting level so we can identify the outermost class.
	classDepth int
}

func newParcelableListener(packageName string) *parcelableListener {
	return &parcelableListener{
		packageName: packageName,
		constants:   make(map[string]int64),
		hasMethods:  make(map[string]string),
		fieldTypes:  make(map[string]string),
	}
}

func (l *parcelableListener) EnterClassDeclaration(ctx *javaparser.ClassDeclarationContext) {
	l.classDepth++
	if l.classDepth > 1 {
		return
	}

	id := ctx.Identifier()
	if id == nil {
		return
	}

	l.className = id.GetText()
	l.constants = make(map[string]int64)
	l.hasMethods = make(map[string]string)
}

func (l *parcelableListener) ExitClassDeclaration(_ *javaparser.ClassDeclarationContext) {
	l.classDepth--
}

// EnterFieldDeclaration collects:
// 1. "private static final int HAS_*_MASK = <value>" constants for mask resolution.
// 2. Field name → type mappings for delegate type resolution.
func (l *parcelableListener) EnterFieldDeclaration(ctx *javaparser.FieldDeclarationContext) {
	if l.classDepth != 1 {
		return
	}

	tt := ctx.TypeType()
	if tt == nil {
		return
	}
	rawTypeName := tt.GetText()

	// Collect field name → type mappings for all fields.
	simpleType := extractSimpleTypeName(rawTypeName)
	if simpleType != "" {
		declarators := ctx.VariableDeclarators()
		if declarators != nil {
			for _, decl := range declarators.AllVariableDeclarator() {
				declID := decl.VariableDeclaratorId()
				if declID == nil {
					continue
				}
				fieldName := deriveFieldName(declID.Identifier().GetText())
				l.fieldTypes[fieldName] = simpleType
			}
		}
	}

	// Collect static final int constants for mask resolution.
	if rawTypeName != "int" || !isStaticFinal(ctx) {
		return
	}

	declarators := ctx.VariableDeclarators()
	if declarators == nil {
		return
	}

	for _, decl := range declarators.AllVariableDeclarator() {
		declID := decl.VariableDeclaratorId()
		if declID == nil {
			continue
		}

		name := declID.GetText()
		init := decl.VariableInitializer()
		if init == nil {
			continue
		}

		value := parseIntExpression(init.GetText())
		l.constants[name] = value
	}
}

// EnterMethodDeclaration is called for each method. It handles two tasks:
//  1. Collects hasXxx() method patterns to map to mask constants.
//  2. Processes writeToParcel methods to extract field specs.
func (l *parcelableListener) EnterMethodDeclaration(ctx *javaparser.MethodDeclarationContext) {
	if l.classDepth != 1 {
		return
	}

	id := ctx.Identifier()
	if id == nil {
		return
	}

	methodName := id.GetText()

	switch {
	case strings.HasPrefix(methodName, "has") && methodName != "hashCode":
		l.collectHasMethod(methodName, ctx)
	case (methodName == "writeToParcel" || methodName == "writeToParcelImpl") && !isStaticMethod(ctx):
		l.processWriteToParcel(ctx)
	}
}

// collectHasMethod extracts the mask constant name from a hasXxx() method body.
// It looks for "(mFieldsMask & HAS_*_MASK) != 0" patterns.
func (l *parcelableListener) collectHasMethod(
	methodName string,
	ctx *javaparser.MethodDeclarationContext,
) {
	body := ctx.MethodBody()
	if body == nil {
		return
	}

	// Get the full text of the method body and look for the mask pattern.
	text := body.GetText()

	// Match pattern: mFieldsMask&CONSTANT_NAME
	maskName := extractMaskConstant(text)
	if maskName == "" {
		return
	}

	l.hasMethods[methodName] = maskName
}

// processWriteToParcel walks the writeToParcel method body and extracts
// FieldSpec entries from parcel.writeXxx() calls.
func (l *parcelableListener) processWriteToParcel(
	ctx *javaparser.MethodDeclarationContext,
) {
	body := ctx.MethodBody()
	if body == nil {
		return
	}

	block := body.Block()
	if block == nil {
		return
	}

	fields := l.extractFieldsFromBlock(block, "")

	if len(fields) == 0 {
		return
	}

	l.specs = append(l.specs, ParcelableSpec{
		Package: l.packageName,
		Type:    l.className,
		Fields:  fields,
	})
}

// extractFieldsFromBlock walks block statements and extracts FieldSpec entries.
// The condition parameter is set when we're inside a conditional block.
func (l *parcelableListener) extractFieldsFromBlock(
	block javaparser.IBlockContext,
	condition string,
) []FieldSpec {
	var fields []FieldSpec

	for _, blockStmt := range block.AllBlockStatement() {
		stmt := blockStmt.Statement()
		if stmt == nil {
			continue
		}

		fields = append(fields, l.extractFieldsFromStatement(stmt, condition)...)
	}

	return fields
}

// extractFieldsFromStatement processes a single statement, handling
// both expression statements (parcel.writeXxx) and if-statements.
func (l *parcelableListener) extractFieldsFromStatement(
	stmt javaparser.IStatementContext,
	condition string,
) []FieldSpec {
	// Check for if-statement.
	if stmt.IF() != nil {
		return l.extractFieldsFromIfStatement(stmt, condition)
	}

	// Check for try block: try { ... } catch/finally { ... }
	// Recurse into the try body to extract write calls.
	if stmt.TRY() != nil {
		blk := stmt.Block()
		if blk != nil {
			return l.extractFieldsFromBlock(blk, condition)
		}
		return nil
	}

	// Check for synchronized block: synchronized(expr) { ... }
	// Recurse into the body to extract write calls.
	if stmt.SYNCHRONIZED() != nil {
		blk := stmt.Block()
		if blk != nil {
			return l.extractFieldsFromBlock(blk, condition)
		}
		return nil
	}

	// Check for expression statement: parcel.writeXxx(arg).
	exprStmt := stmt.GetStatementExpression()
	if exprStmt == nil {
		return nil
	}

	field := l.extractFieldFromExpression(exprStmt, condition)
	if field == nil {
		return nil
	}

	return []FieldSpec{*field}
}

// extractFieldsFromIfStatement handles conditional writes.
// It extracts the condition from the if-expression and recurses
// into the if-body.
func (l *parcelableListener) extractFieldsFromIfStatement(
	stmt javaparser.IStatementContext,
	parentCondition string,
) []FieldSpec {
	exprs := stmt.AllExpression()
	if len(exprs) == 0 {
		return nil
	}

	condExpr := exprs[0]
	condition := l.resolveCondition(condExpr, parentCondition)

	stmts := stmt.AllStatement()
	if len(stmts) == 0 {
		return nil
	}

	// The body of the if-statement is the first child statement.
	bodyStmt := stmts[0]

	// The body might be a block or a single statement.
	bodyBlock := bodyStmt.Block()
	if bodyBlock != nil {
		return l.extractFieldsFromBlock(bodyBlock, condition)
	}

	// Single statement (no braces).
	return l.extractFieldsFromStatement(bodyStmt, condition)
}

// resolveCondition determines the condition string for a conditional write.
// It looks for hasXxx() method calls and resolves them to mask expressions.
func (l *parcelableListener) resolveCondition(
	condExpr javaparser.IExpressionContext,
	parentCondition string,
) string {
	text := condExpr.GetText()

	// Look for hasXxx() calls in the condition.
	for methodName, maskConstant := range l.hasMethods {
		if !strings.Contains(text, methodName+"()") {
			continue
		}

		maskValue, ok := l.constants[maskConstant]
		if !ok {
			// Fall back to the method name if we can't resolve the mask value.
			return combineConditions(parentCondition, methodName)
		}

		return combineConditions(
			parentCondition,
			"FieldsMask & "+strconv.FormatInt(maskValue, 10),
		)
	}

	// Couldn't resolve to a known pattern; use the raw text.
	return combineConditions(parentCondition, text)
}

// extractFieldFromExpression attempts to extract a FieldSpec from
// an expression like parcel.writeXxx(mFieldName).
func (l *parcelableListener) extractFieldFromExpression(
	expr javaparser.IExpressionContext,
	condition string,
) *FieldSpec {
	// The expression must be a method call on a member: expr.methodCall()
	memberRef, ok := expr.(*javaparser.MemberReferenceExpressionContext)
	if !ok {
		return nil
	}

	mc := memberRef.MethodCall()
	if mc == nil {
		return nil
	}

	id := mc.Identifier()
	if id == nil {
		return nil
	}

	methodName := id.GetText()

	// Delegation: mFoo.writeToParcel(dest, flags) means the field mFoo
	// is itself a Parcelable whose wire format is written inline (no
	// nullable wrapper). Unlike writeTypedObject which adds a non-null
	// int32 marker, delegation writes the inner data directly.
	//
	// Static helper: TextUtils.writeToParcel(text, dest, flags) passes
	// the field as the first argument. Detected by uppercase receiver
	// (class name) and 3+ arguments.
	if methodName == "writeToParcel" {
		receiverText := memberRef.Expression().GetText()

		args := mc.Arguments()
		argCount := 0
		if args != nil && args.ExpressionList() != nil {
			argCount = len(args.ExpressionList().AllExpression())
		}

		if argCount >= 3 && len(receiverText) > 0 && unicode.IsUpper(rune(receiverText[0])) {
			// Static helper: first argument is the field being serialized.
			fieldName := deriveFieldName(args.ExpressionList().AllExpression()[0].GetText())
			// TextUtils.writeToParcel writes a CharSequence — use a
			// dedicated spec type so the codegen can skip it.
			specType := "delegate"
			if receiverText == "TextUtils" {
				specType = "char_sequence"
			}
			return &FieldSpec{
				Name:         fieldName,
				Type:         specType,
				DelegateType: receiverText,
				Condition:    condition,
			}
		}

		fieldName := deriveFieldName(receiverText)
		// Use collected field type declarations to resolve the delegate
		// type when the field name differs from the type (e.g., mSmallIcon
		// is declared as type Icon → DelegateType = "Icon").
		delegateType := l.fieldTypes[fieldName]
		return &FieldSpec{
			Name:         fieldName,
			Type:         "delegate",
			DelegateType: delegateType,
			Condition:    condition,
		}
	}

	specType, known := javaWriteMethodToSpecType[methodName]
	if !known {
		// Unknown write method; treat as opaque if it starts with "write".
		if !strings.HasPrefix(methodName, "write") {
			return nil
		}
		specType = "opaque"
	}

	// Extract the first argument text to derive the field name.
	args := mc.Arguments()
	if args == nil {
		return nil
	}

	exprList := args.ExpressionList()
	if exprList == nil {
		return nil
	}

	allExprs := exprList.AllExpression()
	if len(allExprs) == 0 {
		return nil
	}

	argText := allExprs[0].GetText()

	// Handle ternary: dest.writeInt(mFoo ? 1 : 0) → field name "Foo", type bool.
	if ternary, ok := allExprs[0].(*javaparser.TernaryExpressionContext); ok {
		condText := ternary.Expression(0).GetText()
		return &FieldSpec{
			Name:      deriveFieldName(condText),
			Type:      "bool",
			Condition: condition,
		}
	}

	fieldName := deriveFieldName(argText)

	return &FieldSpec{
		Name:      fieldName,
		Type:      specType,
		Condition: condition,
	}
}

// isStaticMethod checks whether a method declaration has the static modifier.
// Static writeToParcel overloads (e.g., ComponentName.writeToParcel(ComponentName c, Parcel out))
// are helper methods, not the instance serialization method we need.
func isStaticMethod(ctx *javaparser.MethodDeclarationContext) bool {
	// MethodDeclaration -> MemberDeclaration -> ClassBodyDeclaration
	memberCtx := ctx.GetParent()
	if memberCtx == nil {
		return false
	}

	classBodyDecl, ok := memberCtx.GetParent().(*javaparser.ClassBodyDeclarationContext)
	if !ok {
		return false
	}

	for _, mod := range classBodyDecl.AllModifier() {
		coiMod := mod.ClassOrInterfaceModifier()
		if coiMod == nil {
			continue
		}
		if coiMod.STATIC() != nil {
			return true
		}
	}
	return false
}

// isStaticFinal checks whether a field declaration has static and final modifiers.
func isStaticFinal(ctx *javaparser.FieldDeclarationContext) bool {
	// FieldDeclaration -> MemberDeclaration -> ClassBodyDeclaration
	memberCtx := ctx.GetParent()
	if memberCtx == nil {
		return false
	}

	classBodyDecl, ok := memberCtx.GetParent().(*javaparser.ClassBodyDeclarationContext)
	if !ok {
		return false
	}

	var hasStatic, hasFinal bool
	for _, mod := range classBodyDecl.AllModifier() {
		coiMod := mod.ClassOrInterfaceModifier()
		if coiMod == nil {
			continue
		}

		switch {
		case coiMod.STATIC() != nil:
			hasStatic = true
		case coiMod.FINAL() != nil:
			hasFinal = true
		}
	}

	return hasStatic && hasFinal
}

// extractMaskConstant extracts a mask constant name from text like
// "(mFieldsMask&HAS_ALTITUDE_MASK)!=0".
func extractMaskConstant(text string) string {
	// Look for "mFieldsMask&CONSTANT" or "mFieldsMask & CONSTANT" patterns.
	// The text from GetText() has no spaces.
	idx := strings.Index(text, "mFieldsMask&")
	if idx < 0 {
		return ""
	}

	rest := text[idx+len("mFieldsMask&"):]

	// Extract the constant name (uppercase letters, digits, underscores).
	var name strings.Builder
	for _, ch := range rest {
		switch {
		case ch >= 'A' && ch <= 'Z', ch >= '0' && ch <= '9', ch == '_':
			name.WriteRune(ch)
		default:
			if name.Len() > 0 {
				return name.String()
			}
			return ""
		}
	}

	return name.String()
}

// parseIntExpression evaluates simple integer constant expressions like
// "1", "1<<0", "1<<3", "0x04000000".
func parseIntExpression(text string) int64 {
	text = strings.TrimSpace(text)

	// Handle shift expressions: "1 << N"
	if parts := strings.SplitN(text, "<<", 2); len(parts) == 2 {
		base := parseIntLiteral(strings.TrimSpace(parts[0]))
		shift := parseIntLiteral(strings.TrimSpace(parts[1]))
		return base << uint(shift)
	}

	return parseIntLiteral(text)
}

// parseIntLiteral parses a single integer literal (decimal or hex).
func parseIntLiteral(text string) int64 {
	text = strings.TrimSpace(text)

	if strings.HasPrefix(text, "0x") || strings.HasPrefix(text, "0X") {
		v, err := strconv.ParseInt(text[2:], 16, 64)
		if err != nil {
			return 0
		}
		return v
	}

	v, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return 0
	}
	return v
}

// combineConditions joins a parent condition with a new condition using " && ".
func combineConditions(parent, child string) string {
	if parent == "" {
		return child
	}
	return parent + " && " + child
}

// extractSimpleTypeName extracts the simple class name from a Java type
// expression like "Icon", "List<Foo>", "android.app.Icon" → "Icon".
func extractSimpleTypeName(text string) string {
	// Strip generic parameters.
	if idx := strings.IndexByte(text, '<'); idx != -1 {
		text = text[:idx]
	}
	// Strip array brackets.
	if idx := strings.IndexByte(text, '['); idx != -1 {
		text = text[:idx]
	}
	// Take last segment of qualified name.
	if idx := strings.LastIndexByte(text, '.'); idx != -1 {
		text = text[idx+1:]
	}
	return text
}
