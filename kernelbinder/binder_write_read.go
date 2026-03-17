//go:build linux

package kernelbinder

import "unsafe"

// binderWriteRead is the struct passed to BINDER_WRITE_READ ioctl.
type binderWriteRead struct {
	writeSize     uint64
	writeConsumed uint64
	writeBuffer   uint64 // pointer to write commands
	readSize      uint64
	readConsumed  uint64
	readBuffer    uint64 // pointer to read buffer
}

const binderCurrentProtocolVersion = 8

// Compile-time size assertion.
var _ [48]byte = [unsafe.Sizeof(binderWriteRead{})]byte{}

// Verify pre-allocated buffer sizes match the struct sizes they contain.
var _ [freeBufferBufSize]byte = [4 + 8]byte{}
