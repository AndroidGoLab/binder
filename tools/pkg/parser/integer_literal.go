package parser

// IntegerLiteral represents an integer constant (decimal, hex, octal, binary).
type IntegerLiteral struct {
	TokenPos Position
	Value    string
}

func (*IntegerLiteral) constExprNode() {}

// ExprPos returns the position of this expression.
func (e *IntegerLiteral) ExprPos() Position { return e.TokenPos }
