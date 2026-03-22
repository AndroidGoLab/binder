package servicemap

import (
	antlr "github.com/antlr4-go/antlr/v4"

	"github.com/AndroidGoLab/binder/tools/pkg/javaparser"
)

// ExtractContextConstants parses a Java source string and returns a map
// of constant name to service name value for all fields matching
// the pattern: public static final String XXX_SERVICE = "value".
func ExtractContextConstants(src string) map[string]string {
	input := antlr.NewInputStream(src)
	lexer := javaparser.NewJavaLexer(input)
	stream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	parser := javaparser.NewJavaParser(stream)

	// Suppress ANTLR error output during parsing.
	parser.RemoveErrorListeners()

	tree := parser.CompilationUnit()

	listener := newServiceConstantListener()
	antlr.ParseTreeWalkerDefault.Walk(listener, tree)

	return listener.Constants
}
