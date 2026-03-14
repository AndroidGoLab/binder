package parser

// UnionDecl represents an AIDL union declaration.
type UnionDecl struct {
	Pos       Position
	Annots    []*Annotation
	UnionName string
	Fields    []*FieldDecl
	Constants []*ConstantDecl
	// Nested type definitions inside this union.
	NestedTypes []Definition
}

func (*UnionDecl) definitionNode() {}

// GetName returns the union name.
func (d *UnionDecl) GetName() string {
	return d.UnionName
}

// GetAnnotations returns the annotations on this union.
func (d *UnionDecl) GetAnnotations() []*Annotation {
	return d.Annots
}
