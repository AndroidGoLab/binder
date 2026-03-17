package binder

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
