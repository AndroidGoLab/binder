package parser

// IdentExpr represents a reference to a constant or enum value.
type IdentExpr struct {
	TokenPos Position
	Name     string
}

func (*IdentExpr) constExprNode() {}

// ExprPos returns the position of this expression.
func (e *IdentExpr) ExprPos() Position { return e.TokenPos }
