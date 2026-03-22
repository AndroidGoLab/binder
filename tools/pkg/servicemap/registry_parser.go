package servicemap

import (
	antlr "github.com/antlr4-go/antlr/v4"

	"github.com/AndroidGoLab/binder/tools/pkg/javaparser"
)

// ExtractRegistrations parses a Java source string (typically SystemServiceRegistry.java)
// and returns the service registrations that use AIDL binder interfaces.
// Only registerService calls containing a Stub.asInterface invocation are included.
func ExtractRegistrations(src string) []Registration {
	input := antlr.NewInputStream(src)
	lexer := javaparser.NewJavaLexer(input)
	stream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	parser := javaparser.NewJavaParser(stream)

	// Suppress ANTLR error output during parsing.
	parser.RemoveErrorListeners()

	tree := parser.CompilationUnit()

	listener := newRegistrationListener()
	antlr.ParseTreeWalkerDefault.Walk(listener, tree)

	return listener.Registrations
}
