package parser

// Token represents a single lexical token from AIDL source.
type Token struct {
	Kind  TokenKind
	Pos   Position
	Value string
}
