package versionaware

// Option configures a version-aware Transport.
type Option interface {
	apply(*config)
}

// Options is a slice of Option.
type Options []Option

func (opts Options) config() config {
	cfg := config{
		CachePath: defaultCachePath,
	}
	for _, o := range opts {
		o.apply(&cfg)
	}
	return cfg
}

// defaultCachePath is the default location for caching resolved
// transaction code tables. Uses tmpfs (/dev/shm on Linux,
// /data/local/tmp on Android) for fast access without disk I/O.
const defaultCachePath = "/tmp/.binder_cache/codes.gob"

type config struct {
	// CachePath is the file path where the resolved VersionTable
	// is cached. Empty string disables caching.
	CachePath string
}

type optionCachePath struct{ path string }

func (o optionCachePath) apply(c *config) { c.CachePath = o.path }

// OptionCachePath sets the file path for caching the resolved transaction
// code table. The cache includes a fingerprint so it is automatically
// invalidated when the OS is updated.
//
// Default: /tmp/.binder_cache/codes.gob
// Pass empty string to disable caching.
func OptionCachePath(path string) Option { return optionCachePath{path: path} }
