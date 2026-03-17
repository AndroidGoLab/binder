package spec

// ParamSpec describes a method parameter.
type ParamSpec struct {
	Name        string    `yaml:"name"`
	Type        TypeRef   `yaml:"type"`
	Direction   Direction `yaml:"direction,omitempty"`
	Annotations []string  `yaml:"annotations,omitempty"`
}
