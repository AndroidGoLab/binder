//go:build linux

package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatMethodSignature_NoParams_NoReturn(t *testing.T) {
	m := MethodInfo{Name: "ForceFlushCommands"}
	assert.Equal(t, "force-flush-commands()", formatMethodSignature(m))
}

func TestFormatMethodSignature_NoParams_WithReturn(t *testing.T) {
	m := MethodInfo{Name: "IsUserAMonkey", ReturnType: "bool"}
	assert.Equal(t, "is-user-a-monkey() -> bool", formatMethodSignature(m))
}

func TestFormatMethodSignature_WithParams_WithReturn(t *testing.T) {
	m := MethodInfo{
		Name: "CheckPermission",
		Params: []ParamInfo{
			{Name: "permission", Type: "string"},
			{Name: "pid", Type: "int32"},
			{Name: "uid", Type: "int32"},
		},
		ReturnType: "int32",
	}
	assert.Equal(t,
		"check-permission(permission string, pid int32, uid int32) -> int32",
		formatMethodSignature(m),
	)
}

func TestFormatMethodSignature_WithParams_NoReturn(t *testing.T) {
	m := MethodInfo{
		Name:   "SetByte",
		Params: []ParamInfo{{Name: "input", Type: "byte"}},
	}
	assert.Equal(t, "set-byte(input byte)", formatMethodSignature(m))
}

func TestMethodsToJSON(t *testing.T) {
	methods := []MethodInfo{
		{Name: "IsUserAMonkey", ReturnType: "bool"},
		{
			Name: "CheckPermission",
			Params: []ParamInfo{
				{Name: "permission", Type: "string"},
				{Name: "pid", Type: "int32"},
			},
			ReturnType: "int32",
		},
		{Name: "Restart"},
	}

	result := methodsToJSON(methods)
	require.Len(t, result, 3)

	assert.Equal(t, "is-user-a-monkey", result[0]["name"])
	assert.Equal(t, "bool", result[0]["return_type"])
	assert.Nil(t, result[0]["params"])

	assert.Equal(t, "check-permission", result[1]["name"])
	assert.Equal(t, "int32", result[1]["return_type"])
	params, ok := result[1]["params"].([]map[string]string)
	require.True(t, ok)
	require.Len(t, params, 2)
	assert.Equal(t, "permission", params[0]["name"])
	assert.Equal(t, "string", params[0]["type"])

	assert.Equal(t, "restart", result[2]["name"])
	assert.Nil(t, result[2]["return_type"])
	assert.Nil(t, result[2]["params"])
}
