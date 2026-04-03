package gralloc

import "sync"

// Mapper provides CPU access to gralloc buffers.
type Mapper interface {
	LockBuffer(b *Buffer) ([]byte, error)
}

// GetMapper returns a Mapper for the current device. The result is cached.
// Returns (nil, nil) when no mapper is available (real devices where mmap
// works directly).
func GetMapper() (Mapper, error) {
	globalMapperOnce.Do(func() {
		globalMapper, globalMapperErr = newBridgeMapper()
	})
	return globalMapper, globalMapperErr
}

var (
	globalMapper     Mapper
	globalMapperOnce sync.Once
	globalMapperErr  error
)
