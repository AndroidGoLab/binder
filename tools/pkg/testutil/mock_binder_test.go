package testutil

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AndroidGoLab/binder/binder"
	aidlerrors "github.com/AndroidGoLab/binder/errors"
	"github.com/AndroidGoLab/binder/parcel"
)

func TestMockBinder_Transact_ReturnsSecurityException(t *testing.T) {
	mock := &MockBinder{}
	data := parcel.New()

	reply, err := mock.Transact(
		context.Background(),
		binder.FirstCallTransaction,
		0,
		data,
	)
	require.NoError(t, err)
	require.NotNil(t, reply)

	statusErr := binder.ReadStatus(reply)
	require.Error(t, statusErr)

	var se *aidlerrors.StatusError
	require.ErrorAs(t, statusErr, &se)
	assert.Equal(t, aidlerrors.ExceptionSecurity, se.Exception)
	assert.Equal(t, "mock: permission denied", se.Message)
}

func TestMockBinder_Handle(t *testing.T) {
	mock := &MockBinder{}
	assert.Equal(t, uint32(42), mock.Handle())
}

func TestMockBinder_IsAlive(t *testing.T) {
	mock := &MockBinder{}
	assert.True(t, mock.IsAlive(context.Background()))
}

func TestMockBinder_Transport(t *testing.T) {
	mock := &MockBinder{}
	assert.Nil(t, mock.Transport())
}

func TestMockBinder_LinkToDeath(t *testing.T) {
	mock := &MockBinder{}
	assert.NoError(t, mock.LinkToDeath(context.Background(), nil))
}

func TestMockBinder_UnlinkToDeath(t *testing.T) {
	mock := &MockBinder{}
	assert.NoError(t, mock.UnlinkToDeath(context.Background(), nil))
}
