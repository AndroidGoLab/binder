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

// TransactionError represents a Binder transaction failure.
type TransactionError struct {
	Code TransactionErrorCode
}

func (e *TransactionError) Error() string {
	switch e.Code {
	case TransactionErrorDeadObject:
		return "binder: dead object"
	case TransactionErrorFailedTransaction:
		return "binder: failed transaction"
	case TransactionErrorFDSNotAllowed:
		return "binder: file descriptors not allowed"
	case TransactionErrorUnexpectedNull:
		return "binder: unexpected null"
	default:
		return fmt.Sprintf("binder: transaction error %d", e.Code)
	}
}

func (e *TransactionError) Unwrap() error {
	return nil
}
