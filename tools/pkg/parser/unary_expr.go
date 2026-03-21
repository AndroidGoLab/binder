package parser

// UnaryExpr represents a unary operator expression.
type UnaryExpr struct {
	TokenPos Position
	Op       TokenKind
	Operand  ConstExpr
}

func (*UnaryExpr) constExprNode() {}

// ExprPos returns the position of this expression.
func (e *UnaryExpr) ExprPos() Position { return e.TokenPos }
