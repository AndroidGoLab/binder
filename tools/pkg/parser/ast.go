package parser

// Document is the root AST node for an AIDL file.
type Document struct {
	Package     *PackageDecl
	Imports     []*ImportDecl
	Definitions []Definition
}

// Definition is implemented by all top-level declarations.
type Definition interface {
	definitionNode()
	GetName() string
	GetAnnotations() []*Annotation
}
