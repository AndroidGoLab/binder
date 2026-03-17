package spec

// EnumSpec describes an AIDL enum.
type EnumSpec struct {
	Name        string           `yaml:"name"`
	BackingType string           `yaml:"backing_type"`
	Values      []EnumeratorSpec `yaml:"values"`
	Annotations []string         `yaml:"annotations,omitempty"`
}
