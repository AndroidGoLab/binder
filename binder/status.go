package binder

import (
	"errors"
	"fmt"

	"github.com/xaionaro-go/binder/parcel"

	aidlerrors "github.com/xaionaro-go/binder/errors"
)

// skipFatReplyHeader reads the header size from the current position
// and advances past the header data. The headerSize field is measured
// from its own start, so we record the position before reading headerSize,
// then seek to startPos + headerSize.
func skipFatReplyHeader(p *parcel.Parcel) error {
	startPos := p.Position()
	headerSize, err := p.ReadInt32()
	if err != nil {
		return fmt.Errorf("binder: reading fat reply header size: %w", err)
	}
	if headerSize < 4 {
		return fmt.Errorf("binder: invalid fat reply header size: %d", headerSize)
	}
	if startPos+int(headerSize) > p.Len() {
		return fmt.Errorf("binder: fat reply header size %d exceeds data length %d", headerSize, p.Len()-startPos)
	}
	p.SetPosition(startPos + int(headerSize))
	return nil
}

// ReadStatus reads an AIDL Status from a reply parcel.
// Returns nil if the status indicates success (ExceptionNone).
// Returns a *aidlerrors.StatusError if an exception is present.
//
// Android Java services may prepend reply parcels with "fat reply headers":
//   - EX_HAS_NOTED_APPOPS_REPLY_HEADER (-127): AppOps header, skip then read
//     the real exception code.
//   - EX_HAS_REPLY_HEADER (-128): StrictMode header, skip then treat as success.
func ReadStatus(p *parcel.Parcel) error {
	code, err := p.ReadInt32()
	if err != nil {
		return fmt.Errorf("binder: reading status exception code: %w", err)
	}

	exception := aidlerrors.ExceptionCode(code)

	// Handle fat reply headers using sequential if statements (matching AOSP).
	// After an AppOps header, the next exception code may itself be a
	// StrictMode reply header, so both must be checked in sequence.
	if exception == aidlerrors.ExHasNotedAppOpsHeader { // -127
		if err := skipFatReplyHeader(p); err != nil {
			return err
		}
		// Read the real exception code after the header.
		code, err = p.ReadInt32()
		if err != nil {
			return fmt.Errorf("binder: reading status exception code after AppOps header: %w", err)
		}
		exception = aidlerrors.ExceptionCode(code)
	}

	if exception == aidlerrors.ExHasReplyHeader { // -128
		if err := skipFatReplyHeader(p); err != nil {
			return err
		}
		// Read the real exception code after the StrictMode header.
		code, err = p.ReadInt32()
		if err != nil {
			return fmt.Errorf("binder: reading status exception code after reply header: %w", err)
		}
		exception = aidlerrors.ExceptionCode(code)
	}

	if exception == aidlerrors.ExceptionNone {
		return nil
	}

	// Read the exception message (String16).
	msg, err := p.ReadString16()
	if err != nil {
		return fmt.Errorf("binder: reading status message: %w", err)
	}

	// Skip the remote stack trace. AOSP uses a self-describing header:
	// int32(headerSize) where headerSize includes the int32 itself.
	// We read the size and skip headerSize-4 additional bytes.
	traceSize, err := p.ReadInt32()
	if err != nil {
		return fmt.Errorf("binder: reading status trace size: %w", err)
	}

	if traceSize >= 4 {
		skip := int(traceSize) - 4
		if skip > 0 && p.Position()+skip <= p.Len() {
			p.SetPosition(p.Position() + skip)
		}
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

	// For parcelable exceptions, skip the blob so the parcel position
	// is left at the right place for subsequent reads.
	// blobSize is a self-describing header: the int32 itself is included
	// in the size, so we skip blobSize-4 additional bytes.
	if exception == aidlerrors.ExceptionParcelable {
		blobSize, err := p.ReadInt32()
		if err != nil {
			return fmt.Errorf("binder: reading parcelable exception blob size: %w", err)
		}
		if blobSize > 4 {
			p.SetPosition(p.Position() + int(blobSize) - 4)
		}
	}

	return statusErr
}

// WriteStatus writes an AIDL Status to a parcel.
// If err is nil, writes ExceptionNone (success).
// If err is a *aidlerrors.StatusError, writes its contents.
// Otherwise, wraps the error as ExceptionIllegalState.
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

		// ExceptionParcelable requires a blob size field after the
		// trace header, even when there is no embedded parcelable.
		// The size is self-describing: it includes the 4-byte size
		// field itself, so an empty blob has size 4.
		if statusErr.Exception == aidlerrors.ExceptionParcelable {
			p.WriteInt32(4) // self-describing header: 4 bytes for the size field itself
		}

		return
	}

	// For non-StatusError errors, wrap as IllegalState. This is more
	// appropriate than TransactionFailed (-129) for application errors
	// because TransactionFailed indicates a transport-level failure,
	// while IllegalState (-5) signals an application-level error condition.
	p.WriteInt32(int32(aidlerrors.ExceptionIllegalState))
	p.WriteString16(err.Error())
	p.WriteInt32(0) // no remote stack trace
}
