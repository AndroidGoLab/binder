package gui

import (
	"context"
	"errors"
	"testing"

	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/parcel"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// captureLayersReplyTransport delivers a received parcel to the generated proxy.
//
// Example: exercise result ownership across the proxy's deferred Recycle.
type captureLayersReplyTransport struct {
	binder.VersionAwareTransport
	reply *parcel.Parcel
}

// ResolveCode selects the simulated captureLayersSync transaction.
//
// Example: invoke the generated synchronous proxy method.
func (*captureLayersReplyTransport) ResolveCode(
	_ context.Context,
	_ string,
	_ string,
) (binder.TransactionCode, error) {
	return 1, nil
}

// ResolveMethodSignature keeps the generated signature.
//
// Example: marshal the compiled LayerCaptureArgs.
func (*captureLayersReplyTransport) ResolveMethodSignature(
	_ context.Context,
	_ string,
	_ string,
) []string {
	return nil
}

// Transact returns the prepared kernel-owned reply.
//
// Example: decode a captured buffer and fence.
func (t *captureLayersReplyTransport) Transact(
	_ context.Context,
	_ uint32,
	_ binder.TransactionCode,
	_ binder.TransactionFlags,
	_ *parcel.Parcel,
) (*parcel.Parcel, error) {
	return t.reply, nil
}

// TestCaptureLayersSyncRetainsReceivedDescriptors verifies the generated consumer.
//
// Example: result descriptors survive reply recycling and close with the result.
func TestCaptureLayersSyncRetainsReceivedDescriptors(t *testing.T) {
	fds := make([]int, 2)
	require.NoError(t, unix.Pipe(fds))
	// Close any descriptor left open by an early assertion failure.
	//
	// Example: a failed decode still releases the test pipe.
	t.Cleanup(func() {
		for _, fd := range fds {
			if fd < 0 {
				continue
			}
			if err := unix.Close(fd); err != nil && !errors.Is(err, unix.EBADF) {
				t.Errorf("close test fd: %v", err)
			}
		}
	})
	p := parcel.New()
	binder.WriteStatus(p, nil)
	p.WriteInt32(1)
	p.WriteBool(true)
	writeGraphicBuffer(p, int32(fds[0]))
	p.WriteBool(true)
	p.WriteInt32(4)
	p.WriteInt32(1)
	p.WriteInt32(0)
	p.WriteFileDescriptor(int32(fds[1]))
	p.WriteBool(false)
	p.WriteBool(false)
	p.WriteUint32(0)
	tr := &captureLayersReplyTransport{reply: parcel.FromReceivedBytes(p.Data(), p.Objects())}
	proxy := NewSurfaceComposerProxy(binder.NewProxyBinder(tr, binder.CallerIdentity{}, 1))
	result, err := proxy.CaptureLayersSync(context.Background(), LayerCaptureArgs{})
	require.NoError(t, err)
	require.Equal(t, int32(fds[0]), result.Buffer.Handle.Fds[0])
	require.Equal(t, int32(fds[1]), result.FenceFD)
	for _, fd := range fds {
		_, err = unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
		require.NoError(t, err)
	}
	require.NoError(t, result.Close())
	for i, fd := range fds {
		_, err = unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
		require.ErrorIs(t, err, unix.EBADF)
		fds[i] = -1
	}
}
