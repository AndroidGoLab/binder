package binder

import (
	"errors"
	"fmt"

	"github.com/xaionaro-go/aidl/parcel"

	aidlerrors "github.com/xaionaro-go/aidl/errors"
)

// ReadStatus reads an AIDL Status from a reply parcel.
// Returns nil if the status indicates success (ExceptionNone).
// Returns a *aidlerrors.StatusError if an exception is present.
func ReadStatus(p *parcel.Parcel) error {
	code, err := p.ReadInt32()
	if err != nil {
		return fmt.Errorf("binder: reading status exception code: %w", err)
	}

	exception := aidlerrors.ExceptionCode(code)
	if exception == aidlerrors.ExceptionNone {
		return nil
	}

	// Read the exception message (String16).
	msg, err := p.ReadString16()
	if err != nil {
		return fmt.Errorf("binder: reading status message: %w", err)
	}

	// Read remote stack trace size (int32, we skip the actual trace).
	traceSize, err := p.ReadInt32()
	if err != nil {
		return fmt.Errorf("binder: reading status trace size: %w", err)
	}

	if traceSize > 0 {
		// Skip the remote stack trace string.
		_, _ = p.ReadString16()
	}

	statusErr := &aidlerrors.StatusError{
		Exception: exception,
		Message:   msg,
	}

	// For service-specific exceptions, read the error code.
	if exception == aidlerrors.ExceptionServiceSpecific {
		errCode, err := p.ReadInt32()
		if err != nil {
			return fmt.Errorf("binder: reading service-specific error code: %w", err)
		}

		statusErr.ServiceSpecificCode = errCode
	}

	return statusErr
}

// WriteStatus writes an AIDL Status to a parcel.
// If err is nil, writes ExceptionNone (success).
// If err is a *aidlerrors.StatusError, writes its contents.
// Otherwise, wraps the error as ExceptionTransactionFailed.
func WriteStatus(
	p *parcel.Parcel,
	err error,
) {
	if err == nil {
		p.WriteInt32(int32(aidlerrors.ExceptionNone))
		return
	}

	var statusErr *aidlerrors.StatusError
	if errors.As(err, &statusErr) {
		p.WriteInt32(int32(statusErr.Exception))
		p.WriteString16(statusErr.Message)
		p.WriteInt32(0) // no remote stack trace

		if statusErr.Exception == aidlerrors.ExceptionServiceSpecific {
			p.WriteInt32(statusErr.ServiceSpecificCode)
		}

		return
	}

	// For non-StatusError errors, wrap as a generic transaction failure.
	p.WriteInt32(int32(aidlerrors.ExceptionTransactionFailed))
	p.WriteString16(err.Error())
	p.WriteInt32(0) // no remote stack trace
}
