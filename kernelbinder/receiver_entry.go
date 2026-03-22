//go:build linux

package kernelbinder

import "github.com/AndroidGoLab/binder/binder"

// receiverEntry holds a registered TransactionReceiver together with the
// heap-allocated anchor whose address is used as the binder cookie.
// Storing the entry as a map value (*receiverEntry) keeps it reachable by the
// GC, so its address remains valid for the kernel binder driver.
type receiverEntry struct {
	receiver binder.TransactionReceiver
}
