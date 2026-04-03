package spec

// JavaWireField describes one field's serialization in the Java
// writeToParcel() method. Used to generate Go marshal/unmarshal
// code that matches the Java wire format.
type JavaWireField struct {
	Name         string          `yaml:"name"`
	WriteMethod  string          `yaml:"write_method"`
	Condition    string          `yaml:"condition,omitempty"`
	DelegateType string          `yaml:"delegate_type,omitempty"`
	Elements     []JavaWireField `yaml:"elements,omitempty"`
}
