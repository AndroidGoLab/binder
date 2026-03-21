package parser

// CharLiteralExpr represents a character constant.
type CharLiteralExpr struct {
	TokenPos Position
	Value    string
}

func (*CharLiteralExpr) constExprNode() {}

// ExprPos returns the position of this expression.
func (e *CharLiteralExpr) ExprPos() Position { return e.TokenPos }
