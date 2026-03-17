//go:build linux

package kernelbinder

import "unsafe"

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

// Compile-time size assertion.
var _ [64]byte = [unsafe.Sizeof(binderTransactionData{})]byte{}

// Verify pre-allocated buffer sizes match the struct sizes they contain.
var _ [replyWriteBufSize]byte = [4 + unsafe.Sizeof(binderTransactionData{})]byte{}
