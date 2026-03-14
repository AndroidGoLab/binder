package parser

// Annotation represents an AIDL annotation like @nullable or @Backing(type="int").
type Annotation struct {
	Pos    Position
	Name   string
	Params map[string]ConstExpr
}
