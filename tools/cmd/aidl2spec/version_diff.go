package main

import (
	"github.com/AndroidGoLab/binder/tools/pkg/spec"
)

// diffMethodParams compares oldMethod (from oldAPI) with newMethod (from newAPI)
// and sets MinAPILevel on params that were added in newAPI.
// Only trailing param additions are detected (AIDL stability guarantees
// params are never removed or reordered).
func diffMethodParams(
	oldMethod spec.MethodSpec,
	newMethod spec.MethodSpec,
	oldAPI int,
	newAPI int,
) []spec.ParamSpec {
	result := make([]spec.ParamSpec, len(newMethod.Params))
	copy(result, newMethod.Params)

	oldCount := len(oldMethod.Params)
	for i := range result {
		if i >= oldCount {
			result[i].MinAPILevel = newAPI
		}
	}
	return result
}
