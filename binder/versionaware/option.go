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

type optionCachePath struct{ path string }

func (o optionCachePath) apply(c *config) { c.CachePath = o.path }

// OptionCachePath sets the file path for caching the resolved transaction
// code table. The cache includes a fingerprint so it is automatically
// invalidated when the OS is updated.
//
// Default: /tmp/.binder_cache/codes.gob
// Pass empty string to disable caching.
func OptionCachePath(path string) Option { return optionCachePath{path: path} }
