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
	// InterfaceTransaction matches Android's INTERFACE_TRANSACTION =
	// B_PACK_CHARS('_','N','T','F') from IBinder.h.
	InterfaceTransaction TransactionCode = ('_' << 24) | ('N' << 16) | ('T' << 8) | 'F'
	SyspropsTransaction  TransactionCode = ('_' << 24) | ('S' << 16) | ('P' << 8) | 'R'
)
