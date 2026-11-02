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

// TransactionStatusError is a native status delivered in a TF_STATUS_CODE reply.
// The remote service may supply this status; it need not originate in the kernel.
//
// Example: recognize Android 10 service enumeration's terminal status with errors.As.
type TransactionStatusError struct {
	Code int32
}

// Error preserves the numeric status and the transport's diagnostic text.
//
// Example: report status -1 as binder: kernel status error: -1 (0xffffffff).
func (e *TransactionStatusError) Error() string {
	return fmt.Sprintf("binder: kernel status error: %d (0x%x)", e.Code, uint32(e.Code))
}

// Unwrap returns nil because the native status has no underlying Go error.
//
// Example: terminate errors.Unwrap traversal after inspecting the status code.
func (e *TransactionStatusError) Unwrap() error {
	return nil
}
