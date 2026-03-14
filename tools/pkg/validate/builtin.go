package validate

// BuiltinTypes maps AIDL built-in type names to their ResolvedType.
var BuiltinTypes = map[string]ResolvedType{
	"void":                 &VoidType{},
	"boolean":              &PrimitiveType{Name: "boolean"},
	"byte":                 &PrimitiveType{Name: "byte"},
	"char":                 &PrimitiveType{Name: "char"},
	"int":                  &PrimitiveType{Name: "int"},
	"long":                 &PrimitiveType{Name: "long"},
	"float":                &PrimitiveType{Name: "float"},
	"double":               &PrimitiveType{Name: "double"},
	"String":               &StringType{},
	"IBinder":              &BinderType{},
	"ParcelFileDescriptor": &FileDescriptorType{},
}

// IsBuiltin returns true if the type name is a built-in AIDL type.
func IsBuiltin(name string) bool {
	_, ok := BuiltinTypes[name]
	return ok
}
