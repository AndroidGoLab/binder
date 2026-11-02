package view

import (
	"encoding/binary"
	"fmt"
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
			if api == 34 {
				s.Producer.Stability = parcel.StabilitySystem
			}
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

func TestSurfaceLocalReadAndTruncation(t *testing.T) {
	for _, api := range []int{29, 34} {
		p := parcel.New()
		p.SetTargetAPILevel(api)
		p.WriteString16("")
		p.WriteBool(false)
		p.WriteUint32(0x62717565)
		p.WriteLocalBinderLegacy(0x100000001, 0x200000002)
		if api == 34 {
			p.WriteInt32(12)
			p.WriteStrongBinder(17)
		}
		data := p.Data()
		objects := p.Objects()
		for end := 0; end < len(data); end++ {
			q := parcel.FromBytesWithObjects(data[:end], objects)
			q.SetTargetAPILevel(api)
			var s Surface
			require.Error(t, s.UnmarshalParcel(q))
		}
		p.SetPosition(0)
		var s Surface
		require.NoError(t, s.UnmarshalParcel(p))
		require.Equal(t, uintptr(0x100000001), s.Producer.Pointer)
		require.Equal(t, uintptr(0x200000002), s.Producer.Cookie)
		q := parcel.New()
		q.SetTargetAPILevel(api)
		require.Error(t, s.MarshalParcel(q))
	}
}

type surfaceOwner struct{}

func (surfaceOwner) BinderPtr() uintptr { return 1 }
func (surfaceOwner) Cookie() uintptr    { return 2 }

type surfaceLease struct{ released bool }

func (l *surfaceLease) Release() { l.released = true }

type surfaceContext struct{ leases []*surfaceLease }

func (c *surfaceContext) AcquireLocalBinder(owner parcel.RegisteredLocalBinder) (parcel.LocalBinderIdentity, parcel.BinderLease, error) {
	l := new(surfaceLease)
	c.leases = append(c.leases, l)
	return parcel.LocalBinderIdentity{Pointer: owner.BinderPtr(), Cookie: owner.Cookie()}, l, nil
}
func TestSurfaceLocalLeases(t *testing.T) {
	for _, api := range []int{29, 34} {
		p := parcel.New()
		p.SetTargetAPILevel(api)
		context := new(surfaceContext)
		p.SetBinderSerializationContext(context)
		s := Surface{Producer: parcel.BinderReference{Kind: parcel.BinderReferenceLocal, LocalOwner: surfaceOwner{}}}
		if api == 34 {
			s.Control = parcel.BinderReference{Kind: parcel.BinderReferenceLocal, LocalOwner: surfaceOwner{}}
		}
		require.NoError(t, s.MarshalParcel(p))
		require.Len(t, context.leases, 1+(api-29)/5)
		for _, l := range context.leases {
			require.False(t, l.released)
		}
		require.NoError(t, p.Recycle())
		for _, l := range context.leases {
			require.True(t, l.released)
		}
	}
}

type failingControlContext struct {
	lease surfaceLease
	calls int
}

func (c *failingControlContext) AcquireLocalBinder(owner parcel.RegisteredLocalBinder) (parcel.LocalBinderIdentity, parcel.BinderLease, error) {
	c.calls++
	if c.calls == 2 {
		return parcel.LocalBinderIdentity{}, nil, fmt.Errorf("control unavailable")
	}
	return parcel.LocalBinderIdentity{Pointer: owner.BinderPtr(), Cookie: owner.Cookie()}, &c.lease, nil
}
func TestSurfaceControlFailureRetainsProducerLease(t *testing.T) {
	p := parcel.New()
	p.SetTargetAPILevel(34)
	context := new(failingControlContext)
	p.SetBinderSerializationContext(context)
	reference := parcel.BinderReference{Kind: parcel.BinderReferenceLocal, LocalOwner: surfaceOwner{}}
	err := (&Surface{Producer: reference, Control: reference}).MarshalParcel(p)
	require.Error(t, err)
	require.Len(t, p.Objects(), 1)
	require.NotEmpty(t, p.Data())
	require.False(t, context.lease.released)
	require.NoError(t, p.Recycle())
	require.True(t, context.lease.released)
}
