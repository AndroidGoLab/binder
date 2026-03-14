package parser

// TypeSpecifier represents a type reference in AIDL.
type TypeSpecifier struct {
	Pos      Position
	Annots   []*Annotation
	Name     string
	TypeArgs []*TypeSpecifier
	IsArray  bool
	// FixedSize holds the fixed-size array dimension expression (e.g., "6"
	// or "CONST_NAME") when the type uses fixed-size array syntax like
	// byte[6]. Empty string means dynamic array or non-array.
	FixedSize string
}
