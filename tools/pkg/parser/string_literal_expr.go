package parser

// StringLiteralExpr represents a string constant (unquoted value).
type StringLiteralExpr struct {
	TokenPos Position
	Value    string
}

func (*StringLiteralExpr) constExprNode() {}

// ExprPos returns the position of this expression.
func (e *StringLiteralExpr) ExprPos() Position { return e.TokenPos }
