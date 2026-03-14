package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCamelToKebab(t *testing.T) {
	assert.Equal(t, "is-user-a-monkey", camelToKebab("IsUserAMonkey"))
	assert.Equal(t, "get-process-limit", camelToKebab("GetProcessLimit"))
	assert.Equal(t, "check-permission", camelToKebab("CheckPermission"))
	assert.Equal(t, "get-gpu-context-priority", camelToKebab("GetGpuContextPriority"))
	assert.Equal(t, "set-wifi-enabled", camelToKebab("SetWifiEnabled"))
	assert.Equal(t, "open-content-uri", camelToKebab("OpenContentUri"))
}

func TestRegistryLookupByDescriptor(t *testing.T) {
	r := &Registry{
		Services: map[string]*ServiceInfo{
			"android.app.IActivityManager": {
				Descriptor: "android.app.IActivityManager",
				Aliases:    []string{"activity"},
				Methods:    []MethodInfo{{Name: "IsUserAMonkey", ReturnType: "bool"}},
			},
		},
	}

	info := r.ByDescriptor("android.app.IActivityManager")
	assert.NotNil(t, info)
	assert.Equal(t, "android.app.IActivityManager", info.Descriptor)

	info = r.ByAlias("activity")
	assert.NotNil(t, info)
	assert.Equal(t, "android.app.IActivityManager", info.Descriptor)

	info = r.ByAlias("nonexistent")
	assert.Nil(t, info)
}
