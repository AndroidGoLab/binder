package parcel

import (
	"encoding/binary"
	"fmt"
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
		{"modern null", 34, BinderReference{}, 0x73622a85, 0, 0, 0, nil, 0},
		{"modern remote", 34, BinderReference{Kind: BinderReferenceRemote, Handle: 7}, 0x73682a85, 0x13, 7, 0, []uint64{0}, 12},
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
			expected := tc.ref
			if tc.api == 34 && expected.Kind != BinderReferenceNull {
				expected.Stability = StabilitySystem
			}
			require.Equal(t, expected, got)
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

func TestBinderReferenceRejectsUnregisteredAndInvalidWire(t *testing.T) {
	p := New()
	p.SetTargetAPILevel(34)
	require.Error(t, p.WriteBinderReference(BinderReference{Kind: BinderReferenceLocal, Pointer: 1, Cookie: 1}))
	require.Empty(t, p.Data())
	for _, tc := range []struct {
		kind      uint32
		value     uint64
		stability uint32
		offsets   []uint64
	}{
		{0x73682a85, 42, 12, nil}, {0x73682a85, 42, 0, []uint64{0}}, {0x73682a85, 42, 99, []uint64{0}}, {0x73622a85, 0, 12, nil},
	} {
		b := make([]byte, 28)
		binary.LittleEndian.PutUint32(b, tc.kind)
		binary.LittleEndian.PutUint64(b[8:], tc.value)
		binary.LittleEndian.PutUint32(b[24:], tc.stability)
		q := FromBytesWithObjects(b, tc.offsets)
		q.SetTargetAPILevel(34)
		_, err := q.ReadBinderReference()
		require.Error(t, err)
	}
}

func TestBinderReferencePreservesDeclaredStability(t *testing.T) {
	for _, level := range []StabilityLevel{3, 12, 63} {
		p := New()
		p.SetTargetAPILevel(34)
		original := BinderReference{Kind: BinderReferenceRemote, Handle: 42, Stability: level}
		require.NoError(t, p.WriteBinderReference(original))
		require.Equal(t, uint32(level), binary.LittleEndian.Uint32(p.Data()[24:]))
		p.SetPosition(0)
		got, err := p.ReadBinderReference()
		require.NoError(t, err)
		require.Equal(t, original, got)
	}
}

type testLeaseOwner struct{}

func (testLeaseOwner) BinderPtr() uintptr { return 1 }
func (testLeaseOwner) Cookie() uintptr    { return 2 }

type testBinderLease struct{ released bool }

func (l *testBinderLease) Release() { l.released = true }

type testBinderContext struct{ lease *testBinderLease }

func (c *testBinderContext) AcquireLocalBinder(owner RegisteredLocalBinder) (LocalBinderIdentity, BinderLease, error) {
	return LocalBinderIdentity{Pointer: owner.BinderPtr(), Cookie: owner.Cookie()}, c.lease, nil
}
func TestBinderReferenceLocalLease(t *testing.T) {
	for _, api := range []int{29, 34} {
		lease := new(testBinderLease)
		p := New()
		p.SetTargetAPILevel(api)
		p.SetBinderSerializationContext(&testBinderContext{lease})
		ref := BinderReference{Kind: BinderReferenceLocal, LocalOwner: testLeaseOwner{}}
		require.NoError(t, p.WriteBinderReference(ref))
		require.Equal(t, uint64(1), binary.LittleEndian.Uint64(p.Data()[8:]))
		require.Equal(t, uint64(2), binary.LittleEndian.Uint64(p.Data()[16:]))
		require.Equal(t, []uint64{0}, p.Objects())
		require.False(t, lease.released)
		require.NoError(t, p.Recycle())
		require.True(t, lease.released)
	}
}

func TestBinderReferenceLocalIdentityMismatchReleasesLease(t *testing.T) {
	lease := new(testBinderLease)
	p := New()
	p.SetTargetAPILevel(34)
	p.SetBinderSerializationContext(&testBinderContext{lease})
	err := p.WriteBinderReference(BinderReference{Kind: BinderReferenceLocal, Pointer: 9, LocalOwner: testLeaseOwner{}})
	require.Error(t, err)
	require.True(t, lease.released)
	require.Empty(t, p.Data())
	require.Empty(t, p.Objects())
}

type failingBinderContext struct{ lease *testBinderLease }

func (c *failingBinderContext) AcquireLocalBinder(RegisteredLocalBinder) (LocalBinderIdentity, BinderLease, error) {
	return LocalBinderIdentity{}, c.lease, fmt.Errorf("registration failed")
}
func TestBinderReferenceAcquireErrorReleasesLease(t *testing.T) {
	lease := new(testBinderLease)
	p := New()
	p.SetTargetAPILevel(34)
	p.SetBinderSerializationContext(&failingBinderContext{lease})
	require.Error(t, p.WriteBinderReference(BinderReference{Kind: BinderReferenceLocal, LocalOwner: testLeaseOwner{}}))
	require.True(t, lease.released)
	require.Empty(t, p.Data())
}
