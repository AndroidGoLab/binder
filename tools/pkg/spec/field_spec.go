package spec

// FieldSpec describes a parcelable or union field.
type FieldSpec struct {
	Name         string   `yaml:"name"`
	Type         TypeRef  `yaml:"type"`
	DefaultValue string   `yaml:"default_value,omitempty"`
	Annotations  []string `yaml:"annotations,omitempty"`
}
