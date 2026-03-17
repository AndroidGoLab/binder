package aidlerrors

import (
	"fmt"
)

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
