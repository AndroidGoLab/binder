package spec

// UnionSpec describes an AIDL union (tagged variant type).
type UnionSpec struct {
	Name        string         `yaml:"name"`
	Fields      []FieldSpec    `yaml:"fields"`
	Constants   []ConstantSpec `yaml:"constants,omitempty"`
	NestedTypes []string       `yaml:"nested_types,omitempty"`
	Annotations []string       `yaml:"annotations,omitempty"`
}
