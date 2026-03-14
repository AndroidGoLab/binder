//go:build linux

package kernelbinder

import (
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
)

func TestIoctlNumbers(t *testing.T) {
	// BINDER_WRITE_READ = _IOWR('b', 1, struct binder_write_read)
	assert.Equal(t,
		uintptr(iowr('b', 1, unsafe.Sizeof(binderWriteRead{}))),
		binderWriteReadIoctl,
	)

	// BINDER_SET_MAX_THREADS = _IOW('b', 5, __u32)
	assert.Equal(t,
		uintptr(iow('b', 5, unsafe.Sizeof(uint32(0)))),
		binderSetMaxThreads,
	)

	// BINDER_VERSION = _IOWR('b', 9, struct binder_version)
	assert.Equal(t,
		uintptr(iowr('b', 9, unsafe.Sizeof(int32(0)))),
		binderVersionIoctl,
	)
}
