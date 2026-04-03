package parcelspec

// FieldSpec describes a single field in a Parcelable wire format.
type FieldSpec struct {
	Name         string      `yaml:"name"`
	Type         string      `yaml:"type"`                    // bool, int32, int64, float32, float64, string8, string16, repeated, opaque
	Condition    string      `yaml:"condition,omitempty"`      // e.g. "FieldsMask & 1"
	DelegateType string      `yaml:"delegate_type,omitempty"` // helper class for static delegates (e.g. "TextUtils")
	Elements     []FieldSpec `yaml:"elements,omitempty"`      // sub-fields for repeated (loop) fields
}
