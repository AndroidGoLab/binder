//go:build linux

package kernelbinder

import "unsafe"

// binderCommand represents a binder command (BC_*) code written to the driver.
type binderCommand uint32

// binderReturn represents a binder return (BR_*) code read from the driver.
type binderReturn uint32

// transactionFlag represents a flag on a binder transaction (e.g. TF_STATUS_CODE).
type transactionFlag uint32

// binderObjectType represents a binder object type code (e.g. BINDER_TYPE_HANDLE).
type binderObjectType uint32

// binderTypeHandle is BINDER_TYPE_HANDLE: B_PACK_CHARS('s','h','*',0x85) = 0x73682a85.
// Used to identify flat_binder_object entries containing remote binder handles.
const binderTypeHandle = binderObjectType(0x73682a85)

// Binder command (BC) codes -- written to the driver.
// These are var (not const) because the values use unsafe.Sizeof, which is
// not a constant expression in Go.
var (
	bcTransaction       = binderCommand(iow('c', 0, unsafe.Sizeof(binderTransactionData{})))
	bcReply             = binderCommand(iow('c', 1, unsafe.Sizeof(binderTransactionData{})))
	bcFreeBuffer        = binderCommand(iow('c', 3, unsafe.Sizeof(uintptr(0)))) // binder_uintptr_t
	bcIncRefs           = binderCommand(iow('c', 4, unsafe.Sizeof(uint32(0))))
	bcAcquire           = binderCommand(iow('c', 5, unsafe.Sizeof(uint32(0))))
	bcRelease           = binderCommand(iow('c', 6, unsafe.Sizeof(uint32(0))))
	bcDecRefs           = binderCommand(iow('c', 7, unsafe.Sizeof(uint32(0))))
	bcIncRefsDone       = binderCommand(iow('c', 8, binderPtrCookieSize))
	bcAcquireDone       = binderCommand(iow('c', 9, binderPtrCookieSize))
	bcRegisterLooper    = binderCommand(ioc(0, 'c', 11, 0))
	bcEnterLooper       = binderCommand(ioc(0, 'c', 12, 0))
	bcExitLooper        = binderCommand(ioc(0, 'c', 13, 0))
	bcRequestDeathNotif = binderCommand(iow('c', 14, binderHandleCookieSize))
	bcClearDeathNotif   = binderCommand(iow('c', 15, binderHandleCookieSize))
	bcDeadBinderDone    = binderCommand(iow('c', 16, unsafe.Sizeof(uintptr(0))))
)

// binderPtrCookieSize is the size of the kernel's binder_ptr_cookie struct:
// binder_uintptr_t ptr + binder_uintptr_t cookie (2 x pointer-sized).
const binderPtrCookieSize = 2 * unsafe.Sizeof(uintptr(0)) // = 16 on 64-bit

// binderHandleCookieSize is the packed size of the kernel's binder_handle_cookie
// struct: __u32 handle + binder_uintptr_t cookie. The kernel struct uses
// __attribute__((packed)), so there is NO alignment padding between fields.
const binderHandleCookieSize = unsafe.Sizeof(uint32(0)) + unsafe.Sizeof(uintptr(0)) // = 12 on 64-bit

// tfStatusCode is TF_STATUS_CODE (0x08). When the kernel sets this flag on a
// BR_REPLY, the 4-byte data payload is a status_t error code, not a regular parcel.
const tfStatusCode = transactionFlag(0x08)

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
	brDeadBinder          = binderReturn(ior('r', 15, unsafe.Sizeof(uintptr(0))))
	brFailedReply         = binderReturn(ioc(0, 'r', 17, 0))
)
