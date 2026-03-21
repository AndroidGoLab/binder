//go:build linux

package kernelbinder

// transactionFlag represents a flag on a binder transaction (e.g. TF_STATUS_CODE).
type transactionFlag uint32

// tfStatusCode is TF_STATUS_CODE (0x08). When the kernel sets this flag on a
// BR_REPLY, the 4-byte data payload is a status_t error code, not a regular parcel.
const tfStatusCode = transactionFlag(0x08)
