package aidlerrors

import (
	"fmt"
)

// TransactionErrorCode represents a Binder transaction failure code.
type TransactionErrorCode int32

const (
	TransactionErrorDeadObject        TransactionErrorCode = -32
	TransactionErrorFailedTransaction TransactionErrorCode = -33
	TransactionErrorFDSNotAllowed     TransactionErrorCode = -34
	TransactionErrorUnexpectedNull    TransactionErrorCode = -35
)

// String returns a human-readable name for the transaction error code.
func (c TransactionErrorCode) String() string {
	switch c {
	case TransactionErrorDeadObject:
		return "DeadObject"
	case TransactionErrorFailedTransaction:
		return "FailedTransaction"
	case TransactionErrorFDSNotAllowed:
		return "FDSNotAllowed"
	case TransactionErrorUnexpectedNull:
		return "UnexpectedNull"
	default:
		return fmt.Sprintf("TransactionErrorCode(%d)", int32(c))
	}
}
