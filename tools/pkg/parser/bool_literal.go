package parser

// BoolLiteral represents a boolean constant.
type BoolLiteral struct {
	TokenPos Position
	Value    bool
}

func (*BoolLiteral) constExprNode() {}

// ExprPos returns the position of this expression.
func (e *BoolLiteral) ExprPos() Position { return e.TokenPos }
