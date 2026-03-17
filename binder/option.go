package binder

// Option configures a Transport.
type Option interface {
	apply(*Config)
}

// Options is a slice of Option.
type Options []Option

// Config applies all options to the default configuration and returns the result.
func (opts Options) Config() Config {
	cfg := DefaultConfig()
	for _, o := range opts {
		o.apply(&cfg)
	}
	return cfg
}

type maxThreadsOption struct{ n uint32 }

func (o maxThreadsOption) apply(c *Config) { c.MaxThreads = o.n }

// WithMaxThreads sets the maximum number of Binder threads.
func WithMaxThreads(n uint32) Option { return maxThreadsOption{n: n} }

type mapSizeOption struct{ n uint32 }

func (o mapSizeOption) apply(c *Config) { c.MapSize = o.n }

// WithMapSize sets the mmap size for the Binder driver.
func WithMapSize(n uint32) Option { return mapSizeOption{n: n} }
