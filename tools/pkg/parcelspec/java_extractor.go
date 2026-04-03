package parcelspec

import (
	"unicode"

	antlr "github.com/antlr4-go/antlr/v4"

	"github.com/AndroidGoLab/binder/tools/pkg/javaparser"
)

// JavaExtractor reuses an ANTLR lexer/parser across multiple files.
// The DFA cache built by the ATN simulator persists across calls,
// dramatically reducing allocation and GC pressure.
type JavaExtractor struct {
	lexer  *javaparser.JavaLexer
	stream *antlr.CommonTokenStream
	parser *javaparser.JavaParser
}

// NewJavaExtractor creates a reusable extractor.
func NewJavaExtractor() *JavaExtractor {
	// Initialize with empty input; will be reset per file.
	input := antlr.NewInputStream("")
	lexer := javaparser.NewJavaLexer(input)
	stream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	parser := javaparser.NewJavaParser(stream)
	parser.RemoveErrorListeners()

	return &JavaExtractor{
		lexer:  lexer,
		stream: stream,
		parser: parser,
	}
}

// ExtractSpecs parses a Java source file and returns ParcelableSpecs
// for each class that contains a writeToParcel method.
// Reuses the internal lexer/parser for DFA cache efficiency.
func (e *JavaExtractor) ExtractSpecs(
	javaSrc string,
	packageName string,
) []ParcelableSpec {
	input := antlr.NewInputStream(javaSrc)
	e.lexer.SetInputStream(input)
	e.stream.SetTokenSource(e.lexer)
	e.parser.SetTokenStream(e.stream)

	tree := e.parser.CompilationUnit()

	listener := newParcelableListener(packageName)
	antlr.ParseTreeWalkerDefault.Walk(listener, tree)

	return listener.specs
}

// ExtractSpecs is a convenience function that creates a one-shot extractor.
// For batch processing, use NewJavaExtractor() and call its ExtractSpecs method.
func ExtractSpecs(
	javaSrc string,
	packageName string,
) []ParcelableSpec {
	return NewJavaExtractor().ExtractSpecs(javaSrc, packageName)
}

// javaWriteMethodToSpecType maps Java Parcel write method names
// to their corresponding spec type strings.
var javaWriteMethodToSpecType = map[string]string{
	"writeString8":      "string8",
	"writeString":       "string16",
	"writeString16":     "string16",
	"writeInt":          "int32",
	"writeLong":         "int64",
	"writeFloat":        "float32",
	"writeDouble":       "float64",
	"writeBoolean":      "bool",
	"writeBundle":       "bundle",
	"writeParcelable":   "typed_object",
	"writeTypedObject":  "typed_object",
	"writeByte":         "int32",
	"writeByteArray":    "byte_array",
	"writeIntArray":     "int_array",
	"writeLongArray":    "long_array",
	"writeFloatArray":   "float_array",
	"writeDoubleArray":  "double_array",
	"writeBooleanArray": "boolean_array",
	"writeStringArray":  "string_array",
	"writeBlob":         "blob",
	"writeStrongBinder": "binder",
	"writeList":         "write_list",
	"writeTypedArray":   "typed_array",
	"writeArraySet":     "array_set",
}

// deriveFieldName converts a Java field name to a spec field name
// by stripping the leading "m" prefix convention.
func deriveFieldName(javaFieldName string) string {
	if len(javaFieldName) < 2 {
		return javaFieldName
	}

	// Strip leading "m" prefix if followed by an uppercase letter.
	if javaFieldName[0] == 'm' && unicode.IsUpper(rune(javaFieldName[1])) {
		return javaFieldName[1:]
	}

	// Capitalize first letter.
	return string(unicode.ToUpper(rune(javaFieldName[0]))) + javaFieldName[1:]
}
