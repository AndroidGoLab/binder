package parser

// JavaWireField describes one field's serialization in the Java
// writeToParcel() method. When present on a ParcelableDecl, the codegen
// uses this to produce marshal/unmarshal code that matches the Java wire
// format (including conditional fields).
type JavaWireField struct {
	// Name is the PascalCase field name (matches the struct field name).
	Name string
	// WriteMethod is the spec type: bool, int32, int64, float32, float64,
	// string8, string16, typed_object, repeated, or opaque.
	WriteMethod string
	// Condition, if non-empty, is a bitmask expression like "FieldsMask & 256"
	// meaning the field is only serialized when that bit is set.
	Condition string
	// GoType, if non-empty, is the qualified Go type for a typed_object field
	// whose parcelable was found in the spec registry (e.g., "os.WorkSource").
	// The codegen uses this to generate a *GoType struct field with proper
	// nullable marshal/unmarshal instead of an opaque null marker.
	GoType string
	// Elements, if non-empty, describes the sub-fields of a repeated
	// (array-of-structs) wire format group. Each element describes one
	// field of the repeated item.
	Elements []JavaWireField
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

	// NativeParcelable marks a parcelable whose wire format is defined
	// by native C++/JNI code. The codegen skips these; hand-written
	// implementations are provided via native_impls/.
	NativeParcelable bool
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
