package versionaware_test

import (
	"context"
	"fmt"
	pm "github.com/AndroidGoLab/binder/android/content/pm"
	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/binder/versionaware"
	"github.com/AndroidGoLab/binder/parcel"
	"github.com/stretchr/testify/require"
	"testing"
)

type captureTransport struct {
	binder.Transport
	Data    []byte
	Objects []uint64
}

func (c *captureTransport) Transact(_ context.Context, _ uint32, _ binder.TransactionCode, _ binder.TransactionFlags, data *parcel.Parcel) (*parcel.Parcel, error) {
	c.Data = append([]byte(nil), data.Data()...)
	c.Objects = append([]uint64(nil), data.Objects()...)
	return parcel.New(), nil
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
	oldRevisions, oldTables := versionaware.Revisions, versionaware.Tables
	versionaware.Revisions = versionaware.APIRevisions{29: {"29.test"}}
	versionaware.Tables = versionaware.MultiVersionTable{"29.test": {{Descriptor: pm.DescriptorIPackageManager, Methods: []versionaware.MethodEntry{{Method: pm.MethodIPackageManagerIsPackageAvailable, Code: 2}}}}}
	t.Cleanup(func() { versionaware.Revisions, versionaware.Tables = oldRevisions, oldTables })
	transport, err := versionaware.NewTransport(context.Background(), capture, binder.APILevelAndroid10, versionaware.OptionCachePath(""))
	require.NoError(t, err)
	transport.MergeTable(versionaware.VersionTable{pm.DescriptorIPackageManager: {pm.MethodIPackageManagerIsPackageAvailable: 2}})
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
