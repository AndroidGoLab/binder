//go:build linux

package kernelbinder

import (
	"context"
	"encoding/binary"
	"fmt"
	"testing"
	"unsafe"

	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/parcel"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// TestIncomingTransactionClosesUnhandledDescriptors exercises a late callback after unregister.
//
// Example: a timed-out screenshot arrives carrying a native buffer FD.
func TestIncomingTransactionClosesUnhandledDescriptors(t *testing.T) {
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, unix.Close(fds[1])) })

	p := parcel.New()
	p.WriteFileDescriptor(int32(fds[0]))
	mapped := make([]byte, len(p.Data())+8)
	copy(mapped, p.Data())
	binary.LittleEndian.PutUint64(mapped[len(p.Data()):], p.Objects()[0])
	d := &Driver{fd: -1, mapped: mapped}
	txn := binderTransactionData{
		code: uint32(binder.FirstCallTransaction), flags: uint32(binder.FlagOneway),
		dataSize: uint64(len(p.Data())), offsetsSize: 8,
		dataBuffer:    uint64(uintptr(unsafe.Pointer(&mapped[0]))),
		offsetsBuffer: uint64(uintptr(unsafe.Pointer(&mapped[len(p.Data())]))),
	}
	d.handleIncomingTransaction(context.Background(), &txn)
	_, err = unix.FcntlInt(uintptr(fds[0]), unix.F_GETFD, 0)
	if err == nil {
		require.NoError(t, unix.Close(fds[0]))
	}
	require.ErrorIs(t, err, unix.EBADF)
	_, err = unix.FcntlInt(uintptr(fds[1]), unix.F_GETFD, 0)
	require.NoError(t, err)
}

// descriptorReceiver transfers its first FD and fails to decode the remainder.
//
// Example: a partially decoded callback owns its buffer but leaves the fence unread.
type descriptorReceiver struct {
	FD int32
}

// Descriptor identifies the test callback interface.
//
// Example: answer an interface transaction during descriptor ownership checks.
func (r *descriptorReceiver) Descriptor() string {
	return "test.descriptors"
}

// OnTransaction transfers the first descriptor and returns a decoding error.
//
// Example: fail after decoding a native buffer FD but before its fence.
func (r *descriptorReceiver) OnTransaction(
	_ context.Context,
	_ binder.TransactionCode,
	p *parcel.Parcel,
) (*parcel.Parcel, error) {
	fd, err := p.ReadFileDescriptor()
	if err != nil {
		return nil, err
	}
	r.FD = fd
	return nil, fmt.Errorf("malformed callback remainder")
}

// TestIncomingTransactionPreservesTransferredDescriptors checks cleanup after a callback error.
//
// Example: a malformed callback transfers one FD and leaves another unread.
func TestIncomingTransactionPreservesTransferredDescriptors(t *testing.T) {
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, unix.Close(fds[0])) })
	p := parcel.New()
	p.WriteFileDescriptor(int32(fds[0]))
	p.WriteFileDescriptor(int32(fds[1]))
	mapped := make([]byte, len(p.Data())+16)
	copy(mapped, p.Data())
	for i, offset := range p.Objects() {
		binary.LittleEndian.PutUint64(mapped[len(p.Data())+i*8:], offset)
	}
	receiver := &descriptorReceiver{FD: -1}
	d := &Driver{fd: -1, mapped: mapped, receivers: map[uintptr]*receiverEntry{1: {receiver: receiver}}}
	txn := binderTransactionData{
		cookie: 1, code: uint32(binder.FirstCallTransaction), flags: uint32(binder.FlagOneway),
		dataSize: uint64(len(p.Data())), offsetsSize: 16,
		dataBuffer:    uint64(uintptr(unsafe.Pointer(&mapped[0]))),
		offsetsBuffer: uint64(uintptr(unsafe.Pointer(&mapped[len(p.Data())]))),
	}
	d.handleIncomingTransaction(context.Background(), &txn)
	require.Equal(t, int32(fds[0]), receiver.FD)
	_, err = unix.FcntlInt(uintptr(fds[0]), unix.F_GETFD, 0)
	require.NoError(t, err)
	_, err = unix.FcntlInt(uintptr(fds[1]), unix.F_GETFD, 0)
	if err == nil {
		require.NoError(t, unix.Close(fds[1]))
	}
	require.ErrorIs(t, err, unix.EBADF)
}
