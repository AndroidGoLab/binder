//go:build linux

package kernelbinder

import "unsafe"

// Binder command (BC) codes -- written to the driver.
// Codes that carry a struct payload encode sizeof(payload) in the ioctl-style
// number, so they must be computed from the actual Go struct sizes.
//
// Only currently-used codes are defined here. The full protocol is documented
// in the design doc and additional codes can be added as features are implemented.
var (
	bcTransaction       = uint32(iow('c', 0, unsafe.Sizeof(binderTransactionData{})))
	bcFreeBuffer        = uint32(iow('c', 3, unsafe.Sizeof(uintptr(0)))) // binder_uintptr_t
	bcIncRefs           = uint32(iow('c', 4, unsafe.Sizeof(uint32(0))))
	bcAcquire           = uint32(iow('c', 5, unsafe.Sizeof(uint32(0))))
	bcRelease           = uint32(iow('c', 6, unsafe.Sizeof(uint32(0))))
	bcRequestDeathNotif = uint32(iow('c', 14, binderHandleCookieSize))
	bcClearDeathNotif   = uint32(iow('c', 15, binderHandleCookieSize))
)

// binderHandleCookieSize is the packed size of the kernel's binder_handle_cookie
// struct: __u32 handle + binder_uintptr_t cookie. The kernel struct uses
// __attribute__((packed)), so there is NO alignment padding between fields.
const binderHandleCookieSize = unsafe.Sizeof(uint32(0)) + unsafe.Sizeof(uintptr(0)) // = 12 on 64-bit

// Binder return (BR) codes -- read from the driver.
var (
	brError               = uint32(ior('r', 0, unsafe.Sizeof(int32(0))))
	brReply               = uint32(ior('r', 3, unsafe.Sizeof(binderTransactionData{})))
	brDeadReply           = uint32(ioc(0, 'r', 5, 0))
	brTransactionComplete = uint32(ioc(0, 'r', 6, 0))
	brNoop                = uint32(ioc(0, 'r', 12, 0))
	brSpawnLooper         = uint32(ioc(0, 'r', 13, 0))
	brFailedReply         = uint32(ioc(0, 'r', 17, 0))
)
