package parser

// EnumDecl represents an AIDL enum declaration.
type EnumDecl struct {
	Pos         Position
	Annots      []*Annotation
	EnumName    string
	BackingType *TypeSpecifier
	Enumerators []*Enumerator
}

func (*EnumDecl) definitionNode() {}

// GetName returns the enum name.
func (d *EnumDecl) GetName() string {
	return d.EnumName
}

// GetAnnotations returns the annotations on this enum.
func (d *EnumDecl) GetAnnotations() []*Annotation {
	return d.Annots
}
