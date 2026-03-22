package resolver

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/AndroidGoLab/binder/tools/pkg/parser"
)

func TestTypeRegistry_RegisterAndLookup(t *testing.T) {
	reg := NewTypeRegistry()

	def := &parser.InterfaceDecl{IntfName: "IFoo"}
	reg.Register("com.example.IFoo", def)

	got, ok := reg.Lookup("com.example.IFoo")
	require.True(t, ok)
	assert.Equal(t, def, got)
}

func TestTypeRegistry_LookupNotFound(t *testing.T) {
	reg := NewTypeRegistry()

	_, ok := reg.Lookup("com.example.NonExistent")
	assert.False(t, ok)
}

func TestTypeRegistry_All_ReturnsCopy(t *testing.T) {
	reg := NewTypeRegistry()

	def1 := &parser.InterfaceDecl{IntfName: "IFoo"}
	def2 := &parser.ParcelableDecl{ParcName: "Data"}
	reg.Register("com.example.IFoo", def1)
	reg.Register("com.example.Data", def2)

	all := reg.All()
	require.Len(t, all, 2)
	assert.Equal(t, def1, all["com.example.IFoo"])
	assert.Equal(t, def2, all["com.example.Data"])

	// Mutating the returned map must not affect the registry.
	delete(all, "com.example.IFoo")
	_, ok := reg.Lookup("com.example.IFoo")
	assert.True(t, ok, "deleting from All() result must not affect registry")
}

func TestTypeRegistry_RegisterOverwrite(t *testing.T) {
	reg := NewTypeRegistry()

	def1 := &parser.InterfaceDecl{IntfName: "IFoo"}
	def2 := &parser.InterfaceDecl{IntfName: "IFooV2"}
	reg.Register("com.example.IFoo", def1)
	reg.Register("com.example.IFoo", def2)

	got, ok := reg.Lookup("com.example.IFoo")
	require.True(t, ok)
	assert.Equal(t, def2, got)
}
