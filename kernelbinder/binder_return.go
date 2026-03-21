//go:build linux

package kernelbinder

import "unsafe"

// binderReturn represents a binder return (BR_*) code read from the driver.
type binderReturn uint32

// Binder return (BR) codes -- read from the driver.
// These are var (not const) because the values use unsafe.Sizeof, which is
// not a constant expression in Go.
var (
	brError               = binderReturn(ior('r', 0, unsafe.Sizeof(int32(0))))
	brTransaction         = binderReturn(ior('r', 2, unsafe.Sizeof(binderTransactionData{})))
	brReply               = binderReturn(ior('r', 3, unsafe.Sizeof(binderTransactionData{})))
	brDeadReply           = binderReturn(ioc(0, 'r', 5, 0))
	brTransactionComplete = binderReturn(ioc(0, 'r', 6, 0))
	brIncRefs             = binderReturn(ior('r', 7, binderPtrCookieSize))
	brAcquire             = binderReturn(ior('r', 8, binderPtrCookieSize))
	brRelease             = binderReturn(ior('r', 9, binderPtrCookieSize))
	brDecrefs             = binderReturn(ior('r', 10, binderPtrCookieSize))
	brNoop                = binderReturn(ioc(0, 'r', 12, 0))
	brSpawnLooper         = binderReturn(ioc(0, 'r', 13, 0))
	brDeadBinder                 = binderReturn(ior('r', 15, unsafe.Sizeof(uintptr(0))))
	brClearDeathNotificationDone = binderReturn(ior('r', 16, unsafe.Sizeof(uintptr(0))))
	brFailedReply                = binderReturn(ioc(0, 'r', 17, 0))

	// brTransactionSecCtx is BR_TRANSACTION_SEC_CTX: same command number
	// as BR_TRANSACTION (2) but with the larger binder_transaction_data_secctx
	// struct. On newer kernels this may be sent instead of BR_TRANSACTION.
	// The extra field is a binder_uintptr_t security context pointer.
	brTransactionSecCtx = binderReturn(ior('r', 2, binderTransactionDataSecctxSize))
)
