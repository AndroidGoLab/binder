package servicemap

import (
	"strings"

	"github.com/AndroidGoLab/binder/tools/pkg/javaparser"
)

// serviceConstantListener walks a Java AST and collects
// public static final String fields whose names end with _SERVICE.
type serviceConstantListener struct {
	javaparser.BaseJavaParserListener

	// Constants maps constant name to its string value.
	Constants map[string]string
}

func newServiceConstantListener() *serviceConstantListener {
	return &serviceConstantListener{
		Constants: make(map[string]string),
	}
}

// EnterFieldDeclaration inspects each field declaration to find
// public static final String constants with string literal values.
func (l *serviceConstantListener) EnterFieldDeclaration(ctx *javaparser.FieldDeclarationContext) {
	typeName := ctx.TypeType().GetText()
	if typeName != "String" {
		return
	}

	if !isPublicStaticFinal(ctx) {
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
		if !strings.HasSuffix(name, "_SERVICE") {
			continue
		}

		init := decl.VariableInitializer()
		if init == nil {
			continue
		}

		value := extractStringLiteral(init.GetText())
		if value == "" {
			continue
		}

		l.Constants[name] = value
	}
}

// isPublicStaticFinal checks whether the field's enclosing classBodyDeclaration
// has public, static, and final modifiers.
func isPublicStaticFinal(ctx *javaparser.FieldDeclarationContext) bool {
	// FieldDeclaration -> MemberDeclaration -> ClassBodyDeclaration
	memberCtx := ctx.GetParent()
	if memberCtx == nil {
		return false
	}

	classBodyDecl, ok := memberCtx.GetParent().(*javaparser.ClassBodyDeclarationContext)
	if !ok {
		return false
	}

	var hasPublic, hasStatic, hasFinal bool
	for _, mod := range classBodyDecl.AllModifier() {
		coiMod := mod.ClassOrInterfaceModifier()
		if coiMod == nil {
			continue
		}

		switch {
		case coiMod.PUBLIC() != nil:
			hasPublic = true
		case coiMod.STATIC() != nil:
			hasStatic = true
		case coiMod.FINAL() != nil:
			hasFinal = true
		}
	}

	return hasPublic && hasStatic && hasFinal
}

// extractStringLiteral strips the surrounding quotes from a Java string literal.
// Returns empty string if the text is not a quoted string.
func extractStringLiteral(text string) string {
	if len(text) < 2 {
		return ""
	}

	if text[0] != '"' || text[len(text)-1] != '"' {
		return ""
	}

	return text[1 : len(text)-1]
}
