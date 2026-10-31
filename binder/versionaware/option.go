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

// optionSurfaceComposerAndroid14R35 selects the Android 14 r35 native contract.
//
// Example: explicitly configure a device known to use that contract.
type optionSurfaceComposerAndroid14R35 struct{}

// apply installs the SurfaceComposer transaction codes for Android 14 r35.
//
// Example: apply the option during transport construction.
func (optionSurfaceComposerAndroid14R35) apply(c *config) {
	c.TransactionCodes = VersionTable{
		"android.gui.ISurfaceComposer": {
			"getPhysicalDisplayIds": 6,
			"captureDisplayById":    26,
		},
	}
}

// OptionSurfaceComposerAndroid14R35 selects the native SurfaceComposer wire
// contract from AOSP android-14.0.0_r35. The caller must establish that the
// device uses this contract; API level 34 alone is insufficient because other
// Android 14 revisions have incompatible captureDisplayById signatures.
//
// Example: select it for a device verified against the r35 contract.
func OptionSurfaceComposerAndroid14R35() Option { return optionSurfaceComposerAndroid14R35{} }
