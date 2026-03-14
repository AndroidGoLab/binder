package validate

// ListType represents List<T>.
type ListType struct {
	ElementType ResolvedType
}

func (t *ListType) resolvedTypeNode() {}

func (t *ListType) String() string {
	return "List<" + t.ElementType.String() + ">"
}
