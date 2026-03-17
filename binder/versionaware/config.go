package versionaware

// defaultCachePath is the default location for caching resolved
// transaction code tables. Uses tmpfs (/dev/shm on Linux,
// /data/local/tmp on Android) for fast access without disk I/O.
const defaultCachePath = "/tmp/.binder_cache/codes.gob"

type config struct {
	// CachePath is the file path where the resolved VersionTable
	// is cached. Empty string disables caching.
	CachePath string
}
