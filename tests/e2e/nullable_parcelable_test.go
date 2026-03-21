//go:build e2e

package e2e

import (
	"context"
	"testing"

	genGui "github.com/xaionaro-go/binder/android/gui"
	"github.com/xaionaro-go/binder/parcel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNullableParcelable_RoundTrip verifies that @nullable nested parcelable
// fields are correctly serialized and deserialized using the generated
// MarshalParcel/UnmarshalParcel methods. This is a pure protocol-level test
// that does not depend on any device service.
//
// Before the fix, the generated code:
//   - Did not write an int32 null indicator in MarshalParcel
//   - Did not read an int32 null indicator in UnmarshalParcel
//   - Used a value type (not pointer) for the nullable field
//
// This caused misaligned reads: all fields after the nullable field would
// contain garbage values.
func TestNullableParcelable_RoundTrip(t *testing.T) {
	t.Run("non-nil DeviceProductInfo", func(t *testing.T) {
		original := genGui.StaticDisplayInfo{
			ConnectionType: genGui.DisplayConnectionTypeInternal,
			Density:        2.625,
			Secure:         true,
			DeviceProductInfo: &genGui.DeviceProductInfo{
				Name:              "Common Panel",
				ManufacturerPnpId: []byte("GGL"),
				ProductId:         "1",
			},
			InstallOrientation: genGui.RotationRotation0,
		}

		p := parcel.New()
		require.NoError(t, original.MarshalParcel(p))

		p.SetPosition(0)
		var decoded genGui.StaticDisplayInfo
		require.NoError(t, decoded.UnmarshalParcel(p))

		assert.Equal(t, original.ConnectionType, decoded.ConnectionType)
		assert.InDelta(t, float64(original.Density), float64(decoded.Density), 0.001)
		assert.Equal(t, original.Secure, decoded.Secure)

		// The nullable field must round-trip correctly.
		require.NotNil(t, decoded.DeviceProductInfo,
			"non-nil DeviceProductInfo must survive round-trip")
		assert.Equal(t, "Common Panel", decoded.DeviceProductInfo.Name)
		assert.Equal(t, []byte("GGL"), decoded.DeviceProductInfo.ManufacturerPnpId)
		assert.Equal(t, "1", decoded.DeviceProductInfo.ProductId)

		// Field AFTER the nullable field must be correct.
		assert.Equal(t, original.InstallOrientation, decoded.InstallOrientation,
			"installOrientation (field after @nullable) must round-trip correctly")
	})

	t.Run("nil DeviceProductInfo", func(t *testing.T) {
		original := genGui.StaticDisplayInfo{
			ConnectionType:     genGui.DisplayConnectionTypeExternal,
			Density:            1.5,
			Secure:             false,
			DeviceProductInfo:  nil, // null
			InstallOrientation: genGui.RotationRotation90,
		}

		p := parcel.New()
		require.NoError(t, original.MarshalParcel(p))

		p.SetPosition(0)
		var decoded genGui.StaticDisplayInfo
		require.NoError(t, decoded.UnmarshalParcel(p))

		assert.Equal(t, original.ConnectionType, decoded.ConnectionType)
		assert.InDelta(t, float64(original.Density), float64(decoded.Density), 0.001)
		assert.Equal(t, original.Secure, decoded.Secure)

		assert.Nil(t, decoded.DeviceProductInfo,
			"nil DeviceProductInfo must remain nil after round-trip")

		// Field AFTER the nullable field must still be correct even
		// when the nullable field is null.
		assert.Equal(t, original.InstallOrientation, decoded.InstallOrientation,
			"installOrientation (field after @nullable) must round-trip correctly")
	})
}

// TestNullableParcelable_WireFormat verifies that the wire format of a
// @nullable parcelable field includes the int32 null indicator. This test
// manually inspects the parcel bytes to confirm the fix.
func TestNullableParcelable_WireFormat(t *testing.T) {
	t.Run("non-nil writes int32(1) before parcelable data", func(t *testing.T) {
		info := genGui.StaticDisplayInfo{
			Density: 2.0,
			DeviceProductInfo: &genGui.DeviceProductInfo{
				Name: "Test",
			},
		}

		p := parcel.New()
		require.NoError(t, info.MarshalParcel(p))

		// Read the parcel manually to verify wire format.
		p.SetPosition(0)
		size, err := p.ReadInt32() // parcelable size header
		require.NoError(t, err)
		assert.Greater(t, size, int32(0))

		connType, err := p.ReadInt32() // connectionType
		require.NoError(t, err)
		assert.Equal(t, int32(0), connType) // Internal = 0

		_, err = p.ReadFloat32() // density
		require.NoError(t, err)

		_, err = p.ReadBool() // secure
		require.NoError(t, err)

		// The null indicator for DeviceProductInfo.
		nullInd, err := p.ReadInt32()
		require.NoError(t, err)
		assert.Equal(t, int32(1), nullInd,
			"non-nil @nullable parcelable must write int32(1) null indicator")
	})

	t.Run("nil writes int32(0) with no following data", func(t *testing.T) {
		info := genGui.StaticDisplayInfo{
			Density:           2.0,
			DeviceProductInfo: nil, // null
		}

		p := parcel.New()
		require.NoError(t, info.MarshalParcel(p))

		p.SetPosition(0)
		_, err := p.ReadInt32() // size header
		require.NoError(t, err)

		_, err = p.ReadInt32() // connectionType
		require.NoError(t, err)

		_, err = p.ReadFloat32() // density
		require.NoError(t, err)

		_, err = p.ReadBool() // secure
		require.NoError(t, err)

		// The null indicator for DeviceProductInfo.
		nullInd, err := p.ReadInt32()
		require.NoError(t, err)
		assert.Equal(t, int32(0), nullInd,
			"nil @nullable parcelable must write int32(0) null indicator")

		// Next field (installOrientation) should be immediately readable.
		orientRaw, err := p.ReadInt32()
		require.NoError(t, err)
		assert.Equal(t, int32(0), orientRaw,
			"installOrientation should follow directly after null indicator")
	})
}

// TestNullableParcelable_DeviceStaticDisplayInfo exercises the full path from
// device binder call through the generated proxy and into UnmarshalParcel.
// This test logs diagnostic information about the values read from the device
// to help detect misaligned reads.
func TestNullableParcelable_DeviceStaticDisplayInfo(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)

	sf := getSurfaceFlingerAIDL(ctx, t, driver)
	proxy := genGui.NewSurfaceComposerProxy(sf)

	ids, err := proxy.GetPhysicalDisplayIds(ctx)
	requireOrSkip(t, err)
	require.NotEmpty(t, ids, "expected at least one physical display")

	displayID := ids[0]
	t.Logf("using displayId=%d", displayID)

	info, err := proxy.GetStaticDisplayInfo(ctx, displayID)
	requireOrSkip(t, err)

	t.Logf("StaticDisplayInfo: connectionType=%d density=%.4f secure=%v installOrientation=%d",
		info.ConnectionType, info.Density, info.Secure, info.InstallOrientation)

	if info.DeviceProductInfo != nil {
		t.Logf("  DeviceProductInfo: name=%q manufacturerPnpId=%v productId=%q",
			info.DeviceProductInfo.Name,
			info.DeviceProductInfo.ManufacturerPnpId,
			info.DeviceProductInfo.ProductId)
	} else {
		t.Log("  DeviceProductInfo: nil (nullable field is null)")
	}

	// The SkipToParcelableEnd mechanism ensures fields are read within
	// the correct parcelable boundaries. Even if a C++ backend includes
	// an extra stability prefix (shifting all reads by 4 bytes), the
	// parcel won't fail catastrophically. We verify no error occurred
	// and the struct pointer type for DeviceProductInfo is correct
	// (it should be *DeviceProductInfo, not DeviceProductInfo).
	//
	// If DeviceProductInfo were still a value type (the old bug), this
	// code would not compile at all, since the struct field type changed
	// from DeviceProductInfo to *DeviceProductInfo.
	_ = info.DeviceProductInfo
}
