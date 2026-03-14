package validate

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsBuiltin(t *testing.T) {
	builtins := []string{
		"void", "boolean", "byte", "char", "int", "long",
		"float", "double", "String", "IBinder", "ParcelFileDescriptor",
	}
	for _, name := range builtins {
		assert.True(t, IsBuiltin(name), "expected %q to be builtin", name)
	}

	assert.False(t, IsBuiltin("MyCustomType"))
	assert.False(t, IsBuiltin("List"))
	assert.False(t, IsBuiltin("Map"))
	assert.False(t, IsBuiltin(""))
}

func TestBuiltinTypesString(t *testing.T) {
	assert.Equal(t, "void", BuiltinTypes["void"].String())
	assert.Equal(t, "int", BuiltinTypes["int"].String())
	assert.Equal(t, "String", BuiltinTypes["String"].String())
	assert.Equal(t, "IBinder", BuiltinTypes["IBinder"].String())
	assert.Equal(t, "ParcelFileDescriptor", BuiltinTypes["ParcelFileDescriptor"].String())
}
