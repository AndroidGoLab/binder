package parser

// TokenKind identifies the type of a lexical token.
type TokenKind int

const (
	TokenEOF TokenKind = iota
	TokenError
	TokenIdent
	TokenIntLiteral
	TokenFloatLiteral
	TokenStringLiteral
	TokenCharLiteral

	// Keywords.
	TokenPackage
	TokenImport
	TokenInterface
	TokenParcelable
	TokenEnum
	TokenUnion
	TokenConst
	TokenOneway
	TokenIn
	TokenOut
	TokenInout
	TokenTrue
	TokenFalse
	TokenVoid
	TokenNull

	// Punctuation.
	TokenLBrace
	TokenRBrace
	TokenLParen
	TokenRParen
	TokenLBracket
	TokenRBracket
	TokenSemicolon
	TokenComma
	TokenDot
	TokenAssign
	TokenLAngle
	TokenRAngle

	// Operators.
	TokenPlus
	TokenMinus
	TokenStar
	TokenSlash
	TokenPercent
	TokenAmp
	TokenPipe
	TokenCaret
	TokenTilde
	TokenBang
	TokenLShift
	TokenRShift
	TokenAmpAmp
	TokenPipePipe
	TokenEqEq
	TokenBangEq
	TokenLessEq
	TokenGreaterEq
	TokenQuestion
	TokenColon

	// Annotations.
	TokenAnnotation
)

var tokenKindNames = map[TokenKind]string{
	TokenEOF:           "EOF",
	TokenError:         "error",
	TokenIdent:         "identifier",
	TokenIntLiteral:    "integer",
	TokenFloatLiteral:  "float",
	TokenStringLiteral: "string",
	TokenCharLiteral:   "char",
	TokenPackage:       "package",
	TokenImport:        "import",
	TokenInterface:     "interface",
	TokenParcelable:    "parcelable",
	TokenEnum:          "enum",
	TokenUnion:         "union",
	TokenConst:         "const",
	TokenOneway:        "oneway",
	TokenIn:            "in",
	TokenOut:           "out",
	TokenInout:         "inout",
	TokenTrue:          "true",
	TokenFalse:         "false",
	TokenVoid:          "void",
	TokenNull:          "null",
	TokenLBrace:        "{",
	TokenRBrace:        "}",
	TokenLParen:        "(",
	TokenRParen:        ")",
	TokenLBracket:      "[",
	TokenRBracket:      "]",
	TokenSemicolon:     ";",
	TokenComma:         ",",
	TokenDot:           ".",
	TokenAssign:        "=",
	TokenLAngle:        "<",
	TokenRAngle:        ">",
	TokenPlus:          "+",
	TokenMinus:         "-",
	TokenStar:          "*",
	TokenSlash:         "/",
	TokenPercent:       "%",
	TokenAmp:           "&",
	TokenPipe:          "|",
	TokenCaret:         "^",
	TokenTilde:         "~",
	TokenBang:          "!",
	TokenLShift:        "<<",
	TokenRShift:        ">>",
	TokenAmpAmp:        "&&",
	TokenPipePipe:      "||",
	TokenEqEq:          "==",
	TokenBangEq:        "!=",
	TokenLessEq:        "<=",
	TokenGreaterEq:     ">=",
	TokenQuestion:      "?",
	TokenColon:         ":",
	TokenAnnotation:    "annotation",
}

// String returns a human-readable name for the token kind.
func (k TokenKind) String() string {
	if name, ok := tokenKindNames[k]; ok {
		return name
	}
	return "unknown"
}

var keywords = map[string]TokenKind{
	"package":    TokenPackage,
	"import":     TokenImport,
	"interface":  TokenInterface,
	"parcelable": TokenParcelable,
	"enum":       TokenEnum,
	"union":      TokenUnion,
	"const":      TokenConst,
	"oneway":     TokenOneway,
	"in":         TokenIn,
	"out":        TokenOut,
	"inout":      TokenInout,
	"true":       TokenTrue,
	"false":      TokenFalse,
	"void":       TokenVoid,
	"null":       TokenNull,
}

// Token represents a single lexical token from AIDL source.
type Token struct {
	Kind  TokenKind
	Pos   Position
	Value string
}
