package gui

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"

	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/parcel"
)

const (
	// android10ComposerDescriptor identifies the legacy SurfaceFlinger interface.
	//
	// Example: write it before an Android 10 composer transaction.
	android10ComposerDescriptor = "android.ui.ISurfaceComposer"
	// android10GetPhysicalDisplayToken is the Android 10 token transaction.
	//
	// Example: obtain a token for a physical display ID.
	android10GetPhysicalDisplayToken = 7
	// android10GetDisplayConfigs is the Android 10 display configuration transaction.
	//
	// Example: read the available display configurations.
	android10GetDisplayConfigs = 11
	// android10GetActiveConfig is the Android 10 active configuration transaction.
	//
	// Example: choose a display configuration by index.
	android10GetActiveConfig = 12
	// android10GetPhysicalDisplayIDs is the Android 10 physical ID transaction.
	//
	// Example: enumerate physical displays.
	android10GetPhysicalDisplayIDs = 36
	// maxDisplayEntries bounds array allocation from a composer reply.
	//
	// Example: reject an invalid display mode count.
	maxDisplayEntries = 100000
	// android10API is the Android 10 SDK version.
	//
	// Example: select legacy composer encoding.
	android10API = 29
	// android14API is the Android 14 SDK version.
	//
	// Example: select AIDL composer encoding.
	android14API = 34
	// android10DisplayInfoSize is the native DisplayInfo struct size.
	//
	// Example: read a configuration from a legacy reply.
	android10DisplayInfoSize = 56
	// android10DisplayInfoWidthOffset is the width field offset.
	//
	// Example: decode a native display configuration.
	android10DisplayInfoWidthOffset = 0
	// android10DisplayInfoHeightOffset is the height field offset.
	//
	// Example: decode a native display configuration.
	android10DisplayInfoHeightOffset = 4
	// android10DisplayInfoXDPIOffset is the horizontal DPI field offset.
	//
	// Example: decode a native display configuration.
	android10DisplayInfoXDPIOffset = 8
	// android10DisplayInfoYDPIOffset is the vertical DPI field offset.
	//
	// Example: decode a native display configuration.
	android10DisplayInfoYDPIOffset = 12
	// android10DisplayInfoFPSOffset is the refresh rate field offset.
	//
	// Example: decode a native display configuration.
	android10DisplayInfoFPSOffset = 16
	// android10DisplayInfoDensityOffset is the density field offset.
	//
	// Example: decode a native display configuration.
	android10DisplayInfoDensityOffset = 20
	// android10DisplayInfoRotationOffset is the orientation byte offset.
	//
	// Example: decode a native display configuration.
	android10DisplayInfoRotationOffset = 24
	// android10DisplayInfoSecureOffset is the secure byte offset.
	//
	// Example: decode a native display configuration.
	android10DisplayInfoSecureOffset = 25
)

// PhysicalDisplayInfo describes the active configuration of a physical display.
//
// Example: use Width and Height to size a capture surface.
type PhysicalDisplayInfo struct {
	Width, Height            uint32
	XDPI, YDPI, FPS, Density float32
	Secure                   bool
	Rotation                 Rotation
	LayerStack               *int32
}

// PhysicalDisplayIDs returns the SurfaceFlinger physical display identifiers.
//
// Example: select the first identifier to inspect the primary physical display.
func PhysicalDisplayIDs(
	ctx context.Context,
	remote binder.IBinder,
	api int,
) ([]uint64, error) {
	if remote == nil {
		return nil, fmt.Errorf("missing SurfaceFlinger service")
	}
	if api == android14API {
		ids, err := NewSurfaceComposerProxy(remote).GetPhysicalDisplayIds(ctx)
		if err != nil {
			return nil, err
		}
		result := make([]uint64, len(ids))
		for i, id := range ids {
			result[i] = uint64(id)
		}
		return result, nil
	}
	if api != android10API {
		return nil, fmt.Errorf("unsupported SurfaceFlinger API %d", api)
	}
	p := parcel.New()
	defer p.Recycle()
	p.WriteInterfaceToken(android10ComposerDescriptor)
	r, err := remote.Transact(ctx, android10GetPhysicalDisplayIDs, 0, p)
	if err != nil {
		return nil, err
	}
	defer r.Recycle()
	count, err := r.ReadInt32()
	if err != nil {
		return nil, err
	}
	if count < 0 || count > maxDisplayEntries {
		return nil, fmt.Errorf("invalid display count %d", count)
	}
	ids := make([]uint64, count)
	for i := range ids {
		ids[i], err = r.ReadUint64()
		if err != nil {
			return nil, err
		}
	}
	return ids, nil
}

// GetPhysicalDisplayInfo returns active physical display metadata for API 29 or 34.
//
// Example: query the identifier returned by PhysicalDisplayIDs.
func GetPhysicalDisplayInfo(
	ctx context.Context,
	remote binder.IBinder,
	api int,
	id uint64,
) (PhysicalDisplayInfo, error) {
	var info PhysicalDisplayInfo
	if remote == nil {
		return info, fmt.Errorf("missing SurfaceFlinger service")
	}
	if api == android14API {
		proxy := NewSurfaceComposerProxy(android14DisplayBinder{remote})
		static, err := proxy.GetStaticDisplayInfo(ctx, int64(id))
		if err != nil {
			return info, fmt.Errorf("static display info: %w", err)
		}
		token, err := proxy.GetPhysicalDisplayToken(ctx, int64(id))
		if err != nil {
			return info, fmt.Errorf("display token: %w", err)
		}
		state, err := proxy.GetDisplayState(ctx, token)
		if err != nil {
			return info, fmt.Errorf("display state: %w", err)
		}
		p := parcel.New()
		defer p.Recycle()
		p.WriteInterfaceToken(DescriptorISurfaceComposer)
		p.WriteUint64(id)
		code := binder.TransactionCode(TransactionISurfaceComposerGetDynamicDisplayInfoFromId)
		r, err := remote.Transact(ctx, code, 0, p)
		if err != nil {
			return info, err
		}
		defer r.Recycle()
		if err = binder.ReadStatus(r); err != nil {
			return info, err
		}
		present, err := r.ReadInt32()
		if err != nil {
			return info, err
		}
		if present == 0 {
			return info, fmt.Errorf("null dynamic display info")
		}
		info, err = readAndroid14DisplayModes(r)
		if err != nil {
			return info, err
		}
		info.Density = static.Density
		info.Secure = static.Secure
		info.Rotation = state.Orientation
		info.LayerStack = &state.LayerStack
		return info, nil
	}
	if api != android10API {
		return info, fmt.Errorf("unsupported SurfaceFlinger API %d", api)
	}
	p := parcel.New()
	defer p.Recycle()
	p.WriteInterfaceToken(android10ComposerDescriptor)
	p.WriteUint64(id)
	r, err := remote.Transact(ctx, android10GetPhysicalDisplayToken, 0, p)
	if err != nil {
		return info, err
	}
	token, present, err := r.ReadNullableStrongBinderLegacy()
	r.Recycle()
	if err != nil {
		return info, err
	}
	if !present {
		return info, fmt.Errorf("display %d not found", id)
	}
	p = parcel.New()
	defer p.Recycle()
	p.WriteInterfaceToken(android10ComposerDescriptor)
	p.WriteStrongBinderLegacy(token)
	r, err = remote.Transact(ctx, android10GetActiveConfig, 0, p)
	if err != nil {
		return info, err
	}
	active, err := r.ReadInt32()
	r.Recycle()
	if err != nil {
		return info, err
	}
	if active < 0 {
		return info, fmt.Errorf("get active display config: %d", active)
	}
	p = parcel.New()
	defer p.Recycle()
	p.WriteInterfaceToken(android10ComposerDescriptor)
	p.WriteStrongBinderLegacy(token)
	r, err = remote.Transact(ctx, android10GetDisplayConfigs, 0, p)
	if err != nil {
		return info, err
	}
	defer r.Recycle()
	status, err := r.ReadInt32()
	if err != nil {
		return info, err
	}
	if status != 0 {
		return info, fmt.Errorf("get display configs: %d", status)
	}
	count, err := r.ReadInt32()
	if err != nil {
		return info, err
	}
	if active >= count {
		return info, fmt.Errorf("active config %d outside count %d", active, count)
	}
	for i := int32(0); i <= active; i++ {
		info, err = readAndroid10DisplayInfo(r)
		if err != nil {
			return info, err
		}
	}
	return info, nil
}

// android14DisplayBinder resolves Android 14 display methods from their AIDL order.
//
// Example: query display state when version metadata lacks these methods.
type android14DisplayBinder struct{ binder.IBinder }

// ResolveCode resolves the Android 14 display methods used here.
//
// Example: resolve getStaticDisplayInfo to transaction 12.
func (b android14DisplayBinder) ResolveCode(
	ctx context.Context,
	descriptor string,
	method string,
) (binder.TransactionCode, error) {
	if descriptor == DescriptorISurfaceComposer {
		switch method {
		case MethodISurfaceComposerGetPhysicalDisplayToken:
			return TransactionISurfaceComposerGetPhysicalDisplayToken, nil
		case MethodISurfaceComposerGetDisplayState:
			return TransactionISurfaceComposerGetDisplayState, nil
		case MethodISurfaceComposerGetStaticDisplayInfo:
			return TransactionISurfaceComposerGetStaticDisplayInfo, nil
		}
	}
	return b.IBinder.ResolveCode(ctx, descriptor, method)
}

// readAndroid10DisplayInfo reads the 64-bit native Android 10 DisplayInfo layout.
//
// Example: decode one entry of a getDisplayConfigs reply.
func readAndroid10DisplayInfo(p *parcel.Parcel) (PhysicalDisplayInfo, error) {
	var info PhysicalDisplayInfo
	b, err := p.Read(android10DisplayInfoSize)
	if err != nil {
		return info, err
	}
	info.Width = binary.LittleEndian.Uint32(b[android10DisplayInfoWidthOffset:])
	info.Height = binary.LittleEndian.Uint32(b[android10DisplayInfoHeightOffset:])
	info.XDPI = math.Float32frombits(binary.LittleEndian.Uint32(b[android10DisplayInfoXDPIOffset:]))
	info.YDPI = math.Float32frombits(binary.LittleEndian.Uint32(b[android10DisplayInfoYDPIOffset:]))
	info.FPS = math.Float32frombits(binary.LittleEndian.Uint32(b[android10DisplayInfoFPSOffset:]))
	info.Density = math.Float32frombits(binary.LittleEndian.Uint32(b[android10DisplayInfoDensityOffset:]))
	info.Rotation = Rotation(b[android10DisplayInfoRotationOffset])
	info.Secure = b[android10DisplayInfoSecureOffset] != 0
	return info, nil
}

// readAndroid14DisplayModes selects the active mode from Android 14 dynamic info.
//
// Example: decode resolution and refresh rate from a SurfaceFlinger reply.
func readAndroid14DisplayModes(p *parcel.Parcel) (PhysicalDisplayInfo, error) {
	var zero PhysicalDisplayInfo
	end, err := parcel.ReadParcelableHeader(p)
	if err != nil {
		return zero, err
	}
	defer parcel.SkipToParcelableEnd(p, end)
	count, err := p.ReadInt32()
	if err != nil {
		return zero, err
	}
	if count < 0 || count > maxDisplayEntries {
		return zero, fmt.Errorf("invalid mode count %d", count)
	}
	modes := make(map[int32]PhysicalDisplayInfo, count)
	for i := int32(0); i < count; i++ {
		present, err := p.ReadInt32()
		if err != nil {
			return zero, err
		}
		if present == 0 {
			return zero, fmt.Errorf("null display mode")
		}
		modeEnd, err := parcel.ReadParcelableHeader(p)
		if err != nil {
			return zero, err
		}
		id, err := p.ReadInt32()
		if err != nil {
			return zero, err
		}
		present, err = p.ReadInt32()
		if err != nil {
			return zero, err
		}
		if present == 0 {
			return zero, fmt.Errorf("null display size")
		}
		var size Size
		if err = size.UnmarshalParcel(p); err != nil {
			return zero, err
		}
		var info PhysicalDisplayInfo
		info.Width = uint32(size.Width)
		info.Height = uint32(size.Height)
		info.XDPI, err = p.ReadFloat32()
		if err != nil {
			return zero, err
		}
		info.YDPI, err = p.ReadFloat32()
		if err != nil {
			return zero, err
		}
		n, err := p.ReadInt32()
		if err != nil {
			return zero, err
		}
		if n < -1 || n > maxDisplayEntries {
			return zero, fmt.Errorf("invalid HDR type count %d", n)
		}
		for j := int32(0); j < n; j++ {
			if _, err = p.ReadInt32(); err != nil {
				return zero, err
			}
		}
		info.FPS, err = p.ReadFloat32()
		if err != nil {
			return zero, err
		}
		parcel.SkipToParcelableEnd(p, modeEnd)
		modes[id] = info
	}
	active, err := p.ReadInt32()
	if err != nil {
		return zero, err
	}
	info, ok := modes[active]
	if !ok {
		return zero, fmt.Errorf("active display mode %d not found", active)
	}
	return info, nil
}
