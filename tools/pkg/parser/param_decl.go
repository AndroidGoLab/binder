package parser

// ParamDecl represents a method parameter.
type ParamDecl struct {
	Pos       Position
	Annots    []*Annotation
	Direction Direction
	Type      *TypeSpecifier
	ParamName string
}
