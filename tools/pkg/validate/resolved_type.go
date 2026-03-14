package validate

// ResolvedType represents a fully resolved AIDL type.
type ResolvedType interface {
	resolvedTypeNode()
	String() string
}
