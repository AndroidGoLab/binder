package view

import (
	"fmt"
	"github.com/AndroidGoLab/binder/parcel"
)

// DisplayInfo is the Java android.view.DisplayInfo parcel layout for API 29 and 34.
type DisplayInfo struct {
	LayerStack, Flags, Type, DisplayId, DisplayGroupId                                                                                                   int32
	Address                                                                                                                                              *DisplayAddress
	DeviceProductInfo                                                                                                                                    *DeviceProductInfo
	Name                                                                                                                                                 string
	AppWidth, AppHeight, SmallestNominalAppWidth, SmallestNominalAppHeight, LargestNominalAppWidth, LargestNominalAppHeight, LogicalWidth, LogicalHeight int32
	OverscanLeft, OverscanTop, OverscanRight, OverscanBottom                                                                                             int32
	DisplayCutout                                                                                                                                        *DisplayCutout
	Rotation, ModeId                                                                                                                                     int32
	RenderFrameRate                                                                                                                                      float32
	DefaultModeId, UserPreferredModeId                                                                                                                   int32
	SupportedModes                                                                                                                                       []DisplayInfoMode
	ColorMode                                                                                                                                            int32
	SupportedColorModes                                                                                                                                  []int32
	HdrCapabilities                                                                                                                                      *DisplayHdrCapabilities
	MinimalPostProcessingSupported                                                                                                                       bool
	LogicalDensityDpi                                                                                                                                    int32
	PhysicalXDpi, PhysicalYDpi                                                                                                                           float32
	AppVsyncOffsetNanos, PresentationDeadlineNanos                                                                                                       int64
	State, CommittedState, OwnerUid                                                                                                                      int32
	OwnerPackageName                                                                                                                                     *string
	UniqueId                                                                                                                                             string
	RemoveMode                                                                                                                                           int32
	RefreshRateOverride, BrightnessMinimum, BrightnessMaximum, BrightnessDefault                                                                         float32
	RoundedCorners                                                                                                                                       *DisplayRoundedCorners
	UserDisabledHdrTypes                                                                                                                                 []int32
	InstallOrientation                                                                                                                                   int32
	DisplayShape                                                                                                                                         *DisplayInfoShape
	LayoutLimitedRefreshRate                                                                                                                             *DisplayRefreshRateRange
	HdrSdrRatio                                                                                                                                          float32
	ThermalRefreshRateThrottling                                                                                                                         []DisplayThermalRefreshRate
	ThermalBrightnessThrottlingDataId                                                                                                                    *string
}

type displayCodec struct {
	p       *parcel.Parcel
	read    bool
	err     error
	api     int
	profile parcel.DisplayInfoWireProfile
}

func (c *displayCodec) i(v *int32) {
	if c.err != nil {
		return
	}
	if c.read {
		*v, c.err = c.p.ReadInt32()
	} else {
		c.p.WriteInt32(*v)
	}
}
func (c *displayCodec) l(v *int64) {
	if c.err != nil {
		return
	}
	if c.read {
		*v, c.err = c.p.ReadInt64()
	} else {
		c.p.WriteInt64(*v)
	}
}
func (c *displayCodec) f(v *float32) {
	if c.err != nil {
		return
	}
	if c.read {
		*v, c.err = c.p.ReadFloat32()
	} else {
		c.p.WriteFloat32(*v)
	}
}
func (c *displayCodec) s(v *string, utf8 bool) {
	if c.err != nil {
		return
	}
	if c.read {
		if utf8 {
			*v, c.err = c.p.ReadString()
		} else {
			*v, c.err = c.p.ReadString16()
		}
	} else if utf8 {
		c.p.WriteString(*v)
	} else {
		c.p.WriteString16(*v)
	}
}
func (c *displayCodec) nullableString(v **string, utf8 bool) {
	if c.err != nil {
		return
	}
	if c.read {
		if utf8 {
			*v, c.err = c.p.ReadNullableString()
		} else {
			*v, c.err = c.p.ReadNullableString16()
		}
	} else if utf8 {
		c.p.WriteNullableString(*v)
	} else {
		c.p.WriteNullableString16(*v)
	}
}
func (c *displayCodec) count(n *int, nullable bool) {
	v := int32(*n)
	if !c.read && nullable && *n < 0 {
		v = -1
	}
	c.i(&v)
	if c.err != nil {
		return
	}
	if v < 0 && !(nullable && v == -1) || v > 262144 {
		c.err = fmt.Errorf("invalid array length %d", v)
		return
	}
	*n = int(v)
}
func (c *displayCodec) ints(a *[]int32, nullable bool) {
	n := len(*a)
	if !c.read && nullable && *a == nil {
		n = -1
	}
	c.count(&n, nullable)
	if c.err != nil {
		return
	}
	if c.read {
		if n < 0 {
			*a = nil
			return
		}
		*a = make([]int32, n)
	}
	for i := range *a {
		c.i(&(*a)[i])
	}
}
func (c *displayCodec) floats(a *[]float32) {
	n := len(*a)
	if !c.read && *a == nil {
		n = -1
	}
	c.count(&n, true)
	if c.err != nil {
		return
	}
	if c.read {
		if n < 0 {
			*a = nil
			return
		}
		*a = make([]float32, n)
	}
	for i := range *a {
		c.f(&(*a)[i])
	}
}
func (c *displayCodec) flag(present *bool) {
	v := int32(0)
	if *present {
		v = 1
	}
	c.i(&v)
	if c.read {
		*present = v != 0
	}
}
func (c *displayCodec) class(name *string) {
	if c.err != nil {
		return
	}
	if c.read {
		var s *string
		s, c.err = c.p.ReadNullableString16()
		if s == nil {
			*name = ""
		} else {
			*name = *s
		}
	} else if *name == "" {
		c.p.WriteNullString16()
	} else {
		c.p.WriteString16(*name)
	}
}
func (s *DisplayInfo) MarshalParcel(p *parcel.Parcel) error   { return s.codec(p, false) }
func (s *DisplayInfo) UnmarshalParcel(p *parcel.Parcel) error { return s.codec(p, true) }
func (s *DisplayInfo) codec(p *parcel.Parcel, read bool) error {
	c := displayCodec{p: p, read: read, api: p.TargetAPILevel(), profile: p.DisplayInfoWireProfile()}
	if !(c.api == 29 && c.profile == parcel.DisplayInfoWireProfileAndroid10 || c.api == 34 && (c.profile == parcel.DisplayInfoWireProfileAndroid14Initial || c.profile == parcel.DisplayInfoWireProfileAndroid14Vsync)) {
		return fmt.Errorf("DisplayInfo: unsupported Android API %d and wire profile %d", c.api, c.profile)
	}
	c.i(&s.LayerStack)
	c.i(&s.Flags)
	c.i(&s.Type)
	c.i(&s.DisplayId)
	if c.api == 34 {
		c.i(&s.DisplayGroupId)
	}
	c.address(&s.Address)
	if c.api == 34 {
		c.product(&s.DeviceProductInfo)
	}
	c.s(&s.Name, c.api == 34)
	for _, v := range []*int32{&s.AppWidth, &s.AppHeight, &s.SmallestNominalAppWidth, &s.SmallestNominalAppHeight, &s.LargestNominalAppWidth, &s.LargestNominalAppHeight, &s.LogicalWidth, &s.LogicalHeight} {
		c.i(v)
	}
	if c.api == 29 {
		for _, v := range []*int32{&s.OverscanLeft, &s.OverscanTop, &s.OverscanRight, &s.OverscanBottom} {
			c.i(v)
		}
	}
	c.cutout(&s.DisplayCutout)
	c.i(&s.Rotation)
	c.i(&s.ModeId)
	if c.api == 34 {
		c.f(&s.RenderFrameRate)
	}
	c.i(&s.DefaultModeId)
	if c.profile == parcel.DisplayInfoWireProfileAndroid14Vsync {
		c.i(&s.UserPreferredModeId)
	} else if c.read {
		s.UserPreferredModeId = 0
	}
	n := len(s.SupportedModes)
	c.count(&n, false)
	if c.err == nil && c.read {
		s.SupportedModes = make([]DisplayInfoMode, n)
	}
	for i := range s.SupportedModes {
		c.mode(&s.SupportedModes[i])
	}
	c.i(&s.ColorMode)
	c.ints(&s.SupportedColorModes, false)
	c.hdr(&s.HdrCapabilities)
	if c.api == 34 {
		c.flag(&s.MinimalPostProcessingSupported)
	}
	c.i(&s.LogicalDensityDpi)
	c.f(&s.PhysicalXDpi)
	c.f(&s.PhysicalYDpi)
	c.l(&s.AppVsyncOffsetNanos)
	c.l(&s.PresentationDeadlineNanos)
	c.i(&s.State)
	if c.api == 34 {
		c.i(&s.CommittedState)
	}
	c.i(&s.OwnerUid)
	c.nullableString(&s.OwnerPackageName, c.api == 34)
	c.s(&s.UniqueId, c.api == 34)
	c.i(&s.RemoveMode)
	if c.api == 34 {
		c.f(&s.RefreshRateOverride)
		c.f(&s.BrightnessMinimum)
		c.f(&s.BrightnessMaximum)
		c.f(&s.BrightnessDefault)
		c.corners(&s.RoundedCorners)
		c.ints(&s.UserDisabledHdrTypes, false)
		c.i(&s.InstallOrientation)
		c.shape(&s.DisplayShape)
		c.rangeObject(&s.LayoutLimitedRefreshRate)
		c.f(&s.HdrSdrRatio)
		c.thermal(&s.ThermalRefreshRateThrottling)
		c.nullableString(&s.ThermalBrightnessThrottlingDataId, true)
	}
	if c.err != nil {
		return fmt.Errorf("DisplayInfo: %w", c.err)
	}
	return nil
}
