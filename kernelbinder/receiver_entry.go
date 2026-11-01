//go:build linux

package kernelbinder

import "github.com/AndroidGoLab/binder/binder"

// receiverEntry holds a registered TransactionReceiver.
//
// Example: route an incoming callback by its cookie.
type receiverEntry struct {
	receiver binder.TransactionReceiver
}
