package versionaware

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/xaionaro-go/aidl/binder"
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
