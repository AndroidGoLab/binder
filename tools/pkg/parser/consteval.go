package parser

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Evaluate evaluates a constant expression to a concrete value.
// Returns int64 for integer expressions, float64 for float, string for string, bool for bool.
func Evaluate(
	expr ConstExpr,
) (any, error) {
	switch e := expr.(type) {
	case *IntegerLiteral:
		return parseIntString(e.Value)

	case *FloatLiteral:
		return parseFloatString(e.Value)

	case *StringLiteralExpr:
		return e.Value, nil

	case *CharLiteralExpr:
		if len(e.Value) == 0 {
			return int64(0), nil
		}
		// The lexer already resolves escape sequences, so e.Value
		// contains the actual character byte. Use it directly.
		return int64(e.Value[0]), nil

	case *BoolLiteral:
		return e.Value, nil

	case *NullLiteral:
		return nil, nil

	case *UnaryExpr:
		return evaluateUnary(e)

	case *BinaryExpr:
		return evaluateBinaryLazy(e)

	case *TernaryExpr:
		return evaluateTernary(e)

	case *IdentExpr:
		return nil, fmt.Errorf(
			"%s: cannot evaluate identifier %q without symbol table",
			e.TokenPos, e.Name,
		)

	default:
		return nil, fmt.Errorf("unsupported expression type %T", expr)
	}
}

func evaluateUnary(
	e *UnaryExpr,
) (any, error) {
	val, err := Evaluate(e.Operand)
	if err != nil {
		return nil, err
	}

	switch e.Op {
	case TokenMinus:
		switch v := val.(type) {
		case int64:
			return -v, nil
		case float64:
			return -v, nil
		default:
			return nil, fmt.Errorf("%s: unary minus on non-numeric type %T", e.TokenPos, val)
		}

	case TokenPlus:
		switch v := val.(type) {
		case int64:
			return v, nil
		case float64:
			return v, nil
		default:
			return nil, fmt.Errorf("%s: unary plus on non-numeric type %T", e.TokenPos, val)
		}

	case TokenTilde:
		v, ok := val.(int64)
		if !ok {
			return nil, fmt.Errorf("%s: bitwise NOT on non-integer type %T", e.TokenPos, val)
		}
		return ^v, nil

	case TokenBang:
		b, err := toBool(val)
		if err != nil {
			return nil, fmt.Errorf("%s: logical NOT: %w", e.TokenPos, err)
		}
		return !b, nil

	default:
		return nil, fmt.Errorf("%s: unsupported unary operator %s", e.TokenPos, e.Op)
	}
}

// evaluateBinaryLazy handles short-circuit evaluation for && and ||,
// then delegates to evaluateBinary for all other operators.
func evaluateBinaryLazy(
	e *BinaryExpr,
) (any, error) {
	switch e.Op {
	case TokenAmpAmp:
		left, err := Evaluate(e.Left)
		if err != nil {
			return nil, err
		}
		lb, err := toBool(left)
		if err != nil {
			return nil, fmt.Errorf("%s: logical AND: %w", e.TokenPos, err)
		}
		if !lb {
			return false, nil
		}
		right, err := Evaluate(e.Right)
		if err != nil {
			return nil, err
		}
		rb, err := toBool(right)
		if err != nil {
			return nil, fmt.Errorf("%s: logical AND: %w", e.TokenPos, err)
		}
		return rb, nil

	case TokenPipePipe:
		left, err := Evaluate(e.Left)
		if err != nil {
			return nil, err
		}
		lb, err := toBool(left)
		if err != nil {
			return nil, fmt.Errorf("%s: logical OR: %w", e.TokenPos, err)
		}
		if lb {
			return true, nil
		}
		right, err := Evaluate(e.Right)
		if err != nil {
			return nil, err
		}
		rb, err := toBool(right)
		if err != nil {
			return nil, fmt.Errorf("%s: logical OR: %w", e.TokenPos, err)
		}
		return rb, nil

	default:
		return evaluateBinary(e)
	}
}

func evaluateBinary(
	e *BinaryExpr,
) (any, error) {
	left, err := Evaluate(e.Left)
	if err != nil {
		return nil, err
	}

	right, err := Evaluate(e.Right)
	if err != nil {
		return nil, err
	}

	// Promote to float if either is float.
	leftInt, leftIsInt := left.(int64)
	rightInt, rightIsInt := right.(int64)
	leftFloat, leftIsFloat := left.(float64)
	rightFloat, rightIsFloat := right.(float64)

	bothInt := leftIsInt && rightIsInt
	eitherFloat := leftIsFloat || rightIsFloat

	if leftIsInt && rightIsFloat {
		leftFloat = float64(leftInt)
		eitherFloat = true
	}
	if leftIsFloat && rightIsInt {
		rightFloat = float64(rightInt)
		eitherFloat = true
	}
	if leftIsFloat && rightIsFloat {
		eitherFloat = true
	}

	switch e.Op {
	case TokenPlus:
		if bothInt {
			return leftInt + rightInt, nil
		}
		if eitherFloat {
			return leftFloat + rightFloat, nil
		}
		// String concatenation.
		ls, lok := left.(string)
		rs, rok := right.(string)
		if lok && rok {
			return ls + rs, nil
		}

	case TokenMinus:
		if bothInt {
			return leftInt - rightInt, nil
		}
		if eitherFloat {
			return leftFloat - rightFloat, nil
		}

	case TokenStar:
		if bothInt {
			return leftInt * rightInt, nil
		}
		if eitherFloat {
			return leftFloat * rightFloat, nil
		}

	case TokenSlash:
		if bothInt {
			if rightInt == 0 {
				return nil, fmt.Errorf("%s: division by zero", e.TokenPos)
			}
			return leftInt / rightInt, nil
		}
		if eitherFloat {
			return leftFloat / rightFloat, nil
		}

	case TokenPercent:
		if bothInt {
			if rightInt == 0 {
				return nil, fmt.Errorf("%s: modulo by zero", e.TokenPos)
			}
			return leftInt % rightInt, nil
		}
		if eitherFloat {
			return math.Mod(leftFloat, rightFloat), nil
		}

	case TokenAmp:
		if bothInt {
			return leftInt & rightInt, nil
		}

	case TokenPipe:
		if bothInt {
			return leftInt | rightInt, nil
		}

	case TokenCaret:
		if bothInt {
			return leftInt ^ rightInt, nil
		}

	case TokenLShift:
		if bothInt {
			if rightInt < 0 || rightInt > 63 {
				return int64(0), nil
			}
			return leftInt << uint(rightInt), nil
		}

	case TokenRShift:
		if bothInt {
			if rightInt < 0 || rightInt > 63 {
				return int64(0), nil
			}
			return leftInt >> uint(rightInt), nil
		}

	case TokenEqEq:
		return evalEquality(left, right, leftFloat, rightFloat, bothInt, eitherFloat, leftInt, rightInt), nil

	case TokenBangEq:
		return !evalEquality(left, right, leftFloat, rightFloat, bothInt, eitherFloat, leftInt, rightInt), nil

	case TokenLAngle:
		if bothInt {
			return leftInt < rightInt, nil
		}
		if eitherFloat {
			return leftFloat < rightFloat, nil
		}

	case TokenRAngle:
		if bothInt {
			return leftInt > rightInt, nil
		}
		if eitherFloat {
			return leftFloat > rightFloat, nil
		}

	case TokenLessEq:
		if bothInt {
			return leftInt <= rightInt, nil
		}
		if eitherFloat {
			return leftFloat <= rightFloat, nil
		}

	case TokenGreaterEq:
		if bothInt {
			return leftInt >= rightInt, nil
		}
		if eitherFloat {
			return leftFloat >= rightFloat, nil
		}
	}

	return nil, fmt.Errorf(
		"%s: unsupported binary operation %s on types %T and %T",
		e.TokenPos, e.Op, left, right,
	)
}

func evalEquality(
	left any,
	right any,
	leftFloat float64,
	rightFloat float64,
	bothInt bool,
	eitherFloat bool,
	leftInt int64,
	rightInt int64,
) bool {
	if bothInt {
		return leftInt == rightInt
	}
	if eitherFloat {
		return leftFloat == rightFloat
	}
	return left == right
}

func evaluateTernary(
	e *TernaryExpr,
) (any, error) {
	cond, err := Evaluate(e.Cond)
	if err != nil {
		return nil, err
	}

	b, err := toBool(cond)
	if err != nil {
		return nil, fmt.Errorf("%s: ternary condition: %w", e.TokenPos, err)
	}

	if b {
		return Evaluate(e.Then)
	}
	return Evaluate(e.Else)
}

func toBool(
	v any,
) (bool, error) {
	switch val := v.(type) {
	case bool:
		return val, nil
	case int64:
		return val != 0, nil
	case float64:
		return val != 0, nil
	default:
		return false, fmt.Errorf("cannot convert %T to bool", v)
	}
}

// aidlIntSuffixes lists AIDL typed integer suffixes to strip during parsing.
// These include unsigned (u8, u16, u32, u64) and signed (i8, i16, i32, i64)
// suffixes. Longer suffixes are listed first so they match before shorter
// ones.
var aidlIntSuffixes = []string{
	"u64", "u32", "u16", "u8",
	"i64", "i32", "i16", "i8",
}

// parseIntString parses an integer literal string (decimal, hex, octal, binary)
// with optional L/l or AIDL typed integer suffixes (u8, u32, i64, etc.).
func parseIntString(
	s string,
) (int64, error) {
	// Strip AIDL typed integer suffixes (e.g. 42u8, 0xFFi32).
	for _, suffix := range aidlIntSuffixes {
		if strings.HasSuffix(s, suffix) {
			s = s[:len(s)-len(suffix)]
			break
		}
	}
	// Strip long suffix (at most one 'L' or 'l').
	s = strings.TrimSuffix(strings.TrimSuffix(s, "L"), "l")
	if s == "" {
		return 0, fmt.Errorf("empty integer literal")
	}

	// Hex.
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		v, err := strconv.ParseInt(s[2:], 16, 64)
		if err != nil {
			// Try unsigned parse for large hex values.
			uv, uerr := strconv.ParseUint(s[2:], 16, 64)
			if uerr != nil {
				return 0, fmt.Errorf("invalid hex literal %q: %w", s, err)
			}
			return int64(uv), nil
		}
		return v, nil
	}

	// Binary.
	if strings.HasPrefix(s, "0b") || strings.HasPrefix(s, "0B") {
		v, err := strconv.ParseInt(s[2:], 2, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid binary literal %q: %w", s, err)
		}
		return v, nil
	}

	// Octal: starts with 0 and has more digits.
	if len(s) > 1 && s[0] == '0' && s[1] >= '0' && s[1] <= '7' {
		v, err := strconv.ParseInt(s[1:], 8, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid octal literal %q: %w", s, err)
		}
		return v, nil
	}

	// Decimal.
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid integer literal %q: %w", s, err)
	}
	return v, nil
}

func parseFloatString(
	s string,
) (float64, error) {
	// Strip float/double suffix.
	s = strings.TrimRight(s, "fFdD")

	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid float literal %q: %w", s, err)
	}
	return v, nil
}
