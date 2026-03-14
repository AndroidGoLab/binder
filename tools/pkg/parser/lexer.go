package parser

import (
	"fmt"
)

// Lexer tokenizes AIDL source code.
type Lexer struct {
	src      []byte
	offset   int
	line     int
	column   int
	filename string
}

// NewLexer creates a new Lexer for the given source.
func NewLexer(
	filename string,
	src []byte,
) *Lexer {
	return &Lexer{
		src:      src,
		offset:   0,
		line:     1,
		column:   1,
		filename: filename,
	}
}

func (l *Lexer) pos() Position {
	return Position{
		Filename: l.filename,
		Line:     l.line,
		Column:   l.column,
	}
}

func (l *Lexer) peekAt(
	delta int,
) byte {
	idx := l.offset + delta
	if idx >= len(l.src) {
		return 0
	}
	return l.src[idx]
}

func (l *Lexer) advance() byte {
	if l.offset >= len(l.src) {
		return 0
	}

	ch := l.src[l.offset]
	l.offset++
	if ch == '\n' {
		l.line++
		l.column = 1
	} else {
		l.column++
	}
	return ch
}

func (l *Lexer) skipWhitespace() {
	for l.offset < len(l.src) {
		ch := l.src[l.offset]
		if ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' {
			l.advance()
		} else {
			break
		}
	}
}

func (l *Lexer) skipLineComment() {
	for l.offset < len(l.src) && l.src[l.offset] != '\n' {
		l.advance()
	}
}

func (l *Lexer) skipBlockComment() error {
	startPos := l.pos()
	// Skip past the opening /*
	l.advance()
	l.advance()

	for l.offset < len(l.src) {
		if l.src[l.offset] == '*' && l.peekAt(1) == '/' {
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
		if l.offset >= len(l.src) {
			return nil
		}

		if l.src[l.offset] == '/' && l.peekAt(1) == '/' {
			l.skipLineComment()
			continue
		}

		if l.src[l.offset] == '/' && l.peekAt(1) == '*' {
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
	off := l.offset
	for off < len(l.src) {
		ch := l.src[off]
		if ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' {
			off++
			continue
		}
		if ch == '/' && off+1 < len(l.src) {
			if l.src[off+1] == '/' {
				for off < len(l.src) && l.src[off] != '\n' {
					off++
				}
				continue
			}
			if l.src[off+1] == '*' {
				off += 2
				for off+1 < len(l.src) {
					if l.src[off] == '*' && l.src[off+1] == '/' {
						off += 2
						break
					}
					off++
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
	start := l.offset
	for l.offset < len(l.src) && isIdentChar(l.src[l.offset]) {
		l.advance()
	}

	text := string(l.src[start:l.offset])
	if kind, ok := keywords[text]; ok {
		return Token{Kind: kind, Pos: p, Value: text}
	}
	return Token{Kind: TokenIdent, Pos: p, Value: text}
}

func (l *Lexer) scanNumber() Token {
	p := l.pos()
	start := l.offset
	isFloat := false

	if l.src[l.offset] == '0' && l.offset+1 < len(l.src) {
		next := l.src[l.offset+1]

		// Hex: 0x or 0X
		if next == 'x' || next == 'X' {
			l.advance() // '0'
			l.advance() // 'x'
			for l.offset < len(l.src) && isHexDigit(l.src[l.offset]) {
				l.advance()
			}
			l.consumeIntSuffix()
			return Token{Kind: TokenIntLiteral, Pos: p, Value: string(l.src[start:l.offset])}
		}

		// Binary: 0b or 0B
		if next == 'b' || next == 'B' {
			l.advance() // '0'
			l.advance() // 'b'
			for l.offset < len(l.src) && isBinaryDigit(l.src[l.offset]) {
				l.advance()
			}
			l.consumeIntSuffix()
			return Token{Kind: TokenIntLiteral, Pos: p, Value: string(l.src[start:l.offset])}
		}

		// Octal: starts with 0 and followed by octal digits
		if isOctalDigit(next) {
			l.advance() // '0'
			for l.offset < len(l.src) && isOctalDigit(l.src[l.offset]) {
				l.advance()
			}

			// Could transition to float if we see '.'
			if l.offset < len(l.src) && l.src[l.offset] == '.' {
				isFloat = true
				l.advance()
				for l.offset < len(l.src) && isDigit(l.src[l.offset]) {
					l.advance()
				}
			}

			if !isFloat {
				l.consumeIntSuffix()
				return Token{Kind: TokenIntLiteral, Pos: p, Value: string(l.src[start:l.offset])}
			}
		}
	}

	if !isFloat {
		// Decimal integer or float
		for l.offset < len(l.src) && isDigit(l.src[l.offset]) {
			l.advance()
		}

		if l.offset < len(l.src) && l.src[l.offset] == '.' {
			isFloat = true
			l.advance()
			for l.offset < len(l.src) && isDigit(l.src[l.offset]) {
				l.advance()
			}
		}
	}

	// Exponent
	if l.offset < len(l.src) && (l.src[l.offset] == 'e' || l.src[l.offset] == 'E') {
		isFloat = true
		l.advance()
		if l.offset < len(l.src) && (l.src[l.offset] == '+' || l.src[l.offset] == '-') {
			l.advance()
		}
		for l.offset < len(l.src) && isDigit(l.src[l.offset]) {
			l.advance()
		}
	}

	// Float suffix
	if l.offset < len(l.src) && (l.src[l.offset] == 'f' || l.src[l.offset] == 'F' || l.src[l.offset] == 'd' || l.src[l.offset] == 'D') {
		isFloat = true
		l.advance()
	}

	if isFloat {
		return Token{Kind: TokenFloatLiteral, Pos: p, Value: string(l.src[start:l.offset])}
	}

	l.consumeIntSuffix()
	return Token{Kind: TokenIntLiteral, Pos: p, Value: string(l.src[start:l.offset])}
}

func (l *Lexer) consumeIntSuffix() {
	if l.offset >= len(l.src) {
		return
	}

	ch := l.src[l.offset]

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
	for l.offset < len(l.src) {
		ch := l.src[l.offset]
		if ch == '"' {
			l.advance()
			return Token{Kind: TokenStringLiteral, Pos: p, Value: string(buf)}, nil
		}

		if ch == '\\' {
			l.advance()
			if l.offset >= len(l.src) {
				return Token{}, fmt.Errorf("%s: unterminated string literal", p)
			}

			esc := l.src[l.offset]
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
	for l.offset < len(l.src) {
		ch := l.src[l.offset]
		if ch == '\'' {
			l.advance()
			return Token{Kind: TokenCharLiteral, Pos: p, Value: string(buf)}, nil
		}

		if ch == '\\' {
			l.advance()
			if l.offset >= len(l.src) {
				return Token{}, fmt.Errorf("%s: unterminated char literal", p)
			}

			esc := l.src[l.offset]
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

	start := l.offset
	for l.offset < len(l.src) && isIdentChar(l.src[l.offset]) {
		l.advance()
	}

	name := string(l.src[start:l.offset])
	return Token{Kind: TokenAnnotation, Pos: p, Value: name}
}

// Next returns the next token from the source.
// Returns TokenEOF at end of input. Returns an error token with Value
// containing the error message on lexer errors.
func (l *Lexer) Next() Token {
	if err := l.skipWhitespaceAndComments(); err != nil {
		return Token{Kind: TokenError, Pos: l.pos(), Value: err.Error()}
	}

	if l.offset >= len(l.src) {
		return Token{Kind: TokenEOF, Pos: l.pos()}
	}

	ch := l.src[l.offset]

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
