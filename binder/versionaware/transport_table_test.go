package versionaware

import (
	"testing"

	"github.com/AndroidGoLab/binder/binder"
	"github.com/stretchr/testify/assert"
)

func TestTransport_ActiveTable(t *testing.T) {
	table := VersionTable{
		"android.app.IFoo": {
			"doThing": binder.FirstCallTransaction + 0,
		},
	}
	tr := &Transport{table: table}
	got := tr.ActiveTable()
	assert.Equal(t, binder.FirstCallTransaction, got.Resolve("android.app.IFoo", "doThing"))
}
