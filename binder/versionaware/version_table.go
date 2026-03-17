package versionaware

import "github.com/xaionaro-go/binder/binder"

// VersionTable maps descriptor -> methodName -> transaction code.
type VersionTable map[string]map[string]binder.TransactionCode

// MultiVersionTable maps Revision -> VersionTable.
// Revisions are like "34.r1", "35.r1", "36.r1", "36.r3", "36.r4".
type MultiVersionTable map[Revision]VersionTable

// APIRevisions maps API level -> list of Revisions (for probing order).
// Within an API level, later revisions are listed first (more likely match).
type APIRevisions map[int][]Revision

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
