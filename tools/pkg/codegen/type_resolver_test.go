package codegen

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/AndroidGoLab/binder/tools/pkg/parser"
	"github.com/AndroidGoLab/binder/tools/pkg/resolver"
)

func TestIsForwardDeclared_CppHeader(t *testing.T) {
	reg := resolver.NewTypeRegistry()
	reg.Register("android.os.NativeHandle", &parser.ParcelableDecl{
		ParcName:  "NativeHandle",
		CppHeader: "cutils/native_handle.h",
	})

	r := NewTypeRefResolver(reg, "android.os", NewGoFile("os"))
	assert.True(t, r.isForwardDeclared("android.os.NativeHandle"))
}

func TestIsForwardDeclared_NdkHeader(t *testing.T) {
	reg := resolver.NewTypeRegistry()
	reg.Register("android.os.SomeNdk", &parser.ParcelableDecl{
		ParcName:  "SomeNdk",
		NdkHeader: "some/ndk_header.h",
	})

	r := NewTypeRefResolver(reg, "android.os", NewGoFile("os"))
	assert.True(t, r.isForwardDeclared("android.os.SomeNdk"))
}

func TestIsForwardDeclared_RustType(t *testing.T) {
	reg := resolver.NewTypeRegistry()
	reg.Register("android.os.SomeRust", &parser.ParcelableDecl{
		ParcName: "SomeRust",
		RustType: "some::RustType",
	})

	r := NewTypeRefResolver(reg, "android.os", NewGoFile("os"))
	assert.True(t, r.isForwardDeclared("android.os.SomeRust"))
}

func TestIsForwardDeclared_JavaOnlyEmpty(t *testing.T) {
	reg := resolver.NewTypeRegistry()
	reg.Register("android.content.Intent", &parser.ParcelableDecl{
		ParcName: "Intent",
	})

	r := NewTypeRefResolver(reg, "android.content", NewGoFile("content"))
	assert.False(t, r.isForwardDeclared("android.content.Intent"))
}

func TestIsForwardDeclared_WithConstants(t *testing.T) {
	reg := resolver.NewTypeRegistry()
	reg.Register("android.os.Config", &parser.ParcelableDecl{
		ParcName: "Config",
		Constants: []*parser.ConstantDecl{
			{
				ConstName: "VERSION",
				Type:      &parser.TypeSpecifier{Name: "int"},
				Value:     &parser.IntegerLiteral{Value: "1"},
			},
		},
	})

	r := NewTypeRefResolver(reg, "android.os", NewGoFile("os"))
	assert.False(t, r.isForwardDeclared("android.os.Config"))
}

func TestIsForwardDeclared_WithFields(t *testing.T) {
	reg := resolver.NewTypeRegistry()
	reg.Register("android.os.Info", &parser.ParcelableDecl{
		ParcName: "Info",
		Fields: []*parser.FieldDecl{
			{
				FieldName: "name",
				Type:      &parser.TypeSpecifier{Name: "String"},
			},
		},
	})

	r := NewTypeRefResolver(reg, "android.os", NewGoFile("os"))
	assert.False(t, r.isForwardDeclared("android.os.Info"))
}

func TestIsForwardDeclared_WithNestedTypes(t *testing.T) {
	reg := resolver.NewTypeRegistry()
	reg.Register("android.os.Container", &parser.ParcelableDecl{
		ParcName: "Container",
		NestedTypes: []parser.Definition{
			&parser.EnumDecl{EnumName: "Status"},
		},
	})

	r := NewTypeRefResolver(reg, "android.os", NewGoFile("os"))
	assert.False(t, r.isForwardDeclared("android.os.Container"))
}

func TestIsForwardDeclared_NonParcelable(t *testing.T) {
	reg := resolver.NewTypeRegistry()
	reg.Register("android.os.IService", &parser.InterfaceDecl{
		IntfName: "IService",
	})

	r := NewTypeRefResolver(reg, "android.os", NewGoFile("os"))
	assert.False(t, r.isForwardDeclared("android.os.IService"))
}

func TestIsForwardDeclared_NotFound(t *testing.T) {
	reg := resolver.NewTypeRegistry()

	r := NewTypeRefResolver(reg, "android.os", NewGoFile("os"))
	assert.False(t, r.isForwardDeclared("android.os.Unknown"))
}

func TestIsForwardDeclared_NilRegistry(t *testing.T) {
	r := NewTypeRefResolver(nil, "android.os", NewGoFile("os"))
	assert.False(t, r.isForwardDeclared("android.os.Anything"))
}

func TestReserveNames_AvoidsParamNameClash(t *testing.T) {
	// When a parameter named "provider" exists and the import path ends
	// in "provider", the alias must be disambiguated to avoid shadowing.
	reg := resolver.NewTypeRegistry()
	reg.Register("android.location.provider.ProviderProperties", &parser.ParcelableDecl{
		ParcName: "ProviderProperties",
	})

	f := NewGoFile("location")
	r := NewTypeRefResolver(reg, "android.location", f)

	// Reserve "provider" as if a method has a parameter named "provider".
	r.ReserveNames([]string{"provider"})

	goType := r.GoTypeRef(&parser.TypeSpecifier{Name: "android.location.provider.ProviderProperties"})

	// The alias must NOT be "provider" because that's a reserved param name.
	assert.NotEqual(t, "provider.ProviderProperties", goType,
		"import alias must not clash with reserved parameter name")
	assert.Contains(t, goType, "ProviderProperties",
		"type name must still reference ProviderProperties")

	// Verify the alias used in the import is not "provider".
	for _, alias := range f.Imports {
		assert.NotEqual(t, "provider", alias,
			"import alias must not match reserved parameter name")
	}
}

func TestReserveNames_NoClashWhenParamDiffers(t *testing.T) {
	// When no parameter name matches the import alias, it stays short.
	reg := resolver.NewTypeRegistry()
	reg.Register("android.location.provider.ProviderProperties", &parser.ParcelableDecl{
		ParcName: "ProviderProperties",
	})

	f := NewGoFile("location")
	r := NewTypeRefResolver(reg, "android.location", f)

	// Reserve a name that does NOT match "provider".
	r.ReserveNames([]string{"name", "enabled"})

	goType := r.GoTypeRef(&parser.TypeSpecifier{Name: "android.location.provider.ProviderProperties"})

	// The alias should remain "provider" since no conflict exists.
	assert.Equal(t, "provider.ProviderProperties", goType)
}

func TestIsForwardDeclared_CppHeaderWithConstants(t *testing.T) {
	// A parcelable with CppHeader but also constants should NOT be
	// considered forward-declared (it has real content).
	reg := resolver.NewTypeRegistry()
	reg.Register("android.os.Hybrid", &parser.ParcelableDecl{
		ParcName:  "Hybrid",
		CppHeader: "some/header.h",
		Constants: []*parser.ConstantDecl{
			{
				ConstName: "FLAG",
				Type:      &parser.TypeSpecifier{Name: "int"},
				Value:     &parser.IntegerLiteral{Value: "0"},
			},
		},
	})

	r := NewTypeRefResolver(reg, "android.os", NewGoFile("os"))
	assert.False(t, r.isForwardDeclared("android.os.Hybrid"))
}
