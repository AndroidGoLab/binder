package parser

// FieldDecl represents a field in a parcelable or union.
type FieldDecl struct {
	Pos          Position
	Annots       []*Annotation
	Type         *TypeSpecifier
	FieldName    string
	DefaultValue ConstExpr
}
