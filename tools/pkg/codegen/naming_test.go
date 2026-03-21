package codegen

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/xaionaro-go/binder/tools/pkg/parser"
)

func TestAIDLToGoName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"getService", "GetService"},
		{"FIRST_CALL_TRANSACTION", "FirstCallTransaction"},
		{"IServiceManager", "IServiceManager"},
		{"oneway", "Oneway"},
		{"", ""},
		{"STATUS_OK", "StatusOk"},
		{"a", "A"},
		{"ABC", "ABC"},
		{"name", "Name"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, AIDLToGoName(tt.input))
		})
	}
}

func TestAIDLToGoPackage(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"android.os", "android/os"},
		{"com.example.service", "com/example/service"},
		{"com.android.internal.foo", "com/android/internal_/foo"},
		{"com.android.ims.internal", "com/android/ims/internal_"},
		{"simple", "simple"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, AIDLToGoPackage(tt.input))
		})
	}
}

func TestAIDLToGoFileName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"IServiceManager", "iservicemanager.go"},
		{"MyParcelable", "myparcelable.go"},
		{"Status", "status.go"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, AIDLToGoFileName(tt.input))
		})
	}
}

func TestAIDLTypeToGo_Primitives(t *testing.T) {
	tests := []struct {
		name     string
		expected string
	}{
		{"void", ""},
		{"boolean", "bool"},
		{"byte", "byte"},
		{"char", "uint16"},
		{"int", "int32"},
		{"long", "int64"},
		{"float", "float32"},
		{"double", "float64"},
		{"String", "string"},
		{"IBinder", "binder.IBinder"},
		{"ParcelFileDescriptor", "int32"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := &parser.TypeSpecifier{Name: tt.name}
			assert.Equal(t, tt.expected, AIDLTypeToGo(ts))
		})
	}
}

func TestAIDLTypeToGo_Generics(t *testing.T) {
	t.Run("List<String>", func(t *testing.T) {
		ts := &parser.TypeSpecifier{
			Name: "List",
			TypeArgs: []*parser.TypeSpecifier{
				{Name: "String"},
			},
		}
		assert.Equal(t, "[]string", AIDLTypeToGo(ts))
	})

	t.Run("List<int>", func(t *testing.T) {
		ts := &parser.TypeSpecifier{
			Name: "List",
			TypeArgs: []*parser.TypeSpecifier{
				{Name: "int"},
			},
		}
		assert.Equal(t, "[]int32", AIDLTypeToGo(ts))
	})

	t.Run("Map<String,int>", func(t *testing.T) {
		ts := &parser.TypeSpecifier{
			Name: "Map",
			TypeArgs: []*parser.TypeSpecifier{
				{Name: "String"},
				{Name: "int"},
			},
		}
		assert.Equal(t, "map[string]int32", AIDLTypeToGo(ts))
	})

	t.Run("List_no_type_args", func(t *testing.T) {
		ts := &parser.TypeSpecifier{Name: "List"}
		assert.Equal(t, "[]any", AIDLTypeToGo(ts))
	})

	t.Run("Map_no_type_args", func(t *testing.T) {
		ts := &parser.TypeSpecifier{Name: "Map"}
		assert.Equal(t, "map[any]any", AIDLTypeToGo(ts))
	})
}

func TestAIDLTypeToGo_Arrays(t *testing.T) {
	t.Run("int[]", func(t *testing.T) {
		ts := &parser.TypeSpecifier{Name: "int", IsArray: true}
		assert.Equal(t, "[]int32", AIDLTypeToGo(ts))
	})

	t.Run("String[]", func(t *testing.T) {
		ts := &parser.TypeSpecifier{Name: "String", IsArray: true}
		assert.Equal(t, "[]string", AIDLTypeToGo(ts))
	})

	t.Run("byte[]", func(t *testing.T) {
		ts := &parser.TypeSpecifier{Name: "byte", IsArray: true}
		assert.Equal(t, "[]byte", AIDLTypeToGo(ts))
	})

	t.Run("MyType[]", func(t *testing.T) {
		ts := &parser.TypeSpecifier{Name: "MyType", IsArray: true}
		assert.Equal(t, "[]MyType", AIDLTypeToGo(ts))
	})
}

func TestAIDLTypeToGo_Nullable(t *testing.T) {
	nullable := []*parser.Annotation{{Name: "nullable"}}

	t.Run("@nullable int", func(t *testing.T) {
		ts := &parser.TypeSpecifier{Name: "int", Annots: nullable}
		assert.Equal(t, "*int32", AIDLTypeToGo(ts))
	})

	t.Run("@nullable String", func(t *testing.T) {
		ts := &parser.TypeSpecifier{Name: "String", Annots: nullable}
		// String already has nullable semantics via empty string.
		assert.Equal(t, "string", AIDLTypeToGo(ts))
	})

	t.Run("@nullable List<int>", func(t *testing.T) {
		ts := &parser.TypeSpecifier{
			Name:   "List",
			Annots: nullable,
			TypeArgs: []*parser.TypeSpecifier{
				{Name: "int"},
			},
		}
		// Slices are already nullable (nil).
		assert.Equal(t, "[]int32", AIDLTypeToGo(ts))
	})

	t.Run("@nullable Map<String,int>", func(t *testing.T) {
		ts := &parser.TypeSpecifier{
			Name:   "Map",
			Annots: nullable,
			TypeArgs: []*parser.TypeSpecifier{
				{Name: "String"},
				{Name: "int"},
			},
		}
		// Maps are already nullable (nil).
		assert.Equal(t, "map[string]int32", AIDLTypeToGo(ts))
	})

	t.Run("@nullable IBinder", func(t *testing.T) {
		ts := &parser.TypeSpecifier{Name: "IBinder", Annots: nullable}
		// IBinder is an interface, but its Go type is "binder.IBinder" which
		// doesn't start with * or [ or map[, so gets a pointer prefix.
		// However, interfaces are naturally nullable, so this depends on policy.
		// Per the spec: non-pointer types get *, so binder.IBinder -> *binder.IBinder.
		assert.Equal(t, "*binder.IBinder", AIDLTypeToGo(ts))
	})

	t.Run("@nullable MyParcelable", func(t *testing.T) {
		ts := &parser.TypeSpecifier{Name: "MyParcelable", Annots: nullable}
		assert.Equal(t, "*MyParcelable", AIDLTypeToGo(ts))
	})
}

func TestAIDLTypeToGo_UserDefined(t *testing.T) {
	ts := &parser.TypeSpecifier{Name: "MyService"}
	assert.Equal(t, "MyService", AIDLTypeToGo(ts))
}

func TestAIDLTypeToGo_Nil(t *testing.T) {
	assert.Equal(t, "", AIDLTypeToGo(nil))
}
