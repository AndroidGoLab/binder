package binder

// TransactionCode identifies a method in a Binder interface.
// User-defined codes start at FirstCallTransaction.
type TransactionCode uint32

const (
	FirstCallTransaction TransactionCode = 0x00000001
	LastCallTransaction  TransactionCode = 0x00ffffff

	PingTransaction      TransactionCode = ('_' << 24) | ('P' << 16) | ('N' << 8) | 'G'
	DumpTransaction      TransactionCode = ('_' << 24) | ('D' << 16) | ('M' << 8) | 'P'
	ShellTransaction     TransactionCode = ('_' << 24) | ('C' << 16) | ('M' << 8) | 'D'
	InterfaceTransaction TransactionCode = ('_' << 24) | ('I' << 16) | ('N' << 8) | 'T'
	SyspropsTransaction  TransactionCode = ('_' << 24) | ('S' << 16) | ('P' << 8) | 'R'
)

// TransactionFlags control transaction behavior.
type TransactionFlags uint32

const (
	FlagOneway              TransactionFlags = 0x00000001
	FlagCollectNotedAppOps  TransactionFlags = 0x00000002
	FlagClearBuf            TransactionFlags = 0x00000020
	FlagPrivateVendor       TransactionFlags = 0x10000000
)
