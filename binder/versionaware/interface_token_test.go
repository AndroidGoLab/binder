package versionaware

import (
	"context"
	"encoding/binary"
	"fmt"
	"testing"

	pm "github.com/AndroidGoLab/binder/android/content/pm"
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

// packageTransport captures a request and supplies a successful boolean reply.
//
// Example: exercise a generated package manager proxy without a device.
type packageTransport struct {
	captureTransport
}

// Transact captures the request and returns a successful boolean response.
//
// Example: represent an available package in a wire fixture.
func (c *packageTransport) Transact(
	ctx context.Context,
	handle uint32,
	code binder.TransactionCode,
	flags binder.TransactionFlags,
	data *parcel.Parcel,
) (*parcel.Parcel, error) {
	_, err := c.captureTransport.Transact(ctx, handle, code, flags, data)
	if err != nil {
		return nil, err
	}
	request := parcel.FromBytes(c.Data)
	strict, err := request.ReadInt32()
	if err != nil || strict != -1<<31 {
		return nil, fmt.Errorf("invalid strict mode header: %d: %v", strict, err)
	}
	workSource, err := request.ReadInt32()
	if err != nil || workSource != -1 {
		return nil, fmt.Errorf("invalid work source: %d: %v", workSource, err)
	}
	descriptor, err := request.ReadString16()
	if err != nil || descriptor != pm.DescriptorIPackageManager {
		return nil, fmt.Errorf("invalid descriptor %q: %v", descriptor, err)
	}
	packageName, err := request.ReadString16()
	if err != nil || packageName != "example.app" {
		return nil, fmt.Errorf("invalid package %q: %v", packageName, err)
	}
	userID, err := request.ReadInt32()
	if err != nil || userID != 10 || request.Position() != request.Len() {
		return nil, fmt.Errorf("invalid user ID or trailing data: %d: %v", userID, err)
	}
	reply := parcel.New()
	reply.WriteInt32(0)
	reply.WriteBool(true)
	return reply, nil
}

// TestAndroid10GeneratedPackageManagerRequest checks the generated proxy wire request.
//
// Example: query whether a package is available on an API 29 transport.
func TestAndroid10GeneratedPackageManagerRequest(t *testing.T) {
	capture := &packageTransport{}
	transport := &Transport{inner: capture, apiLevel: binder.APILevelAndroid10, table: VersionTable{
		pm.DescriptorIPackageManager: {pm.MethodIPackageManagerIsPackageAvailable: 2},
	}}
	proxy := pm.NewPackageManagerProxy(binder.NewProxyBinder(transport, binder.CallerIdentity{UserID: 10}, 1))
	available, err := proxy.IsPackageAvailable(context.Background(), "example.app")
	require.NoError(t, err)
	require.True(t, available)
	expected := parcel.New()
	expected.WriteInt32(-1 << 31)
	expected.WriteInt32(-1)
	expected.WriteString16(pm.DescriptorIPackageManager)
	expected.WriteString16("example.app")
	expected.WriteInt32(10)
	require.Equal(t, expected.Data(), capture.Data)
	require.Empty(t, capture.Objects)
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
