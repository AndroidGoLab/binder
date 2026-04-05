//go:build linux

package main

import (
	"testing"

	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/binder/versionaware"
	"github.com/stretchr/testify/assert"
)

func TestResolveCodeToMethod(t *testing.T) {
	table := versionaware.VersionTable{
		"android.app.IFoo": {
			"doThing": binder.FirstCallTransaction + 0,
			"doOther": binder.FirstCallTransaction + 1,
		},
	}

	name, ok := resolveCodeToMethod(table, "android.app.IFoo", binder.FirstCallTransaction+1)
	assert.True(t, ok)
	assert.Equal(t, "doOther", name)
}

func TestResolveCodeToMethod_NotFound(t *testing.T) {
	table := versionaware.VersionTable{
		"android.app.IFoo": {
			"doThing": binder.FirstCallTransaction + 0,
		},
	}

	_, ok := resolveCodeToMethod(table, "android.app.IFoo", binder.FirstCallTransaction+99)
	assert.False(t, ok)
}

func TestResolveCodeToMethod_UnknownDescriptor(t *testing.T) {
	table := versionaware.VersionTable{}

	_, ok := resolveCodeToMethod(table, "android.app.IFoo", binder.FirstCallTransaction)
	assert.False(t, ok)
}

func TestResolveMethodToCode(t *testing.T) {
	table := versionaware.VersionTable{
		"android.app.IFoo": {
			"doThing": binder.FirstCallTransaction + 0,
			"doOther": binder.FirstCallTransaction + 1,
		},
	}

	code, ok := resolveMethodToCode(table, "android.app.IFoo", "doOther")
	assert.True(t, ok)
	assert.Equal(t, binder.FirstCallTransaction+1, code)
}

func TestResolveMethodToCode_NotFound(t *testing.T) {
	table := versionaware.VersionTable{
		"android.app.IFoo": {
			"doThing": binder.FirstCallTransaction + 0,
		},
	}

	_, ok := resolveMethodToCode(table, "android.app.IFoo", "noSuchMethod")
	assert.False(t, ok)
}

func TestGetActiveTable_NilConn(t *testing.T) {
	_, err := getActiveTable(nil)
	assert.Error(t, err)
}
