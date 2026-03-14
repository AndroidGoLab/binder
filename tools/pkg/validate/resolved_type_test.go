package validate

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPrimitiveTypeString(t *testing.T) {
	p := &PrimitiveType{Name: "int"}
	assert.Equal(t, "int", p.String())
}

func TestVoidTypeString(t *testing.T) {
	v := &VoidType{}
	assert.Equal(t, "void", v.String())
}

func TestStringTypeString(t *testing.T) {
	s := &StringType{}
	assert.Equal(t, "String", s.String())

	s.UTF8 = true
	assert.Equal(t, "String(utf8)", s.String())
}

func TestBinderTypeString(t *testing.T) {
	b := &BinderType{}
	assert.Equal(t, "IBinder", b.String())
}

func TestFileDescriptorTypeString(t *testing.T) {
	f := &FileDescriptorType{}
	assert.Equal(t, "ParcelFileDescriptor", f.String())
}

func TestInterfaceTypeString(t *testing.T) {
	i := &InterfaceType{QualifiedName: "android.os.IServiceManager"}
	assert.Equal(t, "android.os.IServiceManager", i.String())
}

func TestParcelableTypeString(t *testing.T) {
	p := &ParcelableType{QualifiedName: "com.example.Data"}
	assert.Equal(t, "com.example.Data", p.String())
}

func TestEnumTypeString(t *testing.T) {
	e := &EnumType{QualifiedName: "com.example.Status"}
	assert.Equal(t, "com.example.Status", e.String())
}

func TestUnionTypeString(t *testing.T) {
	u := &UnionType{QualifiedName: "com.example.Result"}
	assert.Equal(t, "com.example.Result", u.String())
}

func TestListTypeString(t *testing.T) {
	l := &ListType{ElementType: &StringType{}}
	assert.Equal(t, "List<String>", l.String())
}

func TestMapTypeString(t *testing.T) {
	m := &MapType{
		KeyType:   &StringType{},
		ValueType: &PrimitiveType{Name: "int"},
	}
	assert.Equal(t, "Map<String, int>", m.String())
}

func TestArrayTypeString(t *testing.T) {
	a := &ArrayType{ElementType: &PrimitiveType{Name: "int"}}
	assert.Equal(t, "int[]", a.String())
}

func TestNullableTypeString(t *testing.T) {
	n := &NullableType{Inner: &StringType{}}
	assert.Equal(t, "@nullable String", n.String())
}

func TestResolvedTypeInterface(t *testing.T) {
	// Verify all types satisfy ResolvedType at compile time.
	var _ ResolvedType = &PrimitiveType{}
	var _ ResolvedType = &VoidType{}
	var _ ResolvedType = &StringType{}
	var _ ResolvedType = &BinderType{}
	var _ ResolvedType = &FileDescriptorType{}
	var _ ResolvedType = &InterfaceType{}
	var _ ResolvedType = &ParcelableType{}
	var _ ResolvedType = &EnumType{}
	var _ ResolvedType = &UnionType{}
	var _ ResolvedType = &ListType{}
	var _ ResolvedType = &MapType{}
	var _ ResolvedType = &ArrayType{}
	var _ ResolvedType = &NullableType{}
}
