package validate

// UnionType represents a reference to an AIDL union.
type UnionType struct {
	QualifiedName string
}

func (t *UnionType) resolvedTypeNode() {}

func (t *UnionType) String() string {
	return t.QualifiedName
}
