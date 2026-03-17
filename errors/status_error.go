package aidlerrors

import (
	"fmt"
)

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
