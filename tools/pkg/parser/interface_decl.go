package parser

// InterfaceDecl represents an AIDL interface declaration.
type InterfaceDecl struct {
	Pos       Position
	Annots    []*Annotation
	IntfName  string
	Oneway    bool
	Methods   []*MethodDecl
	Constants []*ConstantDecl
	// Nested type definitions inside this interface.
	NestedTypes []Definition
}

func (*InterfaceDecl) definitionNode() {}

// GetName returns the interface name.
func (d *InterfaceDecl) GetName() string {
	return d.IntfName
}

// GetAnnotations returns the annotations on this interface.
func (d *InterfaceDecl) GetAnnotations() []*Annotation {
	return d.Annots
}
