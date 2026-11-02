package gui

import (
	"context"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/parcel"
	"github.com/stretchr/testify/require"
)

// displayIDBinder records the physical display ID request.
//
// Example: check the Android 10 transaction code and interface token.
type displayIDBinder struct {
	binder.IBinder
	Code       binder.TransactionCode
	Descriptor string
}

// Transact records the request and returns a physical ID vector.
//
// Example: simulate a SurfaceFlinger physical display IDs response.
func (b *displayIDBinder) Transact(
	ctx context.Context,
	code binder.TransactionCode,
	flags binder.TransactionFlags,
	data *parcel.Parcel,
) (*parcel.Parcel, error) {
	b.Code = code
	data.SetPosition(0)
	var err error
	b.Descriptor, err = data.ReadInterfaceToken()
	if err != nil {
		return nil, err
	}
	reply := parcel.New()
	reply.WriteInt32(1)
	reply.WriteUint64(1234)
	reply.SetPosition(0)
	return reply, nil
}

// TestAndroid10PhysicalDisplayIDs checks the Android 10 Binder transaction.
//
// Example: enumerate a physical display with its stable identifier.
func TestAndroid10PhysicalDisplayIDs(t *testing.T) {
	remote := &displayIDBinder{}
	ids, err := PhysicalDisplayIDs(context.Background(), remote, 29)
	require.NoError(t, err)
	require.Equal(t, binder.TransactionCode(36), remote.Code)
	require.Equal(t, "android.ui.ISurfaceComposer", remote.Descriptor)
	require.Equal(t, []uint64{1234}, ids)
}

// android10InfoBinder simulates Android 10 physical display queries.
//
// Example: verify token and active configuration transactions.
type android10InfoBinder struct {
	binder.IBinder
	Codes  []binder.TransactionCode
	Token  uint32
	Active int32
	Status int32
}

// Transact returns a source-layout reply for each physical display query.
//
// Example: select the second of two physical display configurations.
func (b *android10InfoBinder) Transact(
	ctx context.Context,
	code binder.TransactionCode,
	flags binder.TransactionFlags,
	data *parcel.Parcel,
) (*parcel.Parcel, error) {
	b.Codes = append(b.Codes, code)
	data.SetPosition(0)
	if _, err := data.ReadInterfaceToken(); err != nil {
		return nil, err
	}
	r := parcel.New()
	switch code {
	case 7:
		if _, err := data.ReadUint64(); err != nil {
			return nil, err
		}
		if b.Token == 0 {
			r.WriteNullStrongBinderLegacy()
		} else {
			r.WriteStrongBinderLegacy(b.Token)
		}
	case 12, 11:
		token, present, err := data.ReadNullableStrongBinderLegacy()
		if err != nil {
			return nil, err
		}
		if !present || token != b.Token {
			return nil, fmt.Errorf("wrong display token")
		}
		if code == 12 {
			r.WriteInt32(b.Active)
			break
		}
		r.WriteInt32(b.Status)
		if b.Status != 0 {
			break
		}
		r.WriteInt32(2)
		for _, width := range []uint32{100, 200} {
			r.WriteUint32(width)
			r.WriteUint32(300)
			for _, value := range []float32{1, 2, 60, 3} {
				r.WriteFloat32(value)
			}
			r.WriteInt32(0)
			r.WriteInt32(0)
			r.WriteInt64(0)
			r.WriteInt64(0)
			r.WriteUint32(width)
			r.WriteUint32(300)
		}
	default:
		return nil, fmt.Errorf("unexpected transaction %d", code)
	}
	r.SetPosition(0)
	return r, nil
}

// TestAndroid10PhysicalDisplayInfo verifies the legacy transaction sequence.
//
// Example: retrieve the active configuration of the selected physical display.
func TestAndroid10PhysicalDisplayInfo(t *testing.T) {
	remote := &android10InfoBinder{Token: 42, Active: 1}
	info, err := GetPhysicalDisplayInfo(context.Background(), remote, 29, 123)
	require.NoError(t, err)
	require.Equal(t, uint32(200), info.Width)
	require.Equal(t, []binder.TransactionCode{7, 12, 11}, remote.Codes)
	for _, remote := range []*android10InfoBinder{{}, {Token: 42, Active: -1}, {Token: 42, Status: -1}} {
		_, err := GetPhysicalDisplayInfo(context.Background(), remote, 29, 123)
		require.Error(t, err)
	}
}

// android14InfoBinder simulates the Android 14 AIDL display transactions.
//
// Example: verify that static, state and mode metadata are combined.
type android14InfoBinder struct {
	binder.IBinder
	Codes []binder.TransactionCode
}

// Transport returns the transport used to construct a display token proxy.
//
// Example: construct a token proxy for this transaction test.
func (b *android14InfoBinder) Transport() binder.VersionAwareTransport { return nil }

// Identity returns the caller identity used for the display token proxy.
//
// Example: construct a token proxy for this transaction test.
func (b *android14InfoBinder) Identity() binder.CallerIdentity { return binder.CallerIdentity{} }

// Transact returns Android 14 AIDL-formatted display metadata.
//
// Example: combine a static density with active mode geometry.
func (b *android14InfoBinder) Transact(
	ctx context.Context,
	code binder.TransactionCode,
	flags binder.TransactionFlags,
	data *parcel.Parcel,
) (*parcel.Parcel, error) {
	b.Codes = append(b.Codes, code)
	r := parcel.New()
	binder.WriteStatus(r, nil)
	switch code {
	case 12:
		r.WriteInt32(1)
		value := StaticDisplayInfo{Density: 2.5, Secure: true}
		if err := value.MarshalParcel(r); err != nil {
			return nil, err
		}
	case 7:
		r.WriteStrongBinder(42)
	case 11:
		r.WriteInt32(1)
		value := DisplayState{LayerStack: 4, Orientation: RotationRotation90}
		if err := value.MarshalParcel(r); err != nil {
			return nil, err
		}
	case 13:
		r.WriteInt32(1)
		outer := parcel.WriteParcelableHeader(r)
		r.WriteInt32(1)
		r.WriteInt32(1)
		mode := parcel.WriteParcelableHeader(r)
		r.WriteInt32(9)
		r.WriteInt32(1)
		size := parcel.WriteParcelableHeader(r)
		r.WriteInt32(100)
		r.WriteInt32(200)
		parcel.WriteParcelableFooter(r, size)
		r.WriteFloat32(300)
		r.WriteFloat32(301)
		r.WriteInt32(0)
		r.WriteFloat32(60)
		parcel.WriteParcelableFooter(r, mode)
		r.WriteInt32(9)
		parcel.WriteParcelableFooter(r, outer)
	default:
		return nil, fmt.Errorf("unexpected transaction %d", code)
	}
	r.SetPosition(0)
	return r, nil
}

// TestAndroid14PhysicalDisplayInfo verifies the AIDL transaction sequence.
//
// Example: retrieve metadata from a selected physical display.
func TestAndroid14PhysicalDisplayInfo(t *testing.T) {
	remote := &android14InfoBinder{}
	info, err := GetPhysicalDisplayInfo(context.Background(), remote, 34, 123)
	require.NoError(t, err)
	require.Equal(t, []binder.TransactionCode{12, 7, 11, 13}, remote.Codes)
	require.Equal(t, uint32(100), info.Width)
	require.Equal(t, float32(2.5), info.Density)
	require.True(t, info.Secure)
	require.Equal(t, RotationRotation90, info.Rotation)
	require.NotNil(t, info.LayerStack)
	require.Equal(t, int32(4), *info.LayerStack)
}

// TestAndroid10DisplayInfo verifies the native Android 10 struct layout.
//
// Example: decode a physical display configuration returned by SurfaceFlinger.
func TestAndroid10DisplayInfo(t *testing.T) {
	p := parcel.New()
	defer p.Recycle()
	p.WriteUint32(1080)
	p.WriteUint32(2400)
	p.WriteFloat32(420)
	p.WriteFloat32(421)
	p.WriteFloat32(90)
	p.WriteFloat32(2.75)
	p.WriteInt32(0x101)
	p.WriteInt32(0)
	p.WriteInt64(10)
	p.WriteInt64(20)
	p.WriteUint32(1080)
	p.WriteUint32(2400)
	p.SetPosition(0)
	info, err := readAndroid10DisplayInfo(p)
	require.NoError(t, err)
	require.Equal(t, PhysicalDisplayInfo{Width: 1080, Height: 2400, XDPI: 420, YDPI: 421, FPS: 90, Density: 2.75, Rotation: 1, Secure: true}, info)
}

// TestAndroid14DisplayModes verifies Android 14 mode decoding and active selection.
//
// Example: select the active mode from a dynamic display info reply.
func TestAndroid14DisplayModes(t *testing.T) {
	p := parcel.New()
	defer p.Recycle()
	outer := parcel.WriteParcelableHeader(p)
	p.WriteInt32(1)
	p.WriteInt32(1)
	mode := parcel.WriteParcelableHeader(p)
	p.WriteInt32(7)
	p.WriteInt32(1)
	size := parcel.WriteParcelableHeader(p)
	p.WriteInt32(1080)
	p.WriteInt32(2400)
	parcel.WriteParcelableFooter(p, size)
	p.WriteFloat32(420)
	p.WriteFloat32(421)
	p.WriteInt32(0)
	p.WriteFloat32(90)
	p.WriteInt64(1)
	p.WriteInt64(2)
	p.WriteInt64(3)
	p.WriteInt32(4)
	parcel.WriteParcelableFooter(p, mode)
	p.WriteInt32(7)
	p.WriteFloat32(60)
	parcel.WriteParcelableFooter(p, outer)
	p.SetPosition(0)
	info, err := readAndroid14DisplayModes(p)
	require.NoError(t, err)
	require.Equal(t, PhysicalDisplayInfo{Width: 1080, Height: 2400, XDPI: 420, YDPI: 421, FPS: 90}, info)
	binary.LittleEndian.PutUint32(p.Data()[p.Len()-8:], 8)
	p.SetPosition(0)
	_, err = readAndroid14DisplayModes(p)
	require.Error(t, err)
}
