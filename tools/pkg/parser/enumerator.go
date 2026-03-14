package parser

// Enumerator represents a single enumerator within an enum declaration.
type Enumerator struct {
	Pos   Position
	Name  string
	Value ConstExpr
}
