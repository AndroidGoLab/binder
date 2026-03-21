package spec

// TypeRef describes a type reference in the spec. It captures both
// simple types (int32, bool, String16) and complex types (arrays,
// generics, nullable).
type TypeRef struct {
	// Name is the type name. For primitives: "int32", "int64", "bool",
	// "float32", "float64", "String16", "String8", "byte", "char".
	// For AIDL types: fully qualified name (e.g., "android.app.ProcessMemoryState").
	// For arrays: the element type name (with IsArray=true).
	// For generics: the base type name (with TypeArgs set).
	Name string `yaml:"name"`

	// IsArray indicates T[] (variable-length array).
	IsArray bool `yaml:"is_array,omitempty"`

	// FixedSize is the array dimension for fixed-size arrays (e.g., "128"
	// for byte[128]). Empty for variable-length arrays.
	FixedSize string `yaml:"fixed_size,omitempty"`

	// IsNullable indicates the @nullable annotation.
	IsNullable bool `yaml:"is_nullable,omitempty"`

	// Annotations holds type-level annotation names beyond @nullable
	// (e.g., "utf8InCpp"). The @nullable annotation is NOT included
	// here — it is represented by IsNullable.
	Annotations []string `yaml:"annotations,omitempty"`

	// TypeArgs holds generic type arguments (e.g., List<T> → TypeArgs=[T]).
	TypeArgs []TypeRef `yaml:"type_args,omitempty"`
}
