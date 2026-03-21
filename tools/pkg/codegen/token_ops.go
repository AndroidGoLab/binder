package codegen

import (
	"github.com/xaionaro-go/binder/tools/pkg/parser"
)

// tokenToGoOp maps a parser TokenKind operator to its Go operator string.
func tokenToGoOp(op parser.TokenKind) string {
	switch op {
	case parser.TokenPlus:
		return "+"
	case parser.TokenMinus:
		return "-"
	case parser.TokenStar:
		return "*"
	case parser.TokenSlash:
		return "/"
	case parser.TokenPercent:
		return "%"
	case parser.TokenAmp:
		return "&"
	case parser.TokenPipe:
		return "|"
	case parser.TokenCaret:
		return "^"
	case parser.TokenTilde:
		return "^" // Go uses ^ for bitwise NOT
	case parser.TokenBang:
		return "!"
	case parser.TokenLShift:
		return "<<"
	case parser.TokenRShift:
		return ">>"
	case parser.TokenAmpAmp:
		return "&&"
	case parser.TokenPipePipe:
		return "||"
	case parser.TokenEqEq:
		return "=="
	case parser.TokenBangEq:
		return "!="
	case parser.TokenLAngle:
		return "<"
	case parser.TokenRAngle:
		return ">"
	case parser.TokenLessEq:
		return "<="
	case parser.TokenGreaterEq:
		return ">="
	default:
		return "/* unknown_op */"
	}
}
