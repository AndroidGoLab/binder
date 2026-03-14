package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// parseAndEval is a helper that parses a constant expression and evaluates it.
func parseAndEval(
	t *testing.T,
	src string,
) any {
	t.Helper()

	// Wrap the expression in a constant declaration so the parser can handle it.
	full := "interface T { const int X = " + src + "; }"
	doc, err := Parse("test.aidl", []byte(full))
	require.NoError(t, err)

	iface, ok := doc.Definitions[0].(*InterfaceDecl)
	require.True(t, ok)
	require.Len(t, iface.Constants, 1)

	val, err := Evaluate(iface.Constants[0].Value)
	require.NoError(t, err)
	return val
}

func TestEvalArithmetic(t *testing.T) {
	// 1 + 2 * 3 = 7 (multiplicative binds tighter)
	val := parseAndEval(t, "1 + 2 * 3")
	assert.Equal(t, int64(7), val)
}

func TestEvalArithmeticParens(t *testing.T) {
	// (1 + 2) * 3 = 9
	val := parseAndEval(t, "(1 + 2) * 3")
	assert.Equal(t, int64(9), val)
}

func TestEvalSubtraction(t *testing.T) {
	val := parseAndEval(t, "10 - 3")
	assert.Equal(t, int64(7), val)
}

func TestEvalDivision(t *testing.T) {
	val := parseAndEval(t, "15 / 4")
	assert.Equal(t, int64(3), val)
}

func TestEvalModulo(t *testing.T) {
	val := parseAndEval(t, "17 % 5")
	assert.Equal(t, int64(2), val)
}

func TestEvalHex(t *testing.T) {
	val := parseAndEval(t, "0xFF")
	assert.Equal(t, int64(255), val)
}

func TestEvalOctal(t *testing.T) {
	val := parseAndEval(t, "077")
	assert.Equal(t, int64(63), val)
}

func TestEvalBinary(t *testing.T) {
	val := parseAndEval(t, "0b1010")
	assert.Equal(t, int64(10), val)
}

func TestEvalBitwiseShiftLeft(t *testing.T) {
	val := parseAndEval(t, "1 << 4")
	assert.Equal(t, int64(16), val)
}

func TestEvalBitwiseShiftRight(t *testing.T) {
	val := parseAndEval(t, "256 >> 4")
	assert.Equal(t, int64(16), val)
}

func TestEvalBitwiseAnd(t *testing.T) {
	val := parseAndEval(t, "0xFF & 0x0F")
	assert.Equal(t, int64(15), val)
}

func TestEvalBitwiseOr(t *testing.T) {
	val := parseAndEval(t, "0xF0 | 0x0F")
	assert.Equal(t, int64(255), val)
}

func TestEvalBitwiseXor(t *testing.T) {
	val := parseAndEval(t, "0xFF ^ 0x0F")
	assert.Equal(t, int64(240), val)
}

func TestEvalTernaryTrue(t *testing.T) {
	// true ? 1 : 2 = 1
	// We need bool context, so use 1 for true.
	val := parseAndEval(t, "1 ? 10 : 20")
	assert.Equal(t, int64(10), val)
}

func TestEvalTernaryFalse(t *testing.T) {
	val := parseAndEval(t, "0 ? 10 : 20")
	assert.Equal(t, int64(20), val)
}

func TestEvalUnaryMinus(t *testing.T) {
	val := parseAndEval(t, "-42")
	assert.Equal(t, int64(-42), val)
}

func TestEvalUnaryPlus(t *testing.T) {
	val := parseAndEval(t, "+42")
	assert.Equal(t, int64(42), val)
}

func TestEvalBitwiseNot(t *testing.T) {
	val := parseAndEval(t, "~0xFF")
	assert.Equal(t, ^int64(0xFF), val)
}

func TestEvalLogicalNot(t *testing.T) {
	// !1 should be false, but we wrap in int context.
	// Parse as a bool expression.
	src := `interface T { const int X = !0; }`
	doc, err := Parse("test.aidl", []byte(src))
	require.NoError(t, err)

	iface := doc.Definitions[0].(*InterfaceDecl)
	val, err := Evaluate(iface.Constants[0].Value)
	require.NoError(t, err)
	assert.Equal(t, true, val)
}

func TestEvalLogicalAnd(t *testing.T) {
	val := parseAndEval(t, "1 && 1")
	assert.Equal(t, true, val)

	val = parseAndEval(t, "1 && 0")
	assert.Equal(t, false, val)
}

func TestEvalLogicalOr(t *testing.T) {
	val := parseAndEval(t, "0 || 1")
	assert.Equal(t, true, val)

	val = parseAndEval(t, "0 || 0")
	assert.Equal(t, false, val)
}

func TestEvalEquality(t *testing.T) {
	val := parseAndEval(t, "42 == 42")
	assert.Equal(t, true, val)

	val = parseAndEval(t, "42 != 43")
	assert.Equal(t, true, val)

	val = parseAndEval(t, "42 == 43")
	assert.Equal(t, false, val)
}

func TestEvalRelational(t *testing.T) {
	val := parseAndEval(t, "1 < 2")
	assert.Equal(t, true, val)

	val = parseAndEval(t, "2 > 1")
	assert.Equal(t, true, val)

	val = parseAndEval(t, "2 <= 2")
	assert.Equal(t, true, val)

	val = parseAndEval(t, "3 >= 2")
	assert.Equal(t, true, val)
}

func TestEvalComplexExpression(t *testing.T) {
	// (1 << 4) | 0x0F = 16 | 15 = 31
	val := parseAndEval(t, "(1 << 4) | 0x0F")
	assert.Equal(t, int64(31), val)
}

func TestEvalStringLiteral(t *testing.T) {
	src := `interface T { const String X = "hello"; }`
	doc, err := Parse("test.aidl", []byte(src))
	require.NoError(t, err)

	iface := doc.Definitions[0].(*InterfaceDecl)
	val, err := Evaluate(iface.Constants[0].Value)
	require.NoError(t, err)
	assert.Equal(t, "hello", val)
}

func TestEvalBoolLiterals(t *testing.T) {
	src := `parcelable T { boolean x = true; boolean y = false; }`
	doc, err := Parse("test.aidl", []byte(src))
	require.NoError(t, err)

	parc := doc.Definitions[0].(*ParcelableDecl)

	val, err := Evaluate(parc.Fields[0].DefaultValue)
	require.NoError(t, err)
	assert.Equal(t, true, val)

	val, err = Evaluate(parc.Fields[1].DefaultValue)
	require.NoError(t, err)
	assert.Equal(t, false, val)
}

func TestEvalIdentError(t *testing.T) {
	src := `interface T { const int X = FOO; }`
	doc, err := Parse("test.aidl", []byte(src))
	require.NoError(t, err)

	iface := doc.Definitions[0].(*InterfaceDecl)
	_, err = Evaluate(iface.Constants[0].Value)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot evaluate identifier")
}

func TestEvalDivisionByZero(t *testing.T) {
	src := `interface T { const int X = 1 / 0; }`
	doc, err := Parse("test.aidl", []byte(src))
	require.NoError(t, err)

	iface := doc.Definitions[0].(*InterfaceDecl)
	_, err = Evaluate(iface.Constants[0].Value)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "division by zero")
}

func TestEvalNullLiteral(t *testing.T) {
	val, err := Evaluate(&NullLiteral{})
	require.NoError(t, err)
	assert.Nil(t, val)
}

func TestEvalIntSuffix(t *testing.T) {
	val, err := parseIntString("42L")
	require.NoError(t, err)
	assert.Equal(t, int64(42), val)

	val, err = parseIntString("0xFFl")
	require.NoError(t, err)
	assert.Equal(t, int64(255), val)
}

func TestEvalFloatLiteral(t *testing.T) {
	src := `parcelable T { float x = 3.14; }`
	doc, err := Parse("test.aidl", []byte(src))
	require.NoError(t, err)

	parc := doc.Definitions[0].(*ParcelableDecl)
	val, err := Evaluate(parc.Fields[0].DefaultValue)
	require.NoError(t, err)
	assert.InDelta(t, 3.14, val, 0.001)
}

func TestEvalCharLiteral(t *testing.T) {
	val, err := Evaluate(&CharLiteralExpr{Value: "A"})
	require.NoError(t, err)
	assert.Equal(t, int64('A'), val)
}

func TestEvalCharLiteralEmpty(t *testing.T) {
	val, err := Evaluate(&CharLiteralExpr{Value: ""})
	require.NoError(t, err)
	assert.Equal(t, int64(0), val)
}

func TestEvalUnaryMinusFloat(t *testing.T) {
	val, err := Evaluate(&UnaryExpr{
		Op:      TokenMinus,
		Operand: &FloatLiteral{Value: "1.5"},
	})
	require.NoError(t, err)
	assert.Equal(t, -1.5, val)
}

func TestEvalUnaryPlusFloat(t *testing.T) {
	val, err := Evaluate(&UnaryExpr{
		Op:      TokenPlus,
		Operand: &FloatLiteral{Value: "2.5"},
	})
	require.NoError(t, err)
	assert.Equal(t, 2.5, val)
}

func TestEvalUnaryMinusString(t *testing.T) {
	_, err := Evaluate(&UnaryExpr{
		Op:      TokenMinus,
		Operand: &StringLiteralExpr{Value: "x"},
	})
	assert.Error(t, err)
}

func TestEvalUnaryPlusString(t *testing.T) {
	_, err := Evaluate(&UnaryExpr{
		Op:      TokenPlus,
		Operand: &StringLiteralExpr{Value: "x"},
	})
	assert.Error(t, err)
}

func TestEvalUnaryTildeNonInt(t *testing.T) {
	_, err := Evaluate(&UnaryExpr{
		Op:      TokenTilde,
		Operand: &FloatLiteral{Value: "1.0"},
	})
	assert.Error(t, err)
}

func TestEvalUnaryBangBool(t *testing.T) {
	val, err := Evaluate(&UnaryExpr{
		Op:      TokenBang,
		Operand: &BoolLiteral{Value: true},
	})
	require.NoError(t, err)
	assert.Equal(t, false, val)
}

func TestEvalUnaryUnsupportedOp(t *testing.T) {
	_, err := Evaluate(&UnaryExpr{
		Op:      TokenStar,
		Operand: &IntegerLiteral{Value: "1"},
	})
	assert.Error(t, err)
}

func TestEvalBinaryFloatArithmetic(t *testing.T) {
	// int + float promotion
	val, err := Evaluate(&BinaryExpr{
		Op:    TokenPlus,
		Left:  &IntegerLiteral{Value: "1"},
		Right: &FloatLiteral{Value: "2.5"},
	})
	require.NoError(t, err)
	assert.InDelta(t, 3.5, val, 0.001)

	// float - int
	val, err = Evaluate(&BinaryExpr{
		Op:    TokenMinus,
		Left:  &FloatLiteral{Value: "5.0"},
		Right: &IntegerLiteral{Value: "2"},
	})
	require.NoError(t, err)
	assert.InDelta(t, 3.0, val, 0.001)

	// float * float
	val, err = Evaluate(&BinaryExpr{
		Op:    TokenStar,
		Left:  &FloatLiteral{Value: "2.0"},
		Right: &FloatLiteral{Value: "3.0"},
	})
	require.NoError(t, err)
	assert.InDelta(t, 6.0, val, 0.001)

	// float / float
	val, err = Evaluate(&BinaryExpr{
		Op:    TokenSlash,
		Left:  &FloatLiteral{Value: "7.0"},
		Right: &FloatLiteral{Value: "2.0"},
	})
	require.NoError(t, err)
	assert.InDelta(t, 3.5, val, 0.001)

	// float % float
	val, err = Evaluate(&BinaryExpr{
		Op:    TokenPercent,
		Left:  &FloatLiteral{Value: "7.5"},
		Right: &FloatLiteral{Value: "2.0"},
	})
	require.NoError(t, err)
	assert.InDelta(t, 1.5, val, 0.001)
}

func TestEvalBinaryFloatComparison(t *testing.T) {
	val, err := Evaluate(&BinaryExpr{
		Op:    TokenLAngle,
		Left:  &FloatLiteral{Value: "1.0"},
		Right: &FloatLiteral{Value: "2.0"},
	})
	require.NoError(t, err)
	assert.Equal(t, true, val)

	val, err = Evaluate(&BinaryExpr{
		Op:    TokenRAngle,
		Left:  &FloatLiteral{Value: "3.0"},
		Right: &FloatLiteral{Value: "2.0"},
	})
	require.NoError(t, err)
	assert.Equal(t, true, val)

	val, err = Evaluate(&BinaryExpr{
		Op:    TokenLessEq,
		Left:  &FloatLiteral{Value: "2.0"},
		Right: &FloatLiteral{Value: "2.0"},
	})
	require.NoError(t, err)
	assert.Equal(t, true, val)

	val, err = Evaluate(&BinaryExpr{
		Op:    TokenGreaterEq,
		Left:  &FloatLiteral{Value: "2.0"},
		Right: &FloatLiteral{Value: "2.0"},
	})
	require.NoError(t, err)
	assert.Equal(t, true, val)
}

func TestEvalBinaryFloatEquality(t *testing.T) {
	val, err := Evaluate(&BinaryExpr{
		Op:    TokenEqEq,
		Left:  &FloatLiteral{Value: "1.0"},
		Right: &FloatLiteral{Value: "1.0"},
	})
	require.NoError(t, err)
	assert.Equal(t, true, val)

	val, err = Evaluate(&BinaryExpr{
		Op:    TokenBangEq,
		Left:  &FloatLiteral{Value: "1.0"},
		Right: &FloatLiteral{Value: "2.0"},
	})
	require.NoError(t, err)
	assert.Equal(t, true, val)
}

func TestEvalBinaryStringEquality(t *testing.T) {
	val, err := Evaluate(&BinaryExpr{
		Op:    TokenEqEq,
		Left:  &StringLiteralExpr{Value: "abc"},
		Right: &StringLiteralExpr{Value: "abc"},
	})
	require.NoError(t, err)
	assert.Equal(t, true, val)

	val, err = Evaluate(&BinaryExpr{
		Op:    TokenBangEq,
		Left:  &StringLiteralExpr{Value: "abc"},
		Right: &StringLiteralExpr{Value: "def"},
	})
	require.NoError(t, err)
	assert.Equal(t, true, val)
}

func TestEvalBinaryStringConcat(t *testing.T) {
	val, err := Evaluate(&BinaryExpr{
		Op:    TokenPlus,
		Left:  &StringLiteralExpr{Value: "hello"},
		Right: &StringLiteralExpr{Value: " world"},
	})
	require.NoError(t, err)
	assert.Equal(t, "hello world", val)
}

func TestEvalBinaryUnsupportedTypes(t *testing.T) {
	_, err := Evaluate(&BinaryExpr{
		Op:    TokenAmp,
		Left:  &FloatLiteral{Value: "1.0"},
		Right: &FloatLiteral{Value: "2.0"},
	})
	assert.Error(t, err)
}

func TestEvalModuloByZero(t *testing.T) {
	_, err := Evaluate(&BinaryExpr{
		Op:    TokenPercent,
		Left:  &IntegerLiteral{Value: "5"},
		Right: &IntegerLiteral{Value: "0"},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "modulo by zero")
}

func TestEvalTernaryBoolCondition(t *testing.T) {
	val, err := Evaluate(&TernaryExpr{
		Cond: &BoolLiteral{Value: true},
		Then: &IntegerLiteral{Value: "10"},
		Else: &IntegerLiteral{Value: "20"},
	})
	require.NoError(t, err)
	assert.Equal(t, int64(10), val)

	val, err = Evaluate(&TernaryExpr{
		Cond: &BoolLiteral{Value: false},
		Then: &IntegerLiteral{Value: "10"},
		Else: &IntegerLiteral{Value: "20"},
	})
	require.NoError(t, err)
	assert.Equal(t, int64(20), val)
}

func TestEvalTernaryBadCondition(t *testing.T) {
	_, err := Evaluate(&TernaryExpr{
		Cond: &StringLiteralExpr{Value: "not bool"},
		Then: &IntegerLiteral{Value: "1"},
		Else: &IntegerLiteral{Value: "2"},
	})
	assert.Error(t, err)
}

func TestEvalLogicalAndBadOperand(t *testing.T) {
	_, err := Evaluate(&BinaryExpr{
		Op:    TokenAmpAmp,
		Left:  &StringLiteralExpr{Value: "x"},
		Right: &BoolLiteral{Value: true},
	})
	assert.Error(t, err)
}

func TestEvalLogicalOrBadOperand(t *testing.T) {
	_, err := Evaluate(&BinaryExpr{
		Op:    TokenPipePipe,
		Left:  &BoolLiteral{Value: false},
		Right: &StringLiteralExpr{Value: "x"},
	})
	assert.Error(t, err)
}

func TestEvalBangBadOperand(t *testing.T) {
	_, err := Evaluate(&UnaryExpr{
		Op:      TokenBang,
		Operand: &StringLiteralExpr{Value: "x"},
	})
	assert.Error(t, err)
}

func TestParseFloatString(t *testing.T) {
	val, err := parseFloatString("3.14")
	require.NoError(t, err)
	assert.InDelta(t, 3.14, val, 0.001)

	val, err = parseFloatString("1.0f")
	require.NoError(t, err)
	assert.InDelta(t, 1.0, val, 0.001)

	val, err = parseFloatString("2.0d")
	require.NoError(t, err)
	assert.InDelta(t, 2.0, val, 0.001)
}

func TestToBoolFloat(t *testing.T) {
	b, err := toBool(float64(1.0))
	require.NoError(t, err)
	assert.True(t, b)

	b, err = toBool(float64(0.0))
	require.NoError(t, err)
	assert.False(t, b)
}

func TestEvalBinaryIntEquality(t *testing.T) {
	val, err := Evaluate(&BinaryExpr{
		Op:    TokenEqEq,
		Left:  &IntegerLiteral{Value: "5"},
		Right: &IntegerLiteral{Value: "5"},
	})
	require.NoError(t, err)
	assert.Equal(t, true, val)
}

func TestEvalBinaryMixedIntFloatEquality(t *testing.T) {
	val, err := Evaluate(&BinaryExpr{
		Op:    TokenEqEq,
		Left:  &IntegerLiteral{Value: "5"},
		Right: &FloatLiteral{Value: "5.0"},
	})
	require.NoError(t, err)
	assert.Equal(t, true, val)
}

func TestParseIntStringErrors(t *testing.T) {
	_, err := parseIntString("")
	assert.Error(t, err)

	_, err = parseIntString("0xZZ")
	assert.Error(t, err)

	_, err = parseIntString("0b222")
	assert.Error(t, err)

	_, err = parseIntString("abc")
	assert.Error(t, err)
}

func TestParseFloatStringError(t *testing.T) {
	_, err := parseFloatString("notafloat")
	assert.Error(t, err)
}

func TestEvalTernaryCondError(t *testing.T) {
	_, err := Evaluate(&TernaryExpr{
		Cond: &IdentExpr{Name: "UNKNOWN"},
		Then: &IntegerLiteral{Value: "1"},
		Else: &IntegerLiteral{Value: "2"},
	})
	assert.Error(t, err)
}

func TestEvalTernaryThenError(t *testing.T) {
	_, err := Evaluate(&TernaryExpr{
		Cond: &BoolLiteral{Value: true},
		Then: &IdentExpr{Name: "UNKNOWN"},
		Else: &IntegerLiteral{Value: "2"},
	})
	assert.Error(t, err)
}

func TestEvalBinaryLeftError(t *testing.T) {
	_, err := Evaluate(&BinaryExpr{
		Op:    TokenPlus,
		Left:  &IdentExpr{Name: "UNKNOWN"},
		Right: &IntegerLiteral{Value: "1"},
	})
	assert.Error(t, err)
}

func TestEvalBinaryRightError(t *testing.T) {
	_, err := Evaluate(&BinaryExpr{
		Op:    TokenPlus,
		Left:  &IntegerLiteral{Value: "1"},
		Right: &IdentExpr{Name: "UNKNOWN"},
	})
	assert.Error(t, err)
}

func TestEvalUnaryError(t *testing.T) {
	_, err := Evaluate(&UnaryExpr{
		Op:      TokenMinus,
		Operand: &IdentExpr{Name: "UNKNOWN"},
	})
	assert.Error(t, err)
}

func TestEvalNestedTernary(t *testing.T) {
	// 1 ? (0 ? 10 : 20) : 30 => 20
	val := parseAndEval(t, "1 ? 0 ? 10 : 20 : 30")
	assert.Equal(t, int64(20), val)
}
