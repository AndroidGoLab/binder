package parser

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
