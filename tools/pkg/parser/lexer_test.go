package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLexerKeywords(t *testing.T) {
	src := `package import interface parcelable enum union const oneway in out inout true false void null`
	lex := NewLexer("test.aidl", []byte(src))

	expected := []TokenKind{
		TokenPackage, TokenImport, TokenInterface, TokenParcelable,
		TokenEnum, TokenUnion, TokenConst, TokenOneway,
		TokenIn, TokenOut, TokenInout, TokenTrue, TokenFalse,
		TokenVoid, TokenNull, TokenEOF,
	}

	for _, kind := range expected {
		tok := lex.Next()
		assert.Equal(t, kind, tok.Kind, "expected %s, got %s (%q)", kind, tok.Kind, tok.Value)
	}
}

func TestLexerIdentifiers(t *testing.T) {
	src := `foo Bar _baz abc123`
	lex := NewLexer("test.aidl", []byte(src))

	expectedNames := []string{"foo", "Bar", "_baz", "abc123"}
	for _, name := range expectedNames {
		tok := lex.Next()
		assert.Equal(t, TokenIdent, tok.Kind)
		assert.Equal(t, name, tok.Value)
	}

	tok := lex.Next()
	assert.Equal(t, TokenEOF, tok.Kind)
}

func TestLexerDecimalNumbers(t *testing.T) {
	src := `0 42 1234567890`
	lex := NewLexer("test.aidl", []byte(src))

	expectedValues := []string{"0", "42", "1234567890"}
	for _, val := range expectedValues {
		tok := lex.Next()
		assert.Equal(t, TokenIntLiteral, tok.Kind)
		assert.Equal(t, val, tok.Value)
	}
}

func TestLexerHexLiterals(t *testing.T) {
	src := `0xFF 0X1A 0x0`
	lex := NewLexer("test.aidl", []byte(src))

	expectedValues := []string{"0xFF", "0X1A", "0x0"}
	for _, val := range expectedValues {
		tok := lex.Next()
		assert.Equal(t, TokenIntLiteral, tok.Kind, "for value %q", val)
		assert.Equal(t, val, tok.Value)
	}
}

func TestLexerOctalLiterals(t *testing.T) {
	src := `077 0123`
	lex := NewLexer("test.aidl", []byte(src))

	expectedValues := []string{"077", "0123"}
	for _, val := range expectedValues {
		tok := lex.Next()
		assert.Equal(t, TokenIntLiteral, tok.Kind)
		assert.Equal(t, val, tok.Value)
	}
}

func TestLexerBinaryLiterals(t *testing.T) {
	src := `0b1010 0B110`
	lex := NewLexer("test.aidl", []byte(src))

	expectedValues := []string{"0b1010", "0B110"}
	for _, val := range expectedValues {
		tok := lex.Next()
		assert.Equal(t, TokenIntLiteral, tok.Kind)
		assert.Equal(t, val, tok.Value)
	}
}

func TestLexerIntSuffix(t *testing.T) {
	src := `42L 100l`
	lex := NewLexer("test.aidl", []byte(src))

	expectedValues := []string{"42L", "100l"}
	for _, val := range expectedValues {
		tok := lex.Next()
		assert.Equal(t, TokenIntLiteral, tok.Kind)
		assert.Equal(t, val, tok.Value)
	}
}

func TestLexerFloatLiterals(t *testing.T) {
	src := `3.14 1.0e10 2.5E-3 1.0f 2.0d`
	lex := NewLexer("test.aidl", []byte(src))

	expectedValues := []string{"3.14", "1.0e10", "2.5E-3", "1.0f", "2.0d"}
	for _, val := range expectedValues {
		tok := lex.Next()
		assert.Equal(t, TokenFloatLiteral, tok.Kind, "for value %q got %s", val, tok.Kind)
		assert.Equal(t, val, tok.Value)
	}
}

func TestLexerStringLiterals(t *testing.T) {
	src := `"hello" "world\n" "tab\there" "quote\"inside" ""`
	lex := NewLexer("test.aidl", []byte(src))

	expectedValues := []string{"hello", "world\n", "tab\there", "quote\"inside", ""}
	for _, val := range expectedValues {
		tok := lex.Next()
		assert.Equal(t, TokenStringLiteral, tok.Kind)
		assert.Equal(t, val, tok.Value)
	}
}

func TestLexerCharLiterals(t *testing.T) {
	src := `'a' '\n' '\\'`
	lex := NewLexer("test.aidl", []byte(src))

	expectedValues := []string{"a", "\n", "\\"}
	for _, val := range expectedValues {
		tok := lex.Next()
		assert.Equal(t, TokenCharLiteral, tok.Kind)
		assert.Equal(t, val, tok.Value)
	}
}

func TestLexerAnnotations(t *testing.T) {
	src := `@nullable @utf8InCpp @Backing @JavaPassthrough`
	lex := NewLexer("test.aidl", []byte(src))

	expectedNames := []string{"nullable", "utf8InCpp", "Backing", "JavaPassthrough"}
	for _, name := range expectedNames {
		tok := lex.Next()
		assert.Equal(t, TokenAnnotation, tok.Kind)
		assert.Equal(t, name, tok.Value)
	}
}

func TestLexerPunctuation(t *testing.T) {
	src := `{ } ( ) [ ] ; , . = < >`
	lex := NewLexer("test.aidl", []byte(src))

	expected := []TokenKind{
		TokenLBrace, TokenRBrace, TokenLParen, TokenRParen,
		TokenLBracket, TokenRBracket, TokenSemicolon, TokenComma,
		TokenDot, TokenAssign, TokenLAngle, TokenRAngle, TokenEOF,
	}

	for _, kind := range expected {
		tok := lex.Next()
		assert.Equal(t, kind, tok.Kind, "expected %s, got %s (%q)", kind, tok.Kind, tok.Value)
	}
}

func TestLexerSingleCharOperators(t *testing.T) {
	src := `+ - * / % & | ^ ~ ! ? :`
	lex := NewLexer("test.aidl", []byte(src))

	expected := []TokenKind{
		TokenPlus, TokenMinus, TokenStar, TokenSlash, TokenPercent,
		TokenAmp, TokenPipe, TokenCaret, TokenTilde, TokenBang,
		TokenQuestion, TokenColon, TokenEOF,
	}

	for _, kind := range expected {
		tok := lex.Next()
		assert.Equal(t, kind, tok.Kind, "expected %s, got %s (%q)", kind, tok.Kind, tok.Value)
	}
}

func TestLexerTwoCharOperators(t *testing.T) {
	src := `<< >> && || == != <= >=`
	lex := NewLexer("test.aidl", []byte(src))

	expected := []TokenKind{
		TokenLShift, TokenRShift, TokenAmpAmp, TokenPipePipe,
		TokenEqEq, TokenBangEq, TokenLessEq, TokenGreaterEq, TokenEOF,
	}

	for _, kind := range expected {
		tok := lex.Next()
		assert.Equal(t, kind, tok.Kind, "expected %s, got %s (%q)", kind, tok.Kind, tok.Value)
	}
}

func TestLexerCommentsSkipped(t *testing.T) {
	src := `foo // this is a comment
bar /* block comment */ baz
/* multi
   line
   comment */
qux`
	lex := NewLexer("test.aidl", []byte(src))

	expectedNames := []string{"foo", "bar", "baz", "qux"}
	for _, name := range expectedNames {
		tok := lex.Next()
		assert.Equal(t, TokenIdent, tok.Kind, "expected ident %q", name)
		assert.Equal(t, name, tok.Value)
	}
}

func TestLexerPositionTracking(t *testing.T) {
	src := "foo\nbar baz"
	lex := NewLexer("test.aidl", []byte(src))

	tok := lex.Next()
	assert.Equal(t, "foo", tok.Value)
	assert.Equal(t, 1, tok.Pos.Line)
	assert.Equal(t, 1, tok.Pos.Column)

	tok = lex.Next()
	assert.Equal(t, "bar", tok.Value)
	assert.Equal(t, 2, tok.Pos.Line)
	assert.Equal(t, 1, tok.Pos.Column)

	tok = lex.Next()
	assert.Equal(t, "baz", tok.Value)
	assert.Equal(t, 2, tok.Pos.Line)
	assert.Equal(t, 5, tok.Pos.Column)
}

func TestLexerPositionWithFilename(t *testing.T) {
	lex := NewLexer("myfile.aidl", []byte("x"))
	tok := lex.Next()
	require.Equal(t, "myfile.aidl", tok.Pos.Filename)
	assert.Equal(t, "myfile.aidl:1:1", tok.Pos.String())
}

func TestLexerPositionWithoutFilename(t *testing.T) {
	lex := NewLexer("", []byte("x"))
	tok := lex.Next()
	assert.Equal(t, "1:1", tok.Pos.String())
}

func TestLexerEmptyInput(t *testing.T) {
	lex := NewLexer("test.aidl", []byte(""))
	tok := lex.Next()
	assert.Equal(t, TokenEOF, tok.Kind)
}

func TestLexerUnexpectedChar(t *testing.T) {
	lex := NewLexer("test.aidl", []byte("$"))
	tok := lex.Next()
	assert.Equal(t, TokenError, tok.Kind)
	assert.Contains(t, tok.Value, "unexpected character")
}

func TestLexerStringEscapes(t *testing.T) {
	src := `"\r" "\0" "\\"`
	lex := NewLexer("test.aidl", []byte(src))

	tok := lex.Next()
	assert.Equal(t, "\r", tok.Value)
	tok = lex.Next()
	assert.Equal(t, "\x00", tok.Value)
	tok = lex.Next()
	assert.Equal(t, "\\", tok.Value)
}

func TestLexerStringUnknownEscape(t *testing.T) {
	src := `"\z"`
	lex := NewLexer("test.aidl", []byte(src))
	tok := lex.Next()
	assert.Equal(t, TokenStringLiteral, tok.Kind)
	assert.Equal(t, "\\z", tok.Value)
}

func TestLexerCharEscapes(t *testing.T) {
	src := `'\r' '\0' '\t' '\''`
	lex := NewLexer("test.aidl", []byte(src))

	tok := lex.Next()
	assert.Equal(t, TokenCharLiteral, tok.Kind)
	assert.Equal(t, "\r", tok.Value)

	tok = lex.Next()
	assert.Equal(t, "\x00", tok.Value)

	tok = lex.Next()
	assert.Equal(t, "\t", tok.Value)

	tok = lex.Next()
	assert.Equal(t, "'", tok.Value)
}

func TestLexerCharUnknownEscape(t *testing.T) {
	src := `'\z'`
	lex := NewLexer("test.aidl", []byte(src))
	tok := lex.Next()
	assert.Equal(t, TokenCharLiteral, tok.Kind)
	assert.Equal(t, "\\z", tok.Value)
}

func TestLexerFloatExponentOnly(t *testing.T) {
	src := `1e5`
	lex := NewLexer("test.aidl", []byte(src))
	tok := lex.Next()
	assert.Equal(t, TokenFloatLiteral, tok.Kind)
	assert.Equal(t, "1e5", tok.Value)
}

func TestLexerFloatExponentWithSign(t *testing.T) {
	src := `2E+10`
	lex := NewLexer("test.aidl", []byte(src))
	tok := lex.Next()
	assert.Equal(t, TokenFloatLiteral, tok.Kind)
	assert.Equal(t, "2E+10", tok.Value)
}

func TestLexerTokenKindUnknown(t *testing.T) {
	assert.Equal(t, "unknown", TokenKind(-1).String())
}

func TestLexerAdvancePastEOF(t *testing.T) {
	lex := NewLexer("test.aidl", []byte(""))
	// Calling advance on exhausted input returns 0 without panic.
	assert.Equal(t, byte(0), lex.advance())
}

func TestLexerOnlyWhitespace(t *testing.T) {
	lex := NewLexer("test.aidl", []byte("   \t\n  "))
	tok := lex.Next()
	assert.Equal(t, TokenEOF, tok.Kind)
}

func TestLexerSlashNotComment(t *testing.T) {
	// A lone slash should be TokenSlash, not start a comment.
	lex := NewLexer("test.aidl", []byte("3 / 2"))
	tok := lex.Next()
	assert.Equal(t, TokenIntLiteral, tok.Kind)
	tok = lex.Next()
	assert.Equal(t, TokenSlash, tok.Kind)
	tok = lex.Next()
	assert.Equal(t, TokenIntLiteral, tok.Kind)
}

func TestLexerCompleteAIDL(t *testing.T) {
	src := `package com.example;
interface IFoo {
    void bar(in String s);
}`
	lex := NewLexer("test.aidl", []byte(src))

	expected := []struct {
		kind  TokenKind
		value string
	}{
		{TokenPackage, "package"},
		{TokenIdent, "com"},
		{TokenDot, "."},
		{TokenIdent, "example"},
		{TokenSemicolon, ";"},
		{TokenInterface, "interface"},
		{TokenIdent, "IFoo"},
		{TokenLBrace, "{"},
		{TokenVoid, "void"},
		{TokenIdent, "bar"},
		{TokenLParen, "("},
		{TokenIn, "in"},
		{TokenIdent, "String"},
		{TokenIdent, "s"},
		{TokenRParen, ")"},
		{TokenSemicolon, ";"},
		{TokenRBrace, "}"},
		{TokenEOF, ""},
	}

	for i, e := range expected {
		tok := lex.Next()
		assert.Equal(t, e.kind, tok.Kind, "token %d: expected %s, got %s (%q)", i, e.kind, tok.Kind, tok.Value)
		assert.Equal(t, e.value, tok.Value, "token %d: value mismatch", i)
	}
}
