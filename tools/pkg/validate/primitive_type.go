package validate

// PrimitiveType represents a built-in primitive (int, long, float, double, boolean, byte, char).
type PrimitiveType struct {
	Name string
}

func (t *PrimitiveType) resolvedTypeNode() {}

func (t *PrimitiveType) String() string {
	return t.Name
}
