package parser

// FloatLiteral represents a floating-point constant.
type FloatLiteral struct {
	TokenPos Position
	Value    string
}

func (*FloatLiteral) constExprNode() {}

// ExprPos returns the position of this expression.
func (e *FloatLiteral) ExprPos() Position { return e.TokenPos }
