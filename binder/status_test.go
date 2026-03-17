package binder

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	aidlerrors "github.com/xaionaro-go/binder/errors"
	"github.com/xaionaro-go/binder/parcel"
)

func TestReadStatus_ExceptionNone(t *testing.T) {
	p := parcel.New()
	p.WriteInt32(int32(aidlerrors.ExceptionNone))
	p.SetPosition(0)

	err := ReadStatus(p)
	assert.NoError(t, err)
}

func TestWriteStatusNil_ReadStatus(t *testing.T) {
	p := parcel.New()
	WriteStatus(p, nil)
	p.SetPosition(0)

	err := ReadStatus(p)
	assert.NoError(t, err)
}

func TestWriteStatusError_ReadStatus(t *testing.T) {
	original := &aidlerrors.StatusError{
		Exception: aidlerrors.ExceptionIllegalArgument,
		Message:   "bad argument",
	}

	p := parcel.New()
	WriteStatus(p, original)
	p.SetPosition(0)

	err := ReadStatus(p)
	require.Error(t, err)

	var statusErr *aidlerrors.StatusError
	require.ErrorAs(t, err, &statusErr)
	assert.Equal(t, aidlerrors.ExceptionIllegalArgument, statusErr.Exception)
	assert.Equal(t, "bad argument", statusErr.Message)
}

func TestWriteStatusServiceSpecific_ReadStatus(t *testing.T) {
	original := &aidlerrors.StatusError{
		Exception:           aidlerrors.ExceptionServiceSpecific,
		Message:             "service error",
		ServiceSpecificCode: 42,
	}

	p := parcel.New()
	WriteStatus(p, original)
	p.SetPosition(0)

	err := ReadStatus(p)
	require.Error(t, err)

	var statusErr *aidlerrors.StatusError
	require.ErrorAs(t, err, &statusErr)
	assert.Equal(t, aidlerrors.ExceptionServiceSpecific, statusErr.Exception)
	assert.Equal(t, "service error", statusErr.Message)
	assert.Equal(t, int32(42), statusErr.ServiceSpecificCode)
}

func TestWriteStatusGenericError_ReadStatus(t *testing.T) {
	original := fmt.Errorf("something went wrong")

	p := parcel.New()
	WriteStatus(p, original)
	p.SetPosition(0)

	err := ReadStatus(p)
	require.Error(t, err)

	var statusErr *aidlerrors.StatusError
	require.ErrorAs(t, err, &statusErr)
	assert.Equal(t, aidlerrors.ExceptionTransactionFailed, statusErr.Exception)
	assert.Equal(t, "something went wrong", statusErr.Message)
}

func TestReadStatus_TruncatedTraceString(t *testing.T) {
	// Build a parcel that claims traceSize > 0 but contains no trace
	// string data. Before the fix, ReadString16 error was silently
	// discarded, corrupting the read position so that the subsequent
	// ReadInt32 for ServiceSpecificCode would read garbage instead of
	// returning an error.
	p := parcel.New()
	p.WriteInt32(int32(aidlerrors.ExceptionServiceSpecific)) // exception code
	p.WriteString16("service error")                         // message
	p.WriteInt32(1)                                          // traceSize > 0 (claims a trace exists)
	// Deliberately omit the trace string data — parcel is truncated here.

	p.SetPosition(0)

	err := ReadStatus(p)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading status trace string")

	// Must NOT be a StatusError with garbage ServiceSpecificCode.
	var statusErr *aidlerrors.StatusError
	assert.False(t, errors.As(err, &statusErr),
		"truncated parcel must not produce a StatusError with garbage fields")
}
