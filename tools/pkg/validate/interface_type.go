package validate

// InterfaceType represents a reference to an AIDL interface.
type InterfaceType struct {
	QualifiedName string
}

func (t *InterfaceType) resolvedTypeNode() {}

func (t *InterfaceType) String() string {
	return t.QualifiedName
}
