package validate

// ArrayType represents T[].
type ArrayType struct {
	ElementType ResolvedType
}

func (t *ArrayType) resolvedTypeNode() {}

func (t *ArrayType) String() string {
	return t.ElementType.String() + "[]"
}
