package versionaware

// MultiVersionTable maps Revision -> VersionTable.
// Revisions are like "34.r1", "35.r1", "36.r1", "36.r3", "36.r4".
type MultiVersionTable map[Revision]VersionTable
