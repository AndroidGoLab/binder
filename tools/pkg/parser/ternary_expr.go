package parser

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
