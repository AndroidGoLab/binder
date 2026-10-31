//go:build linux

package kernelbinder

import "github.com/AndroidGoLab/binder/binder"

// receiverEntry holds a registered TransactionReceiver.
type receiverEntry struct {
	receiver binder.TransactionReceiver
}
