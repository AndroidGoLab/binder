package versionaware

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/xaionaro-go/binder/binder"
)

func TestVersionTableResolve(t *testing.T) {
	table := VersionTable{
		"android.app.IActivityManager": {
			"isUserAMonkey":   binder.FirstCallTransaction + 105,
			"getProcessLimit": binder.FirstCallTransaction + 52,
		},
	}

	assert.Equal(t,
		binder.FirstCallTransaction+105,
		table.Resolve("android.app.IActivityManager", "isUserAMonkey"),
	)
	assert.Equal(t,
		binder.FirstCallTransaction+52,
		table.Resolve("android.app.IActivityManager", "getProcessLimit"),
	)
	assert.Equal(t,
		binder.TransactionCode(0),
		table.Resolve("android.app.IActivityManager", "nonExistent"),
	)
	assert.Equal(t,
		binder.TransactionCode(0),
		table.Resolve("nonexistent.IFoo", "bar"),
	)
}

func TestMultiVersionTableStringKeys(t *testing.T) {
	tables := MultiVersionTable{
		"36.r1": {
			"android.app.IActivityManager": {
				"isUserAMonkey": binder.FirstCallTransaction + 105,
			},
		},
		"36.r3": {
			"android.app.IActivityManager": {
				"isUserAMonkey": binder.FirstCallTransaction + 110,
			},
		},
	}

	assert.Equal(t,
		binder.FirstCallTransaction+105,
		tables["36.r1"].Resolve("android.app.IActivityManager", "isUserAMonkey"),
	)
	assert.Equal(t,
		binder.FirstCallTransaction+110,
		tables["36.r3"].Resolve("android.app.IActivityManager", "isUserAMonkey"),
	)

	_, exists := tables["36.r2"]
	assert.False(t, exists, "36.r2 should not exist (deduplicated)")
}

func TestAPIRevisions(t *testing.T) {
	revisions := APIRevisions{
		34: {"34.r1"},
		36: {"36.r4", "36.r3", "36.r1"},
	}

	assert.Equal(t, []Revision{"34.r1"}, revisions[34])
	assert.Equal(t, []Revision{"36.r4", "36.r3", "36.r1"}, revisions[36])
	assert.Nil(t, revisions[99], "unknown API level returns nil")
}

func TestTablesLookup(t *testing.T) {
	tables := MultiVersionTable{
		"36.r1": {
			"android.app.IActivityManager": {
				"isUserAMonkey": binder.FirstCallTransaction + 105,
			},
		},
		"36.r4": {
			"android.app.IActivityManager": {
				"isUserAMonkey": binder.FirstCallTransaction + 110,
			},
		},
	}

	// Exact match.
	table, ok := tables["36.r4"]
	assert.True(t, ok)
	assert.Equal(t, binder.FirstCallTransaction+110, table.Resolve("android.app.IActivityManager", "isUserAMonkey"))

	// Unknown version returns not-found.
	_, ok = tables["99.r1"]
	assert.False(t, ok, "unknown version should not exist")
}
