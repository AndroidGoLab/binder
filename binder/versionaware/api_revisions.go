package versionaware

// APIRevisions maps API level -> list of Revisions (for probing order).
// Within an API level, later revisions are listed first (more likely match).
type APIRevisions map[int][]Revision
