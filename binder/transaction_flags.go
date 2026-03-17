package binder

// TransactionFlags control transaction behavior.
type TransactionFlags uint32

const (
	FlagOneway              TransactionFlags = 0x00000001
	FlagCollectNotedAppOps  TransactionFlags = 0x00000002
	// FlagAcceptFDs tells the binder kernel that this process can receive
	// file descriptors in the reply. Without this flag, the kernel rejects
	// replies containing FDs with BR_FAILED_REPLY. Android's
	// IPCThreadState::transact() always sets this flag.
	FlagAcceptFDs           TransactionFlags = 0x00000010
	FlagClearBuf            TransactionFlags = 0x00000020
	FlagPrivateVendor       TransactionFlags = 0x10000000
)
