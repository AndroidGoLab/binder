package gui

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/parcel"
)

// writeAndroid10DisplayGeometry writes the virtual display state after its
// token and surface, following Android 10 DisplayState::write.
//
// Example: project a 1080x2400 display into a 360x800 surface.
func writeAndroid10DisplayGeometry(
	p *parcel.Parcel,
	sourceWidth, sourceHeight, width, height int32,
) {
	writeAndroid10DisplayGeometryOnStack(p, sourceWidth, sourceHeight, width, height, 0)
}

// writeAndroid10DisplayGeometryOnStack encodes a selected layer stack.
//
// Example: project a secondary display's composition.
func writeAndroid10DisplayGeometryOnStack(p *parcel.Parcel, sourceWidth, sourceHeight, width, height, stack int32) {
	for _, value := range []int32{7, stack, 0, 0, 0, sourceWidth, sourceHeight, 0, 0, width, height, width, height} {
		p.WriteInt32(value)
	}
}

// VirtualDisplay owns a SurfaceFlinger virtual display token.
//
// Example: close it after finishing capture to stop composition.
type VirtualDisplay struct {
	remote binder.IBinder
	api    int
	token  uint32
}

// VirtualDisplayOptions controls virtual display creation.
//
// Example: request secure composition with VirtualDisplayOptions{Secure: true}.
type VirtualDisplayOptions struct {
	// Secure requests secure composition, which may require elevated privileges.
	Secure bool
}

// CreateVirtualDisplay creates a projected virtual display using the supplied producer.
//
// Example: project a physical screen into an RGBA buffer producer.
func CreateVirtualDisplay(
	ctx context.Context,
	raw binder.IBinder,
	aidl binder.IBinder,
	transport binder.Transport,
	api int,
	producer *binder.StubBinder,
	sourceWidth, sourceHeight, width, height int32,
) (*VirtualDisplay, error) {
	return CreateVirtualDisplayOnLayerStack(ctx, raw, aidl, transport, api, producer, sourceWidth, sourceHeight, width, height, 0)
}

// CreateVirtualDisplayOnLayerStack projects a selected composition layer stack.
//
// Example: capture a secondary physical display with a known layer stack.
func CreateVirtualDisplayOnLayerStack(
	ctx context.Context,
	raw binder.IBinder,
	aidl binder.IBinder,
	transport binder.Transport,
	api int,
	producer *binder.StubBinder,
	sourceWidth, sourceHeight, width, height, stack int32,
) (*VirtualDisplay, error) {
	return CreateVirtualDisplayOnLayerStackWithOptions(ctx, raw, aidl, transport, api, producer, sourceWidth, sourceHeight, width, height, stack, VirtualDisplayOptions{})
}

// CreateVirtualDisplayWithOptions creates a projected display on layer stack zero.
//
// Example: request a secure virtual display using VirtualDisplayOptions{Secure: true}.
func CreateVirtualDisplayWithOptions(ctx context.Context, raw, aidl binder.IBinder, transport binder.Transport, api int, producer *binder.StubBinder, sourceWidth, sourceHeight, width, height int32, options VirtualDisplayOptions) (*VirtualDisplay, error) {
	return CreateVirtualDisplayOnLayerStackWithOptions(ctx, raw, aidl, transport, api, producer, sourceWidth, sourceHeight, width, height, 0, options)
}

// CreateVirtualDisplayOnLayerStackWithOptions projects a selected layer stack.
//
// Example: request secure composition for a selected display's layer stack.
func CreateVirtualDisplayOnLayerStackWithOptions(ctx context.Context, raw, aidl binder.IBinder, transport binder.Transport, api int, producer *binder.StubBinder, sourceWidth, sourceHeight, width, height, stack int32, options VirtualDisplayOptions) (*VirtualDisplay, error) {
	if sourceWidth <= 0 || sourceHeight <= 0 || width <= 0 || height <= 0 {
		return nil, fmt.Errorf("display dimensions must be positive")
	}
	if api != 29 && api != 34 {
		return nil, fmt.Errorf("unsupported SurfaceFlinger API %d", api)
	}
	if raw == nil || transport == nil || producer == nil || (api == 34 && aidl == nil) {
		return nil, fmt.Errorf("missing display service or producer")
	}
	remote := raw
	if api == 34 {
		remote = aidl
	}
	p := parcel.New()
	defer p.Recycle()
	if api == 29 {
		p.WriteInterfaceToken("android.ui.ISurfaceComposer")
	} else {
		p.WriteInterfaceToken("android.gui.ISurfaceComposer")
	}
	if api == 29 {
		p.WriteString("binder-go-capture")
		p.WriteBool(options.Secure)
	} else {
		p.WriteString16("binder-go-capture")
		p.WriteBool(options.Secure)
		p.WriteFloat32(0)
	}
	code := binder.TransactionCode(5)
	if api == 34 {
		code = 4
	}
	r, err := remote.Transact(ctx, code, 0, p)
	if err != nil {
		return nil, fmt.Errorf("create display: %w", err)
	}
	defer r.Recycle()
	if api == 34 {
		if err := binder.ReadStatus(r); err != nil {
			return nil, fmt.Errorf("create display: %w", err)
		}
	}
	var token uint32
	if api == 29 {
		var present bool
		token, present, err = r.ReadNullableStrongBinderLegacy()
		if err == nil && !present {
			err = fmt.Errorf("null display token")
		}
	} else {
		token, err = r.ReadStrongBinder()
	}
	if err != nil {
		return nil, fmt.Errorf("read display token: %w", err)
	}
	d := &VirtualDisplay{remote: remote, api: api, token: token}
	if err := d.configure(ctx, raw, transport, producer, sourceWidth, sourceHeight, width, height, stack); err != nil {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return nil, errors.Join(err, d.Close(cleanupCtx))
	}
	return d, nil
}

// configure submits the display surface and projection state.
//
// Example: connect a buffer producer to the virtual display.
func (d *VirtualDisplay) configure(ctx context.Context, raw binder.IBinder, transport binder.Transport, producer *binder.StubBinder, sourceWidth, sourceHeight, width, height, stack int32) error {
	p := parcel.New()
	defer p.Recycle()
	if d.api == 29 {
		p.WriteInterfaceToken("android.ui.ISurfaceComposer")
		p.WriteInt32(0)
		p.WriteInt32(1)
		p.WriteStrongBinderLegacy(d.token)
		cookie := producer.Cookie()
		if cookie == 0 {
			cookie = producer.RegisterWithTransport(ctx, transport)
		}
		p.WriteLocalBinderLegacy(producer.BinderPtr(), cookie)
		writeAndroid10DisplayGeometryOnStack(p, sourceWidth, sourceHeight, width, height, stack)
		p.WriteInt32(0)
		p.WriteNullStrongBinderLegacy()
		p.WriteInt32(0)
		p.WriteBool(false)
		p.WriteInt64(0)
		p.WriteNullWeakBinderLegacy()
		p.WriteUint64(0)
		p.WriteInt32(0)
	} else {
		p.WriteInterfaceToken("android.ui.ISurfaceComposer")
		head := parcel.WriteParcelableHeader(p)
		p.WriteInt64(-1)
		p.WriteInt32(0)
		p.WriteInt64(0)
		p.WriteBool(false)
		parcel.WriteParcelableFooter(p, head)
		p.WriteInt32(0)
		p.WriteInt32(1)
		p.WriteStrongBinder(d.token)
		binder.WriteBinderToParcel(ctx, p, producer, transport)
		writeAndroid14DisplayGeometryOnStack(p, sourceWidth, sourceHeight, width, height, stack)
		p.WriteInt32(0)
		binder.WriteBinderToParcel(ctx, p, producer, transport)
		p.WriteInt32(0)
		p.WriteInt32(0)
		p.WriteInt64(0)
		p.WriteBool(true)
		p.WriteInt32(0)
		p.WriteBool(false)
		p.WriteInt32(0)
		p.WriteUint64(1)
		p.WriteInt32(0)
	}
	r, err := raw.Transact(ctx, 8, 0, p)
	if err != nil {
		return fmt.Errorf("configure display: %w", err)
	}
	if r != nil {
		r.Recycle()
	}
	return nil
}

// Close destroys the virtual display.
//
// Example: stop capture before releasing the producer's buffers.
func (d *VirtualDisplay) Close(ctx context.Context) error {
	p := parcel.New()
	defer p.Recycle()
	if d.api == 29 {
		p.WriteInterfaceToken("android.ui.ISurfaceComposer")
	} else {
		p.WriteInterfaceToken("android.gui.ISurfaceComposer")
	}
	if d.api == 29 {
		p.WriteStrongBinderLegacy(d.token)
	} else {
		p.WriteStrongBinder(d.token)
	}
	code := binder.TransactionCode(6)
	if d.api == 34 {
		code = 5
	}
	r, err := d.remote.Transact(ctx, code, 0, p)
	if err != nil {
		return fmt.Errorf("destroy display: %w", err)
	}
	if r != nil {
		defer r.Recycle()
		if d.api == 34 {
			return binder.ReadStatus(r)
		}
	}
	return nil
}

// writeAndroid14DisplayGeometry writes the Android 14 display state fields.
//
// Example: project a physical display into a smaller virtual display.
func writeAndroid14DisplayGeometry(p *parcel.Parcel, sourceWidth, sourceHeight, width, height int32) {
	writeAndroid14DisplayGeometryOnStack(p, sourceWidth, sourceHeight, width, height, 0)
}

// writeAndroid14DisplayGeometryOnStack writes a selected layer stack.
//
// Example: project a secondary display's composition.
func writeAndroid14DisplayGeometryOnStack(p *parcel.Parcel, sourceWidth, sourceHeight, width, height, stack int32) {
	for _, value := range []int32{7, 0, stack, 0, 0, 0, sourceWidth, sourceHeight, 0, 0, width, height, width, height} {
		p.WriteInt32(value)
	}
}
