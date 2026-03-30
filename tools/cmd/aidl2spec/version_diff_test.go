package main

import (
	"testing"

	"github.com/AndroidGoLab/binder/tools/pkg/spec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiffMethodParams(t *testing.T) {
	oldMethod := spec.MethodSpec{
		Name: "registerClient",
		Params: []spec.ParamSpec{
			{Name: "appId", Type: spec.TypeRef{Name: "ParcelUuid"}},
			{Name: "callback", Type: spec.TypeRef{Name: "IBluetoothGattCallback"}},
			{Name: "eatt_support", Type: spec.TypeRef{Name: "boolean"}},
			{Name: "transport", Type: spec.TypeRef{Name: "int"}},
		},
	}
	newMethod := spec.MethodSpec{
		Name: "registerClient",
		Params: []spec.ParamSpec{
			{Name: "appId", Type: spec.TypeRef{Name: "ParcelUuid"}},
			{Name: "callback", Type: spec.TypeRef{Name: "IBluetoothGattCallback"}},
			{Name: "eatt_support", Type: spec.TypeRef{Name: "boolean"}},
			{Name: "transport", Type: spec.TypeRef{Name: "int"}},
			{Name: "attributionSource", Type: spec.TypeRef{Name: "AttributionSource"}},
		},
	}

	result := diffMethodParams(oldMethod, newMethod, 34, 36)
	require.Len(t, result, 5)
	assert.Equal(t, 0, result[0].MinAPILevel) // appId: present in both
	assert.Equal(t, 0, result[3].MinAPILevel) // transport: present in both
	assert.Equal(t, 36, result[4].MinAPILevel) // attributionSource: added in 36
}

func TestDiffMethodParams_NoChange(t *testing.T) {
	method := spec.MethodSpec{
		Name: "disconnect",
		Params: []spec.ParamSpec{
			{Name: "clientIf", Type: spec.TypeRef{Name: "int"}},
			{Name: "address", Type: spec.TypeRef{Name: "String"}},
		},
	}

	result := diffMethodParams(method, method, 34, 36)
	require.Len(t, result, 2)
	assert.Equal(t, 0, result[0].MinAPILevel)
	assert.Equal(t, 0, result[1].MinAPILevel)
}

func TestDiffMethodParams_AllNew(t *testing.T) {
	oldMethod := spec.MethodSpec{
		Name: "newMethod",
	}
	newMethod := spec.MethodSpec{
		Name: "newMethod",
		Params: []spec.ParamSpec{
			{Name: "param1", Type: spec.TypeRef{Name: "int"}},
			{Name: "param2", Type: spec.TypeRef{Name: "String"}},
		},
	}

	result := diffMethodParams(oldMethod, newMethod, 34, 36)
	require.Len(t, result, 2)
	assert.Equal(t, 36, result[0].MinAPILevel)
	assert.Equal(t, 36, result[1].MinAPILevel)
}
