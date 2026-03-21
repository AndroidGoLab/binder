package parser

import (
	"fmt"
)

// Lexer tokenizes AIDL source code.
type Lexer struct {
	Src      []byte
	Offset   int
	Line     int
	Column   int
	Filename string
}

// NewLexer creates a new Lexer for the given source.
func NewLexer(
	filename string,
	src []byte,
) *Lexer {
	return &Lexer{
		Src:      src,
		Offset:   0,
		Line:     1,
		Column:   1,
		Filename: filename,
	}
}

func (l *Lexer) pos() Position {
	return Position{
		Filename: l.Filename,
		Line:     l.Line,
		Column:   l.Column,
	}
}

func (l *Lexer) peekAt(
	delta int,
) byte {
	idx := l.Offset + delta
	if idx >= len(l.Src) {
		return 0
	}
	return l.Src[idx]
}

func (l *Lexer) advance() byte {
	if l.Offset >= len(l.Src) {
		return 0
	}

	ch := l.Src[l.Offset]
	l.Offset++
	if ch == '\n' {
		l.Line++
		l.Column = 1
	} else {
		l.Column++
	}
	return ch
}

func (l *Lexer) skipWhitespace() {
	for l.Offset < len(l.Src) {
		ch := l.Src[l.Offset]
		if ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' {
			l.advance()
		} else {
			break
		}
	}
}

func (l *Lexer) skipLineComment() {
	for l.Offset < len(l.Src) && l.Src[l.Offset] != '\n' {
		l.advance()
	}
}

func (l *Lexer) skipBlockComment() error {
	startPos := l.pos()
	// Skip past the opening /*
	l.advance()
	l.advance()

	for l.Offset < len(l.Src) {
		if l.Src[l.Offset] == '*' && l.peekAt(1) == '/' {
			l.advance()
			l.advance()
			return nil
		}
		l.advance()
	}
	return fmt.Errorf("%s: unterminated block comment", startPos)
}

func (l *Lexer) skipWhitespaceAndComments() error {
	for {
		l.skipWhitespace()
		if l.Offset >= len(l.Src) {
			return nil
		}

		if l.Src[l.Offset] == '/' && l.peekAt(1) == '/' {
			l.skipLineComment()
			continue
		}

		if l.Src[l.Offset] == '/' && l.peekAt(1) == '*' {
			if err := l.skipBlockComment(); err != nil {
				return err
			}
			continue
		}

		return nil
	}
}

func isLetter(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_'
}

func isDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}

func isHexDigit(ch byte) bool {
	return isDigit(ch) || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')
}

func isBinaryDigit(ch byte) bool {
	return ch == '0' || ch == '1'
}

func isOctalDigit(ch byte) bool {
	return ch >= '0' && ch <= '7'
}

func isIdentChar(ch byte) bool {
	return isLetter(ch) || isDigit(ch)
}

// peekNextNonWS returns the next non-whitespace/non-comment byte in the source
// without advancing the lexer position. Returns 0 if only whitespace remains.
func (l *Lexer) peekNextNonWS() byte {
	off := l.Offset
	for off < len(l.Src) {
		ch := l.Src[off]
		if ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' {
			off++
			continue
		}
		if ch == '/' && off+1 < len(l.Src) {
			if l.Src[off+1] == '/' {
				for off < len(l.Src) && l.Src[off] != '\n' {
					off++
				}
				continue
			}
			if l.Src[off+1] == '*' {
				off += 2
				found := false
				for off+1 < len(l.Src) {
					if l.Src[off] == '*' && l.Src[off+1] == '/' {
						off += 2
						found = true
						break
					}
					off++
				}
				if !found {
					// Unterminated block comment: no more non-whitespace.
					return 0
				}
				continue
			}
		}
		return ch
	}
	return 0
}

func (l *Lexer) scanIdent() Token {
	p := l.pos()
	start := l.Offset
	for l.Offset < len(l.Src) && isIdentChar(l.Src[l.Offset]) {
		l.advance()
	}

	text := string(l.Src[start:l.Offset])
	if kind, ok := keywords[text]; ok {
		return Token{Kind: kind, Pos: p, Value: text}
	}
	return Token{Kind: TokenIdent, Pos: p, Value: text}
}

func (l *Lexer) scanNumber() Token {
	p := l.pos()
	start := l.Offset
	isFloat := false

	if l.Src[l.Offset] == '0' && l.Offset+1 < len(l.Src) {
		next := l.Src[l.Offset+1]

		// Hex: 0x or 0X
		if next == 'x' || next == 'X' {
			l.advance() // '0'
			l.advance() // 'x'
			for l.Offset < len(l.Src) && isHexDigit(l.Src[l.Offset]) {
				l.advance()
			}

			// Hex float: 0x1.Ap+3, 0xABp-2, etc.
			// A hex float has an optional fractional part (.) and a
			// mandatory binary exponent (p/P).
			isHexFloat := false
			if l.Offset < len(l.Src) && l.Src[l.Offset] == '.' {
				isHexFloat = true
				l.advance() // '.'
				for l.Offset < len(l.Src) && isHexDigit(l.Src[l.Offset]) {
					l.advance()
				}
			}
			if l.Offset < len(l.Src) && (l.Src[l.Offset] == 'p' || l.Src[l.Offset] == 'P') {
				isHexFloat = true
				l.advance() // 'p' or 'P'
				if l.Offset < len(l.Src) && (l.Src[l.Offset] == '+' || l.Src[l.Offset] == '-') {
					l.advance()
				}
				for l.Offset < len(l.Src) && isDigit(l.Src[l.Offset]) {
					l.advance()
				}
			}
			if isHexFloat {
				// Consume optional float suffix (f/F/d/D).
				if l.Offset < len(l.Src) {
					ch := l.Src[l.Offset]
					if ch == 'f' || ch == 'F' || ch == 'd' || ch == 'D' {
						l.advance()
					}
				}
				return Token{Kind: TokenFloatLiteral, Pos: p, Value: string(l.Src[start:l.Offset])}
			}

			l.consumeIntSuffix()
			return Token{Kind: TokenIntLiteral, Pos: p, Value: string(l.Src[start:l.Offset])}
		}

		// Binary: 0b or 0B
		if next == 'b' || next == 'B' {
			l.advance() // '0'
			l.advance() // 'b'
			for l.Offset < len(l.Src) && isBinaryDigit(l.Src[l.Offset]) {
				l.advance()
			}
			l.consumeIntSuffix()
			return Token{Kind: TokenIntLiteral, Pos: p, Value: string(l.Src[start:l.Offset])}
		}

		// Octal: starts with 0 and followed by octal digits
		if isOctalDigit(next) {
			l.advance() // '0'
			for l.Offset < len(l.Src) && isOctalDigit(l.Src[l.Offset]) {
				l.advance()
			}

			// Could transition to float if we see '.'
			if l.Offset < len(l.Src) && l.Src[l.Offset] == '.' {
				isFloat = true
				l.advance()
				for l.Offset < len(l.Src) && isDigit(l.Src[l.Offset]) {
					l.advance()
				}
			}

			if !isFloat {
				l.consumeIntSuffix()
				return Token{Kind: TokenIntLiteral, Pos: p, Value: string(l.Src[start:l.Offset])}
			}
		}
	}

	if !isFloat {
		// Decimal integer or float
		for l.Offset < len(l.Src) && isDigit(l.Src[l.Offset]) {
			l.advance()
		}

		if l.Offset < len(l.Src) && l.Src[l.Offset] == '.' {
			isFloat = true
			l.advance()
			for l.Offset < len(l.Src) && isDigit(l.Src[l.Offset]) {
				l.advance()
			}
		}
	}

	// Exponent
	if l.Offset < len(l.Src) && (l.Src[l.Offset] == 'e' || l.Src[l.Offset] == 'E') {
		isFloat = true
		l.advance()
		if l.Offset < len(l.Src) && (l.Src[l.Offset] == '+' || l.Src[l.Offset] == '-') {
			l.advance()
		}
		for l.Offset < len(l.Src) && isDigit(l.Src[l.Offset]) {
			l.advance()
		}
	}

	// Float suffix
	if l.Offset < len(l.Src) && (l.Src[l.Offset] == 'f' || l.Src[l.Offset] == 'F' || l.Src[l.Offset] == 'd' || l.Src[l.Offset] == 'D') {
		isFloat = true
		l.advance()
	}

	if isFloat {
		return Token{Kind: TokenFloatLiteral, Pos: p, Value: string(l.Src[start:l.Offset])}
	}

	l.consumeIntSuffix()
	return Token{Kind: TokenIntLiteral, Pos: p, Value: string(l.Src[start:l.Offset])}
}

func (l *Lexer) consumeIntSuffix() {
	if l.Offset >= len(l.Src) {
		return
	}

	ch := l.Src[l.Offset]

	// Long suffix: L or l.
	if ch == 'L' || ch == 'l' {
		l.advance()
		return
	}

	// Unsigned suffix: u8, u32, u64 (AIDL typed integer suffixes).
	if ch == 'u' {
		next := l.peekAt(1)
		if next == '8' {
			l.advance() // 'u'
			l.advance() // '8'
			return
		}
		if next == '1' && l.peekAt(2) == '6' {
			l.advance() // 'u'
			l.advance() // '1'
			l.advance() // '6'
			return
		}
		if next == '3' && l.peekAt(2) == '2' {
			l.advance() // 'u'
			l.advance() // '3'
			l.advance() // '2'
			return
		}
		if next == '6' && l.peekAt(2) == '4' {
			l.advance() // 'u'
			l.advance() // '6'
			l.advance() // '4'
			return
		}
	}

	// Signed suffix: i8, i32, i64 (AIDL typed integer suffixes).
	if ch == 'i' {
		next := l.peekAt(1)
		if next == '8' {
			l.advance() // 'i'
			l.advance() // '8'
			return
		}
		if next == '1' && l.peekAt(2) == '6' {
			l.advance() // 'i'
			l.advance() // '1'
			l.advance() // '6'
			return
		}
		if next == '3' && l.peekAt(2) == '2' {
			l.advance() // 'i'
			l.advance() // '3'
			l.advance() // '2'
			return
		}
		if next == '6' && l.peekAt(2) == '4' {
			l.advance() // 'i'
			l.advance() // '6'
			l.advance() // '4'
			return
		}
	}
}

func (l *Lexer) scanString() (Token, error) {
	p := l.pos()
	l.advance() // opening quote

	var buf []byte
	for l.Offset < len(l.Src) {
		ch := l.Src[l.Offset]
		if ch == '"' {
			l.advance()
			return Token{Kind: TokenStringLiteral, Pos: p, Value: string(buf)}, nil
		}

		if ch == '\\' {
			l.advance()
			if l.Offset >= len(l.Src) {
				return Token{}, fmt.Errorf("%s: unterminated string literal", p)
			}

			esc := l.Src[l.Offset]
			l.advance()
			switch esc {
			case 'n':
				buf = append(buf, '\n')
			case 't':
				buf = append(buf, '\t')
			case 'r':
				buf = append(buf, '\r')
			case '\\':
				buf = append(buf, '\\')
			case '"':
				buf = append(buf, '"')
			case '\'':
				buf = append(buf, '\'')
			case '0':
				buf = append(buf, 0)
			default:
				buf = append(buf, '\\', esc)
			}
			continue
		}

		if ch == '\n' {
			return Token{}, fmt.Errorf("%s: unterminated string literal", p)
		}

		buf = append(buf, ch)
		l.advance()
	}

	return Token{}, fmt.Errorf("%s: unterminated string literal", p)
}

func (l *Lexer) scanChar() (Token, error) {
	p := l.pos()
	l.advance() // opening quote

	var buf []byte
	for l.Offset < len(l.Src) {
		ch := l.Src[l.Offset]
		if ch == '\'' {
			l.advance()
			return Token{Kind: TokenCharLiteral, Pos: p, Value: string(buf)}, nil
		}

		if ch == '\\' {
			l.advance()
			if l.Offset >= len(l.Src) {
				return Token{}, fmt.Errorf("%s: unterminated char literal", p)
			}

			esc := l.Src[l.Offset]
			l.advance()
			switch esc {
			case 'n':
				buf = append(buf, '\n')
			case 't':
				buf = append(buf, '\t')
			case 'r':
				buf = append(buf, '\r')
			case '\\':
				buf = append(buf, '\\')
			case '\'':
				buf = append(buf, '\'')
			case '0':
				buf = append(buf, 0)
			default:
				buf = append(buf, '\\', esc)
			}
			continue
		}

		buf = append(buf, ch)
		l.advance()
	}

	return Token{}, fmt.Errorf("%s: unterminated char literal", p)
}

func (l *Lexer) scanAnnotation() Token {
	p := l.pos()
	l.advance() // '@'

	start := l.Offset
	for l.Offset < len(l.Src) && isIdentChar(l.Src[l.Offset]) {
		l.advance()
	}

	name := string(l.Src[start:l.Offset])
	return Token{Kind: TokenAnnotation, Pos: p, Value: name}
}

// Next returns the next token from the source.
// Returns TokenEOF at end of input. Returns an error token with Value
// containing the error message on lexer errors.
func (l *Lexer) Next() Token {
	if err := l.skipWhitespaceAndComments(); err != nil {
		return Token{Kind: TokenError, Pos: l.pos(), Value: err.Error()}
	}

	if l.Offset >= len(l.Src) {
		return Token{Kind: TokenEOF, Pos: l.pos()}
	}

	ch := l.Src[l.Offset]

	// Identifiers and keywords.
	if isLetter(ch) {
		return l.scanIdent()
	}

	// Numeric literals.
	if isDigit(ch) {
		return l.scanNumber()
	}

	// String literals.
	if ch == '"' {
		tok, err := l.scanString()
		if err != nil {
			return Token{Kind: TokenError, Pos: l.pos(), Value: err.Error()}
		}
		return tok
	}

	// Char literals.
	if ch == '\'' {
		tok, err := l.scanChar()
		if err != nil {
			return Token{Kind: TokenError, Pos: l.pos(), Value: err.Error()}
		}
		return tok
	}

	// Annotations.
	if ch == '@' {
		return l.scanAnnotation()
	}

	// Two-character operators.
	p := l.pos()
	next := l.peekAt(1)

	switch ch {
	case '<':
		if next == '<' {
			l.advance()
			l.advance()
			return Token{Kind: TokenLShift, Pos: p, Value: "<<"}
		}
		if next == '=' {
			l.advance()
			l.advance()
			return Token{Kind: TokenLessEq, Pos: p, Value: "<="}
		}
		l.advance()
		return Token{Kind: TokenLAngle, Pos: p, Value: "<"}

	case '>':
		if next == '>' {
			l.advance()
			l.advance()
			return Token{Kind: TokenRShift, Pos: p, Value: ">>"}
		}
		if next == '=' {
			l.advance()
			l.advance()
			return Token{Kind: TokenGreaterEq, Pos: p, Value: ">="}
		}
		l.advance()
		return Token{Kind: TokenRAngle, Pos: p, Value: ">"}

	case '=':
		if next == '=' {
			l.advance()
			l.advance()
			return Token{Kind: TokenEqEq, Pos: p, Value: "=="}
		}
		l.advance()
		return Token{Kind: TokenAssign, Pos: p, Value: "="}

	case '!':
		if next == '=' {
			l.advance()
			l.advance()
			return Token{Kind: TokenBangEq, Pos: p, Value: "!="}
		}
		l.advance()
		return Token{Kind: TokenBang, Pos: p, Value: "!"}

	case '&':
		if next == '&' {
			l.advance()
			l.advance()
			return Token{Kind: TokenAmpAmp, Pos: p, Value: "&&"}
		}
		l.advance()
		return Token{Kind: TokenAmp, Pos: p, Value: "&"}

	case '|':
		if next == '|' {
			l.advance()
			l.advance()
			return Token{Kind: TokenPipePipe, Pos: p, Value: "||"}
		}
		l.advance()
		return Token{Kind: TokenPipe, Pos: p, Value: "|"}
	}

	// Single-character tokens.
	l.advance()
	switch ch {
	case '{':
		return Token{Kind: TokenLBrace, Pos: p, Value: "{"}
	case '}':
		return Token{Kind: TokenRBrace, Pos: p, Value: "}"}
	case '(':
		return Token{Kind: TokenLParen, Pos: p, Value: "("}
	case ')':
		return Token{Kind: TokenRParen, Pos: p, Value: ")"}
	case '[':
		return Token{Kind: TokenLBracket, Pos: p, Value: "["}
	case ']':
		return Token{Kind: TokenRBracket, Pos: p, Value: "]"}
	case ';':
		return Token{Kind: TokenSemicolon, Pos: p, Value: ";"}
	case ',':
		return Token{Kind: TokenComma, Pos: p, Value: ","}
	case '.':
		return Token{Kind: TokenDot, Pos: p, Value: "."}
	case '+':
		return Token{Kind: TokenPlus, Pos: p, Value: "+"}
	case '-':
		return Token{Kind: TokenMinus, Pos: p, Value: "-"}
	case '*':
		return Token{Kind: TokenStar, Pos: p, Value: "*"}
	case '/':
		return Token{Kind: TokenSlash, Pos: p, Value: "/"}
	case '%':
		return Token{Kind: TokenPercent, Pos: p, Value: "%"}
	case '^':
		return Token{Kind: TokenCaret, Pos: p, Value: "^"}
	case '~':
		return Token{Kind: TokenTilde, Pos: p, Value: "~"}
	case '?':
		return Token{Kind: TokenQuestion, Pos: p, Value: "?"}
	case ':':
		return Token{Kind: TokenColon, Pos: p, Value: ":"}
	}

	return Token{Kind: TokenError, Pos: p, Value: fmt.Sprintf("unexpected character: %c", ch)}
}
