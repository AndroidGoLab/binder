package parcel

import (
	"math"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestReceivedDescriptorOwnership preserves transferred descriptors and closes unread ones.
//
// Example: a callback reads its buffer FD but fails before reading its fence.
func TestReceivedDescriptorOwnership(t *testing.T) {
	read, write, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(
		// Release the test pipe after checking descriptor ownership.
		//
		// Example: either end may already have been closed by the test.
		func() { _ = read.Close(); _ = write.Close() },
	)
	wire := New()
	wire.WriteFileDescriptor(int32(read.Fd()))
	wire.WriteFileDescriptor(int32(write.Fd()))
	p := FromReceivedBytes(wire.Data(), wire.Objects())
	fd, err := p.ReadFileDescriptor()
	require.NoError(t, err)
	require.Equal(t, int32(read.Fd()), fd)
	require.NoError(t, p.Close())
	require.NoError(t, p.Close())
	_, err = read.Stat()
	require.NoError(t, err)
	_, err = write.Stat()
	require.Error(t, err)
	p.SetPosition(0)
	_, err = p.ReadFileDescriptor()
	require.Error(t, err)
}

// TestReceivedDescriptorRecycle releases descriptors without closing borrowed parcels.
//
// Example: recycle an undecoded callback while retaining a local request's FD.
func TestReceivedDescriptorRecycle(t *testing.T) {
	read, write, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(
		// Release the test pipe after checking descriptor ownership.
		//
		// Example: either end may already have been closed by the test.
		func() { _ = read.Close(); _ = write.Close() },
	)
	wire := New()
	wire.WriteFileDescriptor(int32(read.Fd()))
	borrowed := FromBytesWithObjects(wire.Data(), wire.Objects())
	require.NoError(t, borrowed.Close())
	_, err = read.Stat()
	require.NoError(t, err)
	p := FromReceivedBytes(wire.Data(), append(wire.Objects(), math.MaxUint64))
	require.NoError(t, p.Recycle())
	_, err = read.Stat()
	require.Error(t, err)
	_, err = write.Stat()
	require.NoError(t, err)
	require.Empty(t, p.Data())
}

// TestReceivedParcelFileDescriptorClosesComm releases the unreturned communication channel.
//
// Example: decode a ParcelFileDescriptor with hasComm set.
func TestReceivedParcelFileDescriptorClosesComm(t *testing.T) {
	read, write, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(
		// Release the test pipe after checking descriptor ownership.
		//
		// Example: either end may already have been closed by the test.
		func() { _ = read.Close(); _ = write.Close() },
	)
	wire := New()
	wire.WriteInt32(1)
	wire.WriteInt32(1)
	wire.WriteFileDescriptor(int32(read.Fd()))
	wire.WriteFileDescriptor(int32(write.Fd()))
	p := FromReceivedBytes(wire.Data(), wire.Objects())
	fd, err := p.ReadParcelFileDescriptor()
	require.NoError(t, err)
	require.Equal(t, int32(read.Fd()), fd)
	require.NoError(t, p.Close())
	_, err = read.Stat()
	require.NoError(t, err)
	_, err = write.Stat()
	require.Error(t, err)
}

// TestReceivedParcelFileDescriptorClosesPrimaryOnMalformedComm prevents ownership loss on error.
//
// Example: a ParcelFileDescriptor advertises a communication FD that is missing.
func TestReceivedParcelFileDescriptorClosesPrimaryOnMalformedComm(t *testing.T) {
	read, write, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(
		// Release the test pipe after checking descriptor ownership.
		//
		// Example: either end may already have been closed by the test.
		func() { _ = read.Close(); _ = write.Close() },
	)
	wire := New()
	wire.WriteInt32(1)
	wire.WriteInt32(1)
	wire.WriteFileDescriptor(int32(read.Fd()))
	p := FromReceivedBytes(wire.Data(), wire.Objects())
	_, err = p.ReadParcelFileDescriptor()
	require.Error(t, err)
	require.NoError(t, p.Close())
	_, err = read.Stat()
	require.Error(t, err)
	_, err = write.Stat()
	require.NoError(t, err)
}

// TestBorrowedParcelFileDescriptorPreservesComm retains the existing borrowed ownership contract.
//
// Example: parsing a locally constructed parcel does not close the writer's descriptors.
func TestBorrowedParcelFileDescriptorPreservesComm(t *testing.T) {
	read, write, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(
		// Release the test pipe after checking descriptor ownership.
		//
		// Example: either end may already have been closed by the test.
		func() { _ = read.Close(); _ = write.Close() },
	)
	wire := New()
	wire.WriteInt32(1)
	wire.WriteInt32(1)
	wire.WriteFileDescriptor(int32(read.Fd()))
	wire.WriteFileDescriptor(int32(write.Fd()))
	p := FromBytesWithObjects(wire.Data(), wire.Objects())
	fd, err := p.ReadParcelFileDescriptor()
	require.NoError(t, err)
	require.Equal(t, int32(read.Fd()), fd)
	require.NoError(t, p.Close())
	_, err = read.Stat()
	require.NoError(t, err)
	_, err = write.Stat()
	require.NoError(t, err)
}

// TestReceivedParcelRejectsUnlistedDescriptor checks received object ownership.
//
// Example: an FD-shaped object missing from Binder offsets is rejected.
func TestReceivedParcelRejectsUnlistedDescriptor(t *testing.T) {
	wire := New()
	wire.WriteFileDescriptor(123)
	p := FromReceivedBytes(wire.Data(), nil)
	_, err := p.ReadFileDescriptor()
	require.Error(t, err)
}
