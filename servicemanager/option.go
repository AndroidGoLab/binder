package servicemanager

import "github.com/AndroidGoLab/binder/binder"

// Option configures a ServiceManager.
type Option interface {
	apply(*config)
}

// Options is a slice of Option.
type Options []Option

func (opts Options) config() config {
	cfg := defaultConfig()
	for _, o := range opts {
		o.apply(&cfg)
	}
	return cfg
}

type config struct {
	Identity binder.CallerIdentity
}

func defaultConfig() config {
	return config{
		Identity: binder.DefaultCallerIdentity(),
	}
}

type optionIdentity struct{ identity binder.CallerIdentity }

func (o optionIdentity) apply(c *config) { c.Identity = o.identity }

// OptionIdentity sets the caller identity used when creating
// ProxyBinder instances for resolved services.
func OptionIdentity(identity binder.CallerIdentity) Option {
	return optionIdentity{identity: identity}
}
