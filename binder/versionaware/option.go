package versionaware

// Option configures a version-aware Transport.
type Option interface {
	apply(*config)
}

// Options is a slice of Option.
type Options []Option

func (opts Options) config() config {
	cfg := config{}
	for _, o := range opts {
		o.apply(&cfg)
	}
	return cfg
}

type config struct {
	// CachePath is the file path where the resolved VersionTable
	// is cached. Empty means caching is disabled.
	CachePath string
}

type optionCachePath struct{ path string }

func (o optionCachePath) apply(c *config) { c.CachePath = o.path }

// OptionCachePath enables caching of the resolved transaction code
// table to the given file path. The cache includes a fingerprint
// so it is automatically invalidated when the OS is updated.
//
// When not set (default), no caching is performed.
func OptionCachePath(path string) Option { return optionCachePath{path: path} }
