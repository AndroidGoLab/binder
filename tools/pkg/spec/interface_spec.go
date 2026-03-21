package spec

// InterfaceSpec describes an AIDL interface with its methods,
// constants, and multi-version transaction code mappings.
type InterfaceSpec struct {
	Name       string `yaml:"name"`
	Descriptor string `yaml:"descriptor"`
	Oneway     bool   `yaml:"oneway,omitempty"`

	Methods     []MethodSpec   `yaml:"methods,omitempty"`
	Constants   []ConstantSpec `yaml:"constants,omitempty"`
	NestedTypes []string       `yaml:"nested_types,omitempty"`
	Annotations []string       `yaml:"annotations,omitempty"`

	// VersionCodes maps revision ID (e.g., "36.r4") to a map of
	// method name → transaction code offset (relative to FirstCallTransaction).
	// Used by the version-aware transport for multi-API-level support.
	VersionCodes map[string]map[string]int `yaml:"version_codes,omitempty"`
}
