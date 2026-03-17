package versionaware

// cachedTable is the on-disk format for gob-encoded transaction code caches.
// Using a named type because gob works better with registered named types.
//
// The Table field holds the gob-serializable raw form. After
// deserialization, loadCachedTable converts it into ResolvedTable
// (a VersionTable with proper binder.TransactionCode values).
type cachedTable struct {
	Fingerprint   string
	Table         map[string]map[string]uint32
	ScannedJARs   []string
	ResolvedTable VersionTable `gob:"-"`
}
