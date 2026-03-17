package spec

// MethodSpec describes a single AIDL interface method.
type MethodSpec struct {
	Name string `yaml:"name"`

	// TransactionCodeOffset is the offset from binder.FirstCallTransaction.
	// The actual code is FirstCallTransaction + TransactionCodeOffset.
	TransactionCodeOffset int `yaml:"transaction_code_offset"`

	Oneway      bool        `yaml:"oneway,omitempty"`
	Params      []ParamSpec `yaml:"params,omitempty"`
	ReturnType  TypeRef     `yaml:"return_type,omitempty"`
	Annotations []string    `yaml:"annotations,omitempty"`
}
