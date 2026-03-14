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

// binderTransactionData is the data for BC_TRANSACTION/BC_REPLY and BR_TRANSACTION/BR_REPLY.
// The target and cookie fields are binder_uintptr_t (uint64 on 64-bit).
type binderTransactionData struct {
	target        uint64 // binder_uintptr_t: handle for BC_TRANSACTION, ptr for BR_TRANSACTION
	cookie        uint64 // binder_uintptr_t: only for BR_TRANSACTION
	code          uint32
	flags         uint32
	senderPID     int32
	senderEUID    uint32
	dataSize      uint64
	offsetsSize   uint64
	dataBuffer    uint64 // pointer to data
	offsetsBuffer uint64 // pointer to offsets
}

const binderCurrentProtocolVersion = 8

// binderTypeHandle is BINDER_TYPE_HANDLE: B_PACK_CHARS('s','h','*',0x85) = 0x73682a85.
// Used to identify flat_binder_object entries containing remote binder handles.
const binderTypeHandle = uint32(0x73682a85)

// Compile-time size assertions.
var _ [48]byte = [unsafe.Sizeof(binderWriteRead{})]byte{}
var _ [64]byte = [unsafe.Sizeof(binderTransactionData{})]byte{}
