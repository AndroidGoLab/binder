package versionaware

import (
	"context"
	"encoding/binary"
	"testing"

	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/parcel"
	"github.com/stretchr/testify/require"
)

// captureTransport records the request that reaches the underlying transport.
//
// Example: inspect a serialized request without a Binder device.
type captureTransport struct {
	binder.Transport
	Data    []byte
	Objects []uint64
}

type translationLease struct{ releases int }

func (l *translationLease) Release() { l.releases++ }

// TestAndroid10TranslationReleasesClone verifies that translation keeps the
// caller's lease alive while releasing the temporary request's ownership.
func TestAndroid10TranslationReleasesClone(t *testing.T) {
	p := parcel.New()
	p.WriteInterfaceToken("test.Interface")
	lease := &translationLease{}
	p.RetainBinderLease(lease)
	transport := &Transport{inner: &captureTransport{}, apiLevel: binder.APILevelAndroid10}
	_, err := transport.Transact(context.Background(), 1, 2, 0, p)
	require.NoError(t, err)
	require.Zero(t, lease.releases)
	require.NoError(t, p.Recycle())
	require.Equal(t, 1, lease.releases)
}

// Transact copies the request for inspection.
//
// Example: capture a request before returning an empty reply.
func (c *captureTransport) Transact(
	_ context.Context,
	_ uint32,
	_ binder.TransactionCode,
	_ binder.TransactionFlags,
	data *parcel.Parcel,
) (*parcel.Parcel, error) {
	c.Data = append([]byte(nil), data.Data()...)
	c.Objects = append([]uint64(nil), data.Objects()...)
	return parcel.New(), nil
}

// TestAndroid10InterfaceTokenTranslation checks conversion and caller ownership.
//
// Example: a generated proxy sends an API 29 request with a binder argument.
func TestAndroid10InterfaceTokenTranslation(t *testing.T) {
	p := parcel.New()
	p.WriteInterfaceToken("android.content.pm.IPackageManager")
	p.WriteString16("package.name")
	p.WriteStrongBinder(7)
	p.SetPosition(4)
	original := append([]byte(nil), p.Data()...)
	offsets := append([]uint64(nil), p.Objects()...)
	capture := &captureTransport{}
	transport := &Transport{inner: capture, apiLevel: binder.APILevelAndroid10}
	expected := append(append([]byte(nil), original[:8]...), original[12:]...)
	for i := 0; i < 2; i++ {
		_, err := transport.Transact(context.Background(), 1, 2, 0, p)
		require.NoError(t, err)
		require.Equal(t, expected, capture.Data)
		require.Equal(t, []uint64{offsets[0] - 4}, capture.Objects)
		require.Equal(t, original, p.Data())
		require.Equal(t, offsets, p.Objects())
		require.Equal(t, 4, p.Position())
	}
}

// TestInterfaceTokenPassThrough checks versions and noncanonical requests.
//
// Example: a native Android 10 service manager request is already legacy.
func TestInterfaceTokenPassThrough(t *testing.T) {
	modern := parcel.New()
	modern.WriteInterfaceToken("android.content.pm.IPackageManager")
	legacy := parcel.New()
	legacy.WriteInt32(-1 << 31)
	legacy.WriteInt32(-1)
	legacy.WriteString16("android.os.IServiceManager")
	raw := parcel.New()
	raw.WriteInt32(42)
	malformed := append([]byte(nil), modern.Data()...)
	binary.LittleEndian.PutUint32(malformed[12:16], 0x7fffffff)
	unterminated := append([]byte(nil), modern.Data()...)
	unterminated[16+2*int(binary.LittleEndian.Uint32(unterminated[12:16]))] = 1
	for _, tc := range []struct {
		Name   string
		API    int
		Parcel *parcel.Parcel
	}{
		{"modern API", 30, modern}, {"unknown API", 0, modern},
		{"legacy", binder.APILevelAndroid10, legacy}, {"raw", binder.APILevelAndroid10, raw},
		{"unmarked canonical bytes", binder.APILevelAndroid10, parcel.FromBytes(append([]byte(nil), modern.Data()...))},
		{"malformed length", binder.APILevelAndroid10, parcel.FromBytes(malformed)},
		{"missing terminator", binder.APILevelAndroid10, parcel.FromBytes(unterminated)},
	} {
		t.Run(tc.Name,
			// Check that this request remains unchanged.
			//
			// Example: preserve an unmarked request.
			func(t *testing.T) {
				capture := &captureTransport{}
				transport := &Transport{inner: capture, apiLevel: tc.API}
				_, err := transport.Transact(context.Background(), 1, 2, 0, tc.Parcel)
				require.NoError(t, err)
				require.Equal(t, tc.Parcel.Data(), capture.Data)
				require.Equal(t, tc.Parcel.Objects(), capture.Objects)
			})
	}
}

// TestAndroid10TokenMarkerLifecycle checks that only an initial token is adapted.
//
// Example: reusing a parcel for raw data must not retain its old token marker.
func TestAndroid10TokenMarkerLifecycle(t *testing.T) {
	recycled := parcel.New()
	recycled.WriteInterfaceToken("example.IFoo")
	recycled.Recycle()
	recycled.WriteRawBytes([]byte{1, 2, 3, 4})
	prefixed := parcel.New()
	prefixed.WriteInt32(123)
	prefixed.WriteInterfaceToken("example.IFoo")
	invalidOffset := parcel.New()
	invalidOffset.WriteInterfaceToken("example.IFoo")
	invalidOffset.WriteStrongBinder(1)
	invalidOffset.Objects()[0] = 8
	for _, tc := range []struct {
		Name   string
		Parcel *parcel.Parcel
	}{
		{"recycled", recycled}, {"prefixed token", prefixed}, {"overlapping object", invalidOffset},
	} {
		t.Run(tc.Name,
			// Check that this request remains unchanged.
			//
			// Example: preserve an unmarked request.
			func(t *testing.T) {
				capture := &captureTransport{}
				transport := &Transport{inner: capture, apiLevel: binder.APILevelAndroid10}
				_, err := transport.Transact(context.Background(), 1, 2, 0, tc.Parcel)
				require.NoError(t, err)
				require.Equal(t, tc.Parcel.Data(), capture.Data)
				require.Equal(t, tc.Parcel.Objects(), capture.Objects)
			})
	}
}
