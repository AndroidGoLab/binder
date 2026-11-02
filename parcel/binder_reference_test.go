package parcel

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBinderReferenceWire(t *testing.T) {
	for _, tc := range []struct {
		name          string
		api           int
		ref           BinderReference
		typ, flags    uint32
		value, cookie uint64
		objects       []uint64
		stability     int32
	}{
		{"legacy null", 29, BinderReference{}, 0x73622a85, 0x113, 0, 0, nil, 0},
		{"legacy remote zero", 29, BinderReference{Kind: BinderReferenceRemote}, 0x73682a85, 0x113, 0, 0, []uint64{0}, 0},
		{"legacy local", 29, BinderReference{Kind: BinderReferenceLocal, Pointer: 0x100000001, Cookie: 0x200000002}, 0x73622a85, 0x113, 0x100000001, 0x200000002, []uint64{0}, 0},
		{"modern null", 34, BinderReference{}, 0x73622a85, 0, 0, 0, nil, 0},
		{"modern remote", 34, BinderReference{Kind: BinderReferenceRemote, Handle: 7}, 0x73682a85, 0x13, 7, 0, []uint64{0}, 12},
		{"modern local", 34, BinderReference{Kind: BinderReferenceLocal, Pointer: 0x100000001, Cookie: 0x200000002}, 0x73622a85, 0x113, 0x100000001, 0x200000002, []uint64{0}, 12},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := New()
			p.SetTargetAPILevel(tc.api)
			require.NoError(t, p.WriteBinderReference(tc.ref))
			p.WriteInt32(0x12345678)
			b := p.Data()
			require.Equal(t, tc.typ, binary.LittleEndian.Uint32(b))
			require.Equal(t, tc.flags, binary.LittleEndian.Uint32(b[4:]))
			require.Equal(t, tc.value, binary.LittleEndian.Uint64(b[8:]))
			require.Equal(t, tc.cookie, binary.LittleEndian.Uint64(b[16:]))
			require.Equal(t, tc.objects, p.Objects())
			size := 24
			if tc.api == 34 {
				size += 4
				require.Equal(t, tc.stability, int32(binary.LittleEndian.Uint32(b[24:])))
			}
			require.Len(t, b, size+4)
			p.SetPosition(0)
			got, err := p.ReadBinderReference()
			require.NoError(t, err)
			require.Equal(t, tc.ref, got)
			sentinel, err := p.ReadInt32()
			require.NoError(t, err)
			require.Equal(t, int32(0x12345678), sentinel)
		})
	}
}

func TestBinderReferenceRejectsInvalid(t *testing.T) {
	p := New()
	p.SetTargetAPILevel(29)
	require.Error(t, p.WriteBinderReference(BinderReference{Kind: BinderReferenceLocal, Pointer: 1}))
	require.Empty(t, p.Data())
	require.Error(t, p.WriteBinderReference(BinderReference{Kind: 99}))
	_, err := p.ReadBinderReference()
	require.Error(t, err)
	p = FromBytes(make([]byte, 24))
	p.SetTargetAPILevel(34)
	_, err = p.ReadBinderReference()
	require.Error(t, err)
	p = New()
	require.Error(t, p.WriteBinderReference(BinderReference{}))
}
