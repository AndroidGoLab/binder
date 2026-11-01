package gralloc

import (
	"encoding/binary"
	"errors"
	"testing"

	"github.com/AndroidGoLab/binder/parcel"
	"golang.org/x/sys/unix"
)

// graphicParcel creates a flattened GraphicBuffer header for decoding tests.
//
// Example: create a handle with one descriptor and no integer payload.
func graphicParcel(
	fds int32,
	ints int32,
	data []byte,
) *parcel.Parcel {
	raw := make([]byte, 52+len(data))
	binary.LittleEndian.PutUint32(raw, graphicBufferMagic)
	binary.LittleEndian.PutUint32(raw[40:], uint32(fds))
	binary.LittleEndian.PutUint32(raw[44:], uint32(ints))
	copy(raw[52:], data)
	p := parcel.New()
	p.WriteInt32(int32(len(raw)))
	p.WriteInt32(fds)
	p.WriteRawBytes(raw)
	return p
}

// TestReadGraphicBufferReportsCloseFailure retains errors from partial cleanup.
//
// Example: a repeated borrowed FD causes a second close to report EBADF.
func TestReadGraphicBufferReportsCloseFailure(t *testing.T) {
	fds := make([]int, 2)
	if err := unix.Pipe(fds); err != nil {
		t.Fatal(err)
	}
	defer unix.Close(fds[1])
	p := graphicParcel(3, 0, nil)
	p.WriteFileDescriptor(int32(fds[0]))
	p.WriteFileDescriptor(int32(fds[0]))
	_, err := ReadGraphicBuffer(p)
	if !errors.Is(err, unix.EBADF) {
		t.Fatalf("cleanup error not preserved: %v", err)
	}
}

// TestReadGraphicBufferRejectsTruncatedInts checks handle length validation.
//
// Example: reject a header that declares more words than are present.
func TestReadGraphicBufferRejectsTruncatedInts(t *testing.T) {
	p := graphicParcel(0, 2, make([]byte, 4))
	p.SetPosition(0)
	if _, err := ReadGraphicBuffer(p); err == nil {
		t.Fatal("accepted truncated native handle")
	}
}

// TestReadGraphicBufferClosesPartialFDs checks cleanup after a truncated handle.
//
// Example: a missing second FD closes the first FD already read.
func TestReadGraphicBufferClosesPartialFDs(t *testing.T) {
	fds := make([]int, 2)
	if err := unix.Pipe(fds); err != nil {
		t.Fatal(err)
	}
	defer unix.Close(fds[1])
	p := graphicParcel(2, 0, nil)
	p.WriteFileDescriptor(int32(fds[0]))
	p.SetPosition(0)
	if _, err := ReadGraphicBuffer(p); err == nil {
		t.Fatal("accepted missing second fd")
	}
	if _, err := unix.FcntlInt(uintptr(fds[0]), unix.F_GETFD, 0); err != unix.EBADF {
		unix.Close(fds[0])
		t.Fatalf("partial fd remains open: %v", err)
	}
}
