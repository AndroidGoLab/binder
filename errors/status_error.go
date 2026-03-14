package aidlerrors

import (
	"fmt"
)

// ExceptionCode represents an AIDL exception code sent in a reply parcel.
type ExceptionCode int32

const (
	ExceptionNone                 ExceptionCode = 0
	ExceptionSecurity             ExceptionCode = -1
	ExceptionBadParcelable        ExceptionCode = -2
	ExceptionIllegalArgument      ExceptionCode = -3
	ExceptionNullPointer          ExceptionCode = -4
	ExceptionIllegalState         ExceptionCode = -5
	ExceptionNetworkMain          ExceptionCode = -6
	ExceptionUnsupportedOperation ExceptionCode = -7
	ExceptionServiceSpecific      ExceptionCode = -8
	ExceptionParcelable           ExceptionCode = -9
	ExceptionTransactionFailed    ExceptionCode = -129
)

// String returns a human-readable name for the exception code.
func (c ExceptionCode) String() string {
	switch c {
	case ExceptionNone:
		return "None"
	case ExceptionSecurity:
		return "Security"
	case ExceptionBadParcelable:
		return "BadParcelable"
	case ExceptionIllegalArgument:
		return "IllegalArgument"
	case ExceptionNullPointer:
		return "NullPointer"
	case ExceptionIllegalState:
		return "IllegalState"
	case ExceptionNetworkMain:
		return "NetworkMain"
	case ExceptionUnsupportedOperation:
		return "UnsupportedOperation"
	case ExceptionServiceSpecific:
		return "ServiceSpecific"
	case ExceptionParcelable:
		return "Parcelable"
	case ExceptionTransactionFailed:
		return "TransactionFailed"
	default:
		return fmt.Sprintf("ExceptionCode(%d)", int32(c))
	}
}

// StatusError represents an AIDL-level exception returned in a Status block.
type StatusError struct {
	Exception ExceptionCode
	Message   string
	// ServiceSpecificCode is set when Exception == ExceptionServiceSpecific.
	ServiceSpecificCode int32
}

func (e *StatusError) Error() string {
	msg := "aidl: exception " + e.exceptionName()

	if e.Message != "" {
		msg += ": " + e.Message
	}

	if e.Exception == ExceptionServiceSpecific {
		msg += fmt.Sprintf(" (code %d)", e.ServiceSpecificCode)
	}

	return msg
}

func (e *StatusError) Unwrap() error {
	return nil
}

func (e *StatusError) exceptionName() string {
	return e.Exception.String()
}
