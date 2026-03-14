package binder

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	aidlerrors "github.com/xaionaro-go/aidl/errors"
	"github.com/xaionaro-go/aidl/parcel"
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
