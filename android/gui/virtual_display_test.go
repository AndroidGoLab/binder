package gui

import (
	"context"
	"fmt"
	"testing"

	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/parcel"
	"github.com/stretchr/testify/require"
)

// These assignments verify that existing constructor function types remain compatible.
//
// Example: callers may assign CreateVirtualDisplay to its original function type.
var _ func(context.Context, binder.IBinder, binder.IBinder, binder.Transport, int, *binder.StubBinder, int32, int32, int32, int32) (*VirtualDisplay, error) = CreateVirtualDisplay

// This assignment also preserves the layer-stack constructor's function type.
//
// Example: callers may store CreateVirtualDisplayOnLayerStack as a function value.
var _ func(context.Context, binder.IBinder, binder.IBinder, binder.Transport, int, *binder.StubBinder, int32, int32, int32, int32, int32) (*VirtualDisplay, error) = CreateVirtualDisplayOnLayerStack

// TestAndroid10DisplayState verifies the geometry and flag order from
// Android 10's DisplayState::write.
func TestAndroid10DisplayState(t *testing.T) {
	p := parcel.New()
	writeAndroid10DisplayGeometry(p, 1080, 2400, 360, 800)
	for _, want := range []int32{7, 0, 0, 0, 0, 1080, 2400, 0, 0, 360, 800, 360, 800} {
		got, err := p.ReadInt32()
		require.NoError(t, err)
		require.Equal(t, want, got)
	}
	require.Equal(t, p.Len(), p.Position())
}

// TestVirtualDisplayRejectsInvalidGeometry checks malformed projections are
// rejected before any Binder transactions.
func TestVirtualDisplayRejectsInvalidGeometry(t *testing.T) {
	_, err := CreateVirtualDisplay(context.Background(), nil, nil, nil, 29, nil, 0, 2400, 360, 800)
	require.Error(t, err)
	_, err = CreateVirtualDisplay(context.Background(), nil, nil, nil, 30, nil, 1080, 2400, 360, 800)
	require.Error(t, err)
}

// virtualDisplayRemote records SurfaceFlinger transactions.
type virtualDisplayRemote struct {
	binder.IBinder
	requests []*parcel.Parcel
	codes    []binder.TransactionCode
	aidl     bool
}

// Transact returns a legacy display token and records requests.
func (r *virtualDisplayRemote) Transact(_ context.Context, code binder.TransactionCode, _ binder.TransactionFlags, p *parcel.Parcel) (*parcel.Parcel, error) {
	r.codes = append(r.codes, code)
	r.requests = append(r.requests, parcel.FromBytes(append([]byte(nil), p.Data()...)))
	reply := parcel.New()
	if r.aidl && code == 4 {
		binder.WriteStatus(reply, nil)
		reply.WriteStrongBinder(19)
	}
	if code == 5 {
		reply.WriteStrongBinderLegacy(19)
	}
	return reply, nil
}

// TestAndroid10CreateUsesNativeDescriptor checks the native service contract.
func TestAndroid10CreateUsesNativeDescriptor(t *testing.T) {
	r := &virtualDisplayRemote{}
	_, err := CreateVirtualDisplay(context.Background(), r, nil, &virtualDisplayTransport{}, binder.APILevelAndroid10, binder.NewStubBinder(nil), 1080, 2400, 360, 800)
	require.NoError(t, err)
	p := r.requests[0]
	p.SetPosition(0)
	descriptor, err := p.ReadInterfaceToken()
	require.NoError(t, err)
	require.Equal(t, "android.ui.ISurfaceComposer", descriptor)
}

// virtualDisplayTransport registers a producer for a wire fixture.
type virtualDisplayTransport struct{ binder.Transport }

// RegisterReceiver supplies a stable local Binder cookie.
func (*virtualDisplayTransport) RegisterReceiver(context.Context, binder.TransactionReceiver) uintptr {
	return 1
}

// TestAndroid14DisplayState verifies the extra flags field in its projection.
func TestAndroid14DisplayState(t *testing.T) {
	p := parcel.New()
	writeAndroid14DisplayGeometry(p, 1080, 2400, 360, 800)
	for _, want := range []int32{7, 0, 0, 0, 0, 0, 1080, 2400, 0, 0, 360, 800, 360, 800} {
		got, err := p.ReadInt32()
		require.NoError(t, err)
		require.Equal(t, want, got)
	}
	require.Equal(t, p.Len(), p.Position())
}

// TestAndroid10TransactionState verifies the native transaction's counts,
// Binder object widths and projection fields.
func TestAndroid10TransactionState(t *testing.T) {
	r := &virtualDisplayRemote{}
	d, err := CreateVirtualDisplay(context.Background(), r, nil, &virtualDisplayTransport{}, 29, binder.NewStubBinder(nil), 1080, 2400, 360, 800)
	require.NoError(t, err)
	require.NoError(t, d.Close(context.Background()))
	require.Equal(t, []binder.TransactionCode{5, 8, 6}, r.codes)
	p := r.requests[1]
	descriptor, err := p.ReadInterfaceToken()
	require.NoError(t, err)
	require.Equal(t, "android.ui.ISurfaceComposer", descriptor)
	layers, err := p.ReadInt32()
	require.NoError(t, err)
	require.EqualValues(t, 0, layers)
	displays, err := p.ReadInt32()
	require.NoError(t, err)
	require.EqualValues(t, 1, displays)
	token, present, err := p.ReadNullableStrongBinderLegacy()
	require.NoError(t, err)
	require.True(t, present)
	require.EqualValues(t, 19, token)
	_, present, err = p.ReadNullableStrongBinderLegacy()
	require.NoError(t, err)
	require.True(t, present)
	for _, want := range []int32{7, 0, 0, 0, 0, 1080, 2400, 0, 0, 360, 800, 360, 800, 0} {
		got, err := p.ReadInt32()
		require.NoError(t, err)
		require.Equal(t, want, got)
	}
	_, present, err = p.ReadNullableStrongBinderLegacy()
	require.NoError(t, err)
	require.False(t, present)
	count, err := p.ReadInt32()
	require.NoError(t, err)
	require.Zero(t, count)
	sync, err := p.ReadBool()
	require.NoError(t, err)
	require.False(t, sync)
	desired, err := p.ReadInt64()
	require.NoError(t, err)
	require.Zero(t, desired)
	// The final weak-null binder is 24 bytes followed by the cache ID and callback count.
	p.SetPosition(p.Position() + 24)
	cache, err := p.ReadUint64()
	require.NoError(t, err)
	require.Zero(t, cache)
	callbacks, err := p.ReadInt32()
	require.NoError(t, err)
	require.Zero(t, callbacks)
	require.Equal(t, p.Len(), p.Position())
}

// TestVirtualDisplayLayerStack checks the selected display's layer stack is
// carried into the Android 10 transaction.
func TestVirtualDisplayLayerStack(t *testing.T) {
	r := &virtualDisplayRemote{}
	_, err := CreateVirtualDisplayOnLayerStack(context.Background(), r, nil, &virtualDisplayTransport{}, 29, binder.NewStubBinder(nil), 1080, 2400, 360, 800, 7)
	require.NoError(t, err)
	p := r.requests[1]
	_, err = p.ReadInterfaceToken()
	require.NoError(t, err)
	_, err = p.ReadInt32()
	require.NoError(t, err)
	_, err = p.ReadInt32()
	require.NoError(t, err)
	_, _, err = p.ReadNullableStrongBinderLegacy()
	require.NoError(t, err)
	_, _, err = p.ReadNullableStrongBinderLegacy()
	require.NoError(t, err)
	flags, err := p.ReadInt32()
	require.NoError(t, err)
	require.EqualValues(t, 7, flags)
	stack, err := p.ReadInt32()
	require.NoError(t, err)
	require.EqualValues(t, 7, stack)
}

// TestVirtualDisplaySecureOption verifies the create-display wire field on both APIs.
//
// Example: secure=true is encoded on Android 10 and Android 14.
func TestVirtualDisplaySecureOption(t *testing.T) {
	for _, api := range []int{29, 34} {
		for _, secure := range []bool{false, true} {
			t.Run(fmt.Sprintf("api%d_secure%t", api, secure),
				// Check the selected secure value in this API's create request.
				//
				// Example: API 29 encodes true as a nonzero integer.
				func(t *testing.T) {
					raw := &virtualDisplayRemote{}
					aidl := &virtualDisplayRemote{aidl: true}
					_, err := CreateVirtualDisplayOnLayerStackWithOptions(context.Background(), raw, aidl, &virtualDisplayTransport{}, api, binder.NewStubBinder(nil), 1080, 2400, 360, 800, 0, VirtualDisplayOptions{Secure: secure})
					require.NoError(t, err)
					p := raw.requests[0]
					if api == 34 {
						p = aidl.requests[0]
					}
					_, err = p.ReadInterfaceToken()
					require.NoError(t, err)
					if api == 29 {
						_, err = p.ReadString()
					} else {
						_, err = p.ReadString16()
					}
					require.NoError(t, err)
					got, err := p.ReadBool()
					require.NoError(t, err)
					require.Equal(t, secure, got)
				})
		}
	}
}

// TestVirtualDisplayDefaultAndOptions validates default and option forwarding.
//
// Example: the original constructor defaults to a non-secure display.
func TestVirtualDisplayDefaultAndOptions(t *testing.T) {
	for _, api := range []int{29, 34} {
		for _, secure := range []bool{false, true} {
			raw := &virtualDisplayRemote{}
			aidl := &virtualDisplayRemote{aidl: true}
			var err error
			if secure {
				_, err = CreateVirtualDisplayWithOptions(context.Background(), raw, aidl, &virtualDisplayTransport{}, api, binder.NewStubBinder(nil), 1080, 2400, 360, 800, VirtualDisplayOptions{Secure: true})
			} else {
				_, err = CreateVirtualDisplay(context.Background(), raw, aidl, &virtualDisplayTransport{}, api, binder.NewStubBinder(nil), 1080, 2400, 360, 800)
			}
			require.NoError(t, err)
			p := raw.requests[0]
			if api == 34 {
				p = aidl.requests[0]
			}
			_, err = p.ReadInterfaceToken()
			require.NoError(t, err)
			if api == 29 {
				_, err = p.ReadString()
			} else {
				_, err = p.ReadString16()
			}
			require.NoError(t, err)
			got, err := p.ReadBool()
			require.NoError(t, err)
			require.Equal(t, secure, got)
		}
	}
}
