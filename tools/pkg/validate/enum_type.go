package validate

// EnumType represents a reference to an AIDL enum.
type EnumType struct {
	QualifiedName string
}

func (t *EnumType) resolvedTypeNode() {}

func (t *EnumType) String() string {
	return t.QualifiedName
}
