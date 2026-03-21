package parser

// ConstExpr is implemented by all constant expression AST nodes.
type ConstExpr interface {
	constExprNode()
	ExprPos() Position
}
