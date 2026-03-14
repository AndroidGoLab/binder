package validate

// MapType represents Map<K,V>.
type MapType struct {
	KeyType   ResolvedType
	ValueType ResolvedType
}

func (t *MapType) resolvedTypeNode() {}

func (t *MapType) String() string {
	return "Map<" + t.KeyType.String() + ", " + t.ValueType.String() + ">"
}
