package versionaware

import "github.com/xaionaro-go/binder/binder"

// VersionTable maps descriptor -> methodName -> transaction code.
type VersionTable map[string]map[string]binder.TransactionCode

// Resolve looks up the transaction code for a method.
// Returns 0 if not found.
func (t VersionTable) Resolve(
	descriptor string,
	method string,
) binder.TransactionCode {
	methods, ok := t[descriptor]
	if !ok {
		return 0
	}
	return methods[method]
}
