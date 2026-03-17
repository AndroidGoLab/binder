//go:build linux

package kernelbinder

import (
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
)

func TestStructSizes(t *testing.T) {
	assert.Equal(t, uintptr(48), unsafe.Sizeof(binderWriteRead{}))
	assert.Equal(t, uintptr(64), unsafe.Sizeof(binderTransactionData{}))
}
