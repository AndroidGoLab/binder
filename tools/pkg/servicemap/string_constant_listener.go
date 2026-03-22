package servicemap

import (
	"github.com/AndroidGoLab/binder/tools/pkg/javaparser"
)

// stringConstantListener walks a Java AST and collects
// all public static final String fields with string literal values.
type stringConstantListener struct {
	javaparser.BaseJavaParserListener

	// Constants maps constant name to its string value.
	Constants map[string]string
}

func newStringConstantListener() *stringConstantListener {
	return &stringConstantListener{
		Constants: make(map[string]string),
	}
}

// EnterFieldDeclaration inspects each field declaration to find
// public static final String constants with string literal values.
func (l *stringConstantListener) EnterFieldDeclaration(ctx *javaparser.FieldDeclarationContext) {
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
