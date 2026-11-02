package view

import (
	"encoding/binary"
	"testing"

	"github.com/AndroidGoLab/binder/parcel"
	"github.com/stretchr/testify/require"
)

func TestSurfaceWire(t *testing.T) {
	for _, api := range []int{29, 34} {
		t.Run(string(rune('0'+api/10))+string(rune('0'+api%10)), func(t *testing.T) {
			p := parcel.New()
			p.SetTargetAPILevel(api)
			s := Surface{Name: "A", IsSingleBuffered: true, Producer: parcel.BinderReference{Kind: parcel.BinderReferenceRemote, Handle: 42}}
			require.NoError(t, s.MarshalParcel(p))
			p.WriteInt32(0x12345678)
			want := []byte{1, 0, 0, 0, 'A', 0, 0, 0, 1, 0, 0, 0, 0x65, 0x75, 0x71, 0x62, 0x85, 0x2a, 0x68, 0x73}
			flags := uint32(0x113)
			if api == 34 {
				flags = 0x13
			}
			want = binary.LittleEndian.AppendUint32(want, flags)
			want = binary.LittleEndian.AppendUint64(want, 42)
			want = binary.LittleEndian.AppendUint64(want, 0)
			if api == 34 {
				want = binary.LittleEndian.AppendUint32(want, 12)
				want = binary.LittleEndian.AppendUint32(want, 0x73622a85)
				want = append(want, make([]byte, 24)...)
			}
			want = binary.LittleEndian.AppendUint32(want, 0x12345678)
			require.Equal(t, want, p.Data())
			require.Equal(t, []uint64{16}, p.Objects())
			p.SetPosition(0)
			var decoded Surface
			require.NoError(t, decoded.UnmarshalParcel(p))
			require.Equal(t, s, decoded)
			v, err := p.ReadInt32()
			require.NoError(t, err)
			require.Equal(t, int32(0x12345678), v)
		})
	}
}

func TestSurfaceErrors(t *testing.T) {
	p := parcel.New()
	require.Error(t, (&Surface{}).MarshalParcel(p))
	require.Empty(t, p.Data())
	p.SetTargetAPILevel(29)
	require.Error(t, (&Surface{Control: parcel.BinderReference{Kind: parcel.BinderReferenceRemote, Handle: 1}}).MarshalParcel(p))
	require.Empty(t, p.Data())
	p.WriteString16("")
	p.WriteBool(false)
	p.WriteUint32(7)
	p.SetPosition(0)
	require.Error(t, (&Surface{}).UnmarshalParcel(p))
	p = parcel.New()
	p.SetTargetAPILevel(29)
	p.WriteString16("")
	p.WriteBool(false)
	p.WriteUint32(0x62717565)
	p.SetPosition(0)
	require.Error(t, (&Surface{}).UnmarshalParcel(p))
}

func TestSurfaceLocalAndControlReferences(t *testing.T) {
	for _, api := range []int{29, 34} {
		p := parcel.New()
		p.SetTargetAPILevel(api)
		s := Surface{Producer: parcel.BinderReference{Kind: parcel.BinderReferenceLocal, Pointer: 0x100000001, Cookie: 0x200000002}}
		if api == 34 {
			s.Control = parcel.BinderReference{Kind: parcel.BinderReferenceRemote, Handle: 17}
		}
		require.NoError(t, s.MarshalParcel(p))
		if api == 29 {
			require.Equal(t, []uint64{16}, p.Objects())
			require.Len(t, p.Data(), 40)
		} else {
			require.Equal(t, []uint64{16, 44}, p.Objects())
			require.Len(t, p.Data(), 72)
		}
		require.Equal(t, uint64(0x100000001), binary.LittleEndian.Uint64(p.Data()[24:]))
		require.Equal(t, uint64(0x200000002), binary.LittleEndian.Uint64(p.Data()[32:]))
		p.SetPosition(0)
		var got Surface
		require.NoError(t, got.UnmarshalParcel(p))
		require.Equal(t, s, got)
		for end := 0; end < len(p.Data()); end++ {
			q := parcel.FromBytes(p.Data()[:end])
			q.SetTargetAPILevel(api)
			var truncated Surface
			require.Error(t, truncated.UnmarshalParcel(q), "API %d truncated at %d", api, end)
		}
	}
}
