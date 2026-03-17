//go:build linux

package kernelbinder

import "github.com/xaionaro-go/binder/binder"

// deathRecipientEntry holds a DeathRecipient together with the handle it
// monitors. The entry is heap-allocated and its address is used as the
// binder cookie for BC_REQUEST_DEATH_NOTIFICATION. Storing the entry in
// Driver.deathRecipients keeps it reachable by the GC, so the address
// remains valid when the kernel echoes the cookie back in BR_DEAD_BINDER.
type deathRecipientEntry struct {
	recipient binder.DeathRecipient
	handle    uint32
}
