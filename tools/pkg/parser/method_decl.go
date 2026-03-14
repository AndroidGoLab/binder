package parser

// MethodDecl represents a method declaration inside an interface.
type MethodDecl struct {
	Pos           Position
	Annots        []*Annotation
	Oneway        bool
	ReturnType    *TypeSpecifier
	MethodName    string
	Params        []*ParamDecl
	TransactionID int
}
