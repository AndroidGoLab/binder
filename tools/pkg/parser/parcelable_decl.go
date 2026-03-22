package parser

// JavaWireField describes one field's serialization in the Java
// writeToParcel() method. When present on a ParcelableDecl, the codegen
// uses this to produce marshal/unmarshal code that matches the Java wire
// format (including conditional fields).
type JavaWireField struct {
	// Name is the PascalCase field name (matches the struct field name).
	Name string
	// WriteMethod is the spec type: bool, int32, int64, float32, float64,
	// string8, string16, or opaque.
	WriteMethod string
	// Condition, if non-empty, is a bitmask expression like "FieldsMask & 256"
	// meaning the field is only serialized when that bit is set.
	Condition string
}

// ParcelableDecl represents an AIDL parcelable declaration.
type ParcelableDecl struct {
	Pos       Position
	Annots    []*Annotation
	ParcName  string
	Fields    []*FieldDecl
	Constants []*ConstantDecl
	// Nested type definitions inside this parcelable.
	NestedTypes []Definition
	// CppHeader is set for forward-declared parcelables (cpp_header "...").
	CppHeader string
	// NdkHeader is set for forward-declared parcelables (ndk_header "...").
	NdkHeader string
	// RustType is set for forward-declared parcelables (rust_type "...").
	RustType string
	// JavaWireFormat, when non-nil, overrides the standard AIDL-field-based
	// marshal/unmarshal with code matching the Java writeToParcel() layout.
	// Fields are still populated for struct generation, but marshal/unmarshal
	// uses this instead of the generic field-walking approach.
	JavaWireFormat []JavaWireField
}

func (*ParcelableDecl) definitionNode() {}

// GetName returns the parcelable name.
func (d *ParcelableDecl) GetName() string {
	return d.ParcName
}

// GetAnnotations returns the annotations on this parcelable.
func (d *ParcelableDecl) GetAnnotations() []*Annotation {
	return d.Annots
}
