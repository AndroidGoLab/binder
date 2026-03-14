package validate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xaionaro-go/aidl/tools/pkg/parser"
)

func TestValidate_BuiltinTypes(t *testing.T) {
	doc, err := parser.Parse("test.aidl", []byte(`
		package com.example;
		interface IFoo {
			void doStuff(in int x, in String s);
			boolean isReady(in long id);
		}
	`))
	require.NoError(t, err)

	errs := Validate(doc, nil)
	assert.Empty(t, errs)
}

func TestValidate_UnresolvedType(t *testing.T) {
	doc, err := parser.Parse("test.aidl", []byte(`
		package com.example;
		interface IFoo {
			void doStuff(in UnknownType x);
		}
	`))
	require.NoError(t, err)

	errs := Validate(doc, nil)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "unresolved type: UnknownType")
}

func TestValidate_OnewayMustReturnVoid(t *testing.T) {
	doc, err := parser.Parse("test.aidl", []byte(`
		package com.example;
		interface IFoo {
			oneway int doStuff(in int x);
		}
	`))
	require.NoError(t, err)

	errs := Validate(doc, nil)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "oneway method must return void")
}

func TestValidate_OnewayParamsMustBeIn(t *testing.T) {
	doc, err := parser.Parse("test.aidl", []byte(`
		package com.example;
		interface IFoo {
			oneway void doStuff(out int x);
		}
	`))
	require.NoError(t, err)

	errs := Validate(doc, nil)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "oneway method parameters must be 'in'")
}

func TestValidate_InterfaceOneway(t *testing.T) {
	doc, err := parser.Parse("test.aidl", []byte(`
		package com.example;
		oneway interface IFoo {
			int doStuff(in int x);
		}
	`))
	require.NoError(t, err)

	errs := Validate(doc, nil)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "oneway method must return void")
}

func TestValidate_MissingDirection(t *testing.T) {
	doc, err := parser.Parse("test.aidl", []byte(`
		package com.example;
		interface IFoo {
			void doStuff(int x);
		}
	`))
	require.NoError(t, err)

	errs := Validate(doc, nil)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "must have a direction")
}

func TestValidate_EnumBackingType(t *testing.T) {
	doc, err := parser.Parse("test.aidl", []byte(`
		package com.example;
		@Backing(type="float")
		enum Status {
			OK = 0,
		}
	`))
	require.NoError(t, err)

	errs := Validate(doc, nil)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "enum backing type must be byte, int, or long")
}

func TestValidate_ValidEnumBackingTypes(t *testing.T) {
	for _, backingType := range []string{"byte", "int", "long"} {
		t.Run(backingType, func(t *testing.T) {
			doc, err := parser.Parse("test.aidl", []byte(`
				package com.example;
				@Backing(type="`+backingType+`")
				enum Status {
					OK = 0,
				}
			`))
			require.NoError(t, err)

			errs := Validate(doc, nil)
			assert.Empty(t, errs)
		})
	}
}

func TestValidate_ParcelableFieldTypes(t *testing.T) {
	doc, err := parser.Parse("test.aidl", []byte(`
		package com.example;
		parcelable Data {
			int id;
			String name;
			UnknownType bad;
		}
	`))
	require.NoError(t, err)

	errs := Validate(doc, nil)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "unresolved type: UnknownType")
}

func TestValidate_LookupTypeResolvesCustomType(t *testing.T) {
	doc, err := parser.Parse("test.aidl", []byte(`
		package com.example;
		import com.other.MyData;
		interface IFoo {
			void doStuff(in MyData data);
		}
	`))
	require.NoError(t, err)

	lookup := func(qualifiedName string) bool {
		return qualifiedName == "com.other.MyData"
	}

	errs := Validate(doc, lookup)
	assert.Empty(t, errs)
}

func TestValidate_GenericTypes(t *testing.T) {
	doc, err := parser.Parse("test.aidl", []byte(`
		package com.example;
		parcelable Data {
			List<String> names;
			Map<String, int> values;
		}
	`))
	require.NoError(t, err)

	errs := Validate(doc, nil)
	assert.Empty(t, errs)
}

func TestValidate_UnionFields(t *testing.T) {
	doc, err := parser.Parse("test.aidl", []byte(`
		package com.example;
		union Result {
			int intValue;
			String strValue;
			UnknownType bad;
		}
	`))
	require.NoError(t, err)

	errs := Validate(doc, nil)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "unresolved type: UnknownType")
}

func TestValidationError_Error(t *testing.T) {
	e := &ValidationError{
		Pos:     parser.Position{Filename: "test.aidl", Line: 5, Column: 10},
		Message: "something is wrong",
	}
	assert.Equal(t, "test.aidl:5:10: something is wrong", e.Error())
}

func TestValidate_SamePackageResolution(t *testing.T) {
	doc, err := parser.Parse("test.aidl", []byte(`
		package com.example;
		interface IFoo {
			void doStuff(in MyData data);
		}
	`))
	require.NoError(t, err)

	lookup := func(qualifiedName string) bool {
		return qualifiedName == "com.example.MyData"
	}

	errs := Validate(doc, lookup)
	assert.Empty(t, errs)
}
