package validate

// NullableType wraps a type to indicate @nullable.
type NullableType struct {
	Inner ResolvedType
}

func (t *NullableType) resolvedTypeNode() {}

func (t *NullableType) String() string {
	return "@nullable " + t.Inner.String()
}
