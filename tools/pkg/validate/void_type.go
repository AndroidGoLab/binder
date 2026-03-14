package validate

// VoidType represents the void return type.
type VoidType struct{}

func (t *VoidType) resolvedTypeNode() {}

func (t *VoidType) String() string {
	return "void"
}
