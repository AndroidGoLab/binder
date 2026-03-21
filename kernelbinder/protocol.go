//go:build linux

package kernelbinder

import "unsafe"

// binderPtrCookieSize is the size of the kernel's binder_ptr_cookie struct:
// binder_uintptr_t ptr + binder_uintptr_t cookie (2 x pointer-sized).
const binderPtrCookieSize = 2 * unsafe.Sizeof(uintptr(0)) // = 16 on 64-bit

// binderHandleCookieSize is the packed size of the kernel's binder_handle_cookie
// struct: __u32 handle + binder_uintptr_t cookie. The kernel struct uses
// __attribute__((packed)), so there is NO alignment padding between fields.
const binderHandleCookieSize = unsafe.Sizeof(uint32(0)) + unsafe.Sizeof(uintptr(0)) // = 12 on 64-bit

// binderTransactionDataSecctxSize is the size of the kernel's
// binder_transaction_data_secctx struct: binder_transaction_data + binder_uintptr_t.
const binderTransactionDataSecctxSize = unsafe.Sizeof(binderTransactionData{}) + unsafe.Sizeof(uintptr(0))
