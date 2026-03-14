package parser

// ConstExpr is implemented by all constant expression AST nodes.
type ConstExpr interface {
	constExprNode()
	ExprPos() Position
}

// IntegerLiteral represents an integer constant (decimal, hex, octal, binary).
type IntegerLiteral struct {
	TokenPos Position
	Value    string
}

func (*IntegerLiteral) constExprNode() {}

// ExprPos returns the position of this expression.
func (e *IntegerLiteral) ExprPos() Position { return e.TokenPos }

// FloatLiteral represents a floating-point constant.
type FloatLiteral struct {
	TokenPos Position
	Value    string
}

func (*FloatLiteral) constExprNode() {}

// ExprPos returns the position of this expression.
func (e *FloatLiteral) ExprPos() Position { return e.TokenPos }

// StringLiteralExpr represents a string constant (unquoted value).
type StringLiteralExpr struct {
	TokenPos Position
	Value    string
}

func (*StringLiteralExpr) constExprNode() {}

// ExprPos returns the position of this expression.
func (e *StringLiteralExpr) ExprPos() Position { return e.TokenPos }

// CharLiteralExpr represents a character constant.
type CharLiteralExpr struct {
	TokenPos Position
	Value    string
}

func (*CharLiteralExpr) constExprNode() {}

// ExprPos returns the position of this expression.
func (e *CharLiteralExpr) ExprPos() Position { return e.TokenPos }

// BoolLiteral represents a boolean constant.
type BoolLiteral struct {
	TokenPos Position
	Value    bool
}

func (*BoolLiteral) constExprNode() {}

// ExprPos returns the position of this expression.
func (e *BoolLiteral) ExprPos() Position { return e.TokenPos }

// NullLiteral represents the null constant.
type NullLiteral struct {
	TokenPos Position
}

func (*NullLiteral) constExprNode() {}

// ExprPos returns the position of this expression.
func (e *NullLiteral) ExprPos() Position { return e.TokenPos }

// IdentExpr represents a reference to a constant or enum value.
type IdentExpr struct {
	TokenPos Position
	Name     string
}

func (*IdentExpr) constExprNode() {}

// ExprPos returns the position of this expression.
func (e *IdentExpr) ExprPos() Position { return e.TokenPos }

// UnaryExpr represents a unary operator expression.
type UnaryExpr struct {
	TokenPos Position
	Op       TokenKind
	Operand  ConstExpr
}

func (*UnaryExpr) constExprNode() {}

// ExprPos returns the position of this expression.
func (e *UnaryExpr) ExprPos() Position { return e.TokenPos }

// BinaryExpr represents a binary operator expression.
type BinaryExpr struct {
	TokenPos Position
	Op       TokenKind
	Left     ConstExpr
	Right    ConstExpr
}

func (*BinaryExpr) constExprNode() {}

// ExprPos returns the position of this expression.
func (e *BinaryExpr) ExprPos() Position { return e.TokenPos }

// TernaryExpr represents a ternary (conditional) expression.
type TernaryExpr struct {
	TokenPos Position
	Cond     ConstExpr
	Then     ConstExpr
	Else     ConstExpr
}

func (*TernaryExpr) constExprNode() {}

// ExprPos returns the position of this expression.
func (e *TernaryExpr) ExprPos() Position { return e.TokenPos }
