//go:build unix

package parcel

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// TestReadOwnedFileDescriptorDuplicatesBorrowed preserves the source descriptor.
//
// Example: a native object may close its owned copy without closing the writer's FD.
func TestReadOwnedFileDescriptorDuplicatesBorrowed(t *testing.T) {
	read, write, err := os.Pipe()
	require.NoError(t, err)
	defer closeTestFile(t, read)
	defer closeTestFile(t, write)
	p := New()
	p.WriteFileDescriptor(int32(read.Fd()))
	fd, err := p.ReadOwnedFileDescriptor()
	require.NoError(t, err)
	require.NotEqual(t, int32(read.Fd()), fd)
	flags, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
	require.NoError(t, err)
	require.NotZero(t, flags&unix.FD_CLOEXEC)
	require.NoError(t, unix.Close(int(fd)))
	_, err = read.Stat()
	require.NoError(t, err)
}

// TestReadOwnedFileDescriptorTransfersReceived avoids duplicating incoming descriptors.
//
// Example: the caller closes a received FD after the parcel is closed.
func TestReadOwnedFileDescriptorTransfersReceived(t *testing.T) {
	read, write, err := os.Pipe()
	require.NoError(t, err)
	readOpen := true
	defer // Close the source only if the test has not already closed it.
	//
	// Example: no deferred close can target a reused FD.
	func() {
		if readOpen {
			closeTestFile(t, read)
		}
	}()
	defer closeTestFile(t, write)
	wire := New()
	wire.WriteFileDescriptor(int32(read.Fd()))
	p := FromReceivedBytes(wire.Data(), wire.Objects())
	fd, err := p.ReadOwnedFileDescriptor()
	require.NoError(t, err)
	require.Equal(t, int32(read.Fd()), fd)
	require.NoError(t, p.Close())
	_, err = read.Stat()
	require.NoError(t, err)
	err = read.Close()
	readOpen = false
	require.NoError(t, err)
	_, err = unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
	require.ErrorIs(t, err, unix.EBADF)
}

// TestReadOwnedFileDescriptorRejectsClosedBorrowedFD reports duplication errors.
//
// Example: an invalid source is not returned as an owned descriptor.
func TestReadOwnedFileDescriptorRejectsClosedBorrowedFD(t *testing.T) {
	fds := []int{0, 0}
	require.NoError(t, unix.Pipe(fds))
	defer // Close the remaining pipe endpoint.
	//
	// Example: an unexpected cleanup failure fails the test.
	func() { require.NoError(t, unix.Close(fds[1])) }()
	require.NoError(t, unix.Close(fds[0]))
	p := New()
	p.WriteFileDescriptor(int32(fds[0]))
	_, err := p.ReadOwnedFileDescriptor()
	require.ErrorIs(t, err, unix.EBADF)
}
