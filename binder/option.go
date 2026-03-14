package binder

// Option configures a Transport.
type Option interface {
	apply(*Config)
}

// Config holds Transport configuration.
type Config struct {
	MaxThreads uint32
	MapSize    uint32
}

// DefaultConfig returns the default transport configuration.
func DefaultConfig() Config {
	return Config{
		MaxThreads: 0,
		MapSize:    1024*1024 - 2*4096, // 1MB - 2*PAGE_SIZE
	}
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
