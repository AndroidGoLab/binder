package gralloc

import (
	"encoding/binary"
	"testing"

	"github.com/AndroidGoLab/binder/parcel"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// TestPixelDescriptor checks the supported gralloc4 descriptor geometry.
//
// Example: invalid dimensions never produce an allocation request.
func TestPixelDescriptor(t *testing.T) {
	desc, err := pixelDescriptor(360, 800)
	require.NoError(t, err)
	require.Len(t, desc, 37)
	require.Equal(t, uint32(40), binary.LittleEndian.Uint32(desc))
	require.Equal(t, uint32(360), binary.LittleEndian.Uint32(desc[4:]))
	require.Equal(t, uint32(800), binary.LittleEndian.Uint32(desc[8:]))
	_, err = pixelDescriptor(0, 800)
	require.Error(t, err)
}

// TestReadPixelAllocationOwnsBorrowedFD verifies independent descriptor ownership.
//
// Example: a decoded buffer remains valid after its caller closes the source FD.
func TestReadPixelAllocationOwnsBorrowedFD(t *testing.T) {
	var fds [2]int
	require.NoError(t, unix.Pipe(fds[:]))
	defer unix.Close(fds[0])
	defer unix.Close(fds[1])
	p := parcel.New()
	outer := parcel.WriteParcelableHeader(p)
	p.WriteInt32(2)
	p.WriteInt32(1)
	p.WriteInt32(1)
	inner := parcel.WriteParcelableHeader(p)
	p.WriteInt32(1)
	p.WriteParcelFileDescriptor(int32(fds[0]))
	p.WriteInt32(0)
	parcel.WriteParcelableFooter(p, inner)
	parcel.WriteParcelableFooter(p, outer)
	p.SetPosition(0)
	_, handles, err := readPixelAllocation(p, 1)
	require.NoError(t, err)
	require.Len(t, handles, 1)
	require.NotEqual(t, int32(fds[0]), handles[0].Fds[0])
	require.NoError(t, unix.Close(int(handles[0].Fds[0])))
}
