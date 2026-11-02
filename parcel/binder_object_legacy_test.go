package parcel

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

// legacyBinderWriter describes the Android 10 Binder encoding operations.
//
// Example: write a legacy local Binder into a transaction.
type legacyBinderWriter interface {
	WriteLocalBinderLegacy(uintptr, uintptr)
	WriteStrongBinderLegacy(uint32)
	WriteNullStrongBinderLegacy()
	WriteNullWeakBinderLegacy()
}

// TestLegacyBinderObjects checks Android 10 objects and following data boundaries.
//
// Example: preserve the next field after an object without a stability suffix.
func TestLegacyBinderObjects(t *testing.T) {
	p := New()
	w, ok := any(p).(legacyBinderWriter)
	require.True(t, ok, "Parcel must support Android 10 Binder objects")
	w.WriteLocalBinderLegacy(0x1234, 0x5678)
	w.WriteStrongBinderLegacy(42)
	w.WriteNullStrongBinderLegacy()
	w.WriteNullWeakBinderLegacy()
	p.WriteInt32(99)
	require.Equal(t, []uint64{0, 24}, p.Objects())
	require.Len(t, p.Data(), 100)
	for _, tc := range []struct {
		offset        int
		kind, flags   uint32
		value, cookie uint64
	}{
		{0, 0x73622a85, 0x113, 0x1234, 0x5678},
		{24, 0x73682a85, 0x113, 42, 0},
		{48, 0x73622a85, 0x113, 0, 0},
		{72, 0x73622a85, 0x17f, 0, 0},
	} {
		b := p.Data()[tc.offset:]
		require.Equal(t, tc.kind, binary.LittleEndian.Uint32(b))
		require.Equal(t, tc.flags, binary.LittleEndian.Uint32(b[4:]))
		require.Equal(t, tc.value, binary.LittleEndian.Uint64(b[8:]))
		require.Equal(t, tc.cookie, binary.LittleEndian.Uint64(b[16:]))
	}
	p.SetPosition(24)
	handle, valid, err := p.ReadNullableStrongBinderLegacy()
	require.NoError(t, err)
	require.True(t, valid)
	require.Equal(t, uint32(42), handle)
	for range 2 {
		_, valid, err = p.ReadNullableStrongBinderLegacy()
		require.NoError(t, err)
		require.False(t, valid)
	}
	next, err := p.ReadInt32()
	require.NoError(t, err)
	require.Equal(t, int32(99), next)
}
