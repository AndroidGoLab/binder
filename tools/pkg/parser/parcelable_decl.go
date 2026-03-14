package parser

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
