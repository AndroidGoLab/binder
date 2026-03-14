package parser

// ConstantDecl represents a constant declaration inside an interface, parcelable, or union.
type ConstantDecl struct {
	Pos       Position
	Type      *TypeSpecifier
	ConstName string
	Value     ConstExpr
}
