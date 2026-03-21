package parser

// NullLiteral represents the null constant.
type NullLiteral struct {
	TokenPos Position
}

func (*NullLiteral) constExprNode() {}

// ExprPos returns the position of this expression.
func (e *NullLiteral) ExprPos() Position { return e.TokenPos }
