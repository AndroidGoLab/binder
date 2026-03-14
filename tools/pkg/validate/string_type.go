package validate

// StringType represents the AIDL String type (UTF-16 or UTF-8).
type StringType struct {
	UTF8 bool // true if @utf8InCpp annotated
}

func (t *StringType) resolvedTypeNode() {}

func (t *StringType) String() string {
	if t.UTF8 {
		return "String(utf8)"
	}
	return "String"
}
