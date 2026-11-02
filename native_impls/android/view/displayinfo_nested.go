package view

import (
	"fmt"
	"github.com/AndroidGoLab/binder/parcel"
)

// DisplayAddress selects Physical or Network using its Android parcelable class name.
type DisplayAddress struct {
	Class      string
	PhysicalId int64
	MacAddress string
}
type DisplayInfoMode struct {
	Id, Width, Height       int32
	RefreshRate, VsyncRate  float32
	AlternativeRefreshRates []float32
	SupportedHdrTypes       []int32
}
type DisplayHdrCapabilities struct {
	SupportedHdrTypes                               []int32
	MaxLuminance, MaxAverageLuminance, MinLuminance float32
}
type DisplayRect struct{ Left, Top, Right, Bottom int32 }
type DisplayCutout struct {
	Variant                                                                  int32
	SafeInsets                                                               *DisplayRect
	Bounds                                                                   []*DisplayRect
	WaterfallInsets                                                          *DisplayRect
	DisplayWidth, DisplayHeight, PhysicalDisplayWidth, PhysicalDisplayHeight int32
	Density                                                                  float32
	CutoutSpec                                                               string
	Rotation                                                                 int32
	Scale, PhysicalPixelDisplaySizeRatio                                     float32
}
type DisplayRoundedCorner struct{ Position, Radius, CenterX, CenterY int32 }
type DisplayRoundedCorners struct {
	Variant int32
	Corners []*DisplayRoundedCorner
}
type DisplayInfoShape struct {
	Spec                          string
	Width, Height                 int32
	PhysicalPixelDisplaySizeRatio float32
	Rotation, OffsetX, OffsetY    int32
	Scale                         float32
}
type DisplayRefreshRateRange struct{ Min, Max float32 }
type DisplayThermalRefreshRate struct {
	Status int32
	Range  *DisplayRefreshRateRange
}
type DeviceProductInfo struct {
	Name                 *string
	ManufacturerPnpId    string
	ProductId            *string
	ModelYear            *int32
	ManufactureDate      *DisplayManufactureDate
	ConnectionToSinkType int32
}
type DisplayManufactureDate struct{ Week, Year *int32 }

func (c *displayCodec) address(a **DisplayAddress) {
	name := ""
	if *a != nil {
		name = (*a).Class
	}
	c.class(&name)
	if c.err != nil {
		return
	}
	if name == "" {
		if c.read {
			*a = nil
		}
		return
	}
	if c.read {
		*a = &DisplayAddress{Class: name}
	}
	switch name {
	case "android.view.DisplayAddress$Physical":
		c.l(&(*a).PhysicalId)
	case "android.view.DisplayAddress$Network":
		c.s(&(*a).MacAddress, false)
	default:
		c.err = fmt.Errorf("unknown display address class %q", name)
	}
}
func (c *displayCodec) mode(m *DisplayInfoMode) {
	c.i(&m.Id)
	c.i(&m.Width)
	c.i(&m.Height)
	c.f(&m.RefreshRate)
	if c.api == 34 {
		if c.profile == parcel.DisplayInfoWireProfileAndroid14Vsync {
			c.f(&m.VsyncRate)
		}
		c.floats(&m.AlternativeRefreshRates)
		c.ints(&m.SupportedHdrTypes, true)
	}
}
func (c *displayCodec) hdr(h **DisplayHdrCapabilities) {
	name := ""
	if *h != nil {
		name = "android.view.Display$HdrCapabilities"
	}
	c.class(&name)
	if c.err != nil {
		return
	}
	if name == "" {
		if c.read {
			*h = nil
		}
		return
	}
	if name != "android.view.Display$HdrCapabilities" {
		c.err = fmt.Errorf("unknown HDR capabilities class %q", name)
		return
	}
	if c.read {
		*h = &DisplayHdrCapabilities{}
	}
	c.ints(&(*h).SupportedHdrTypes, false)
	c.f(&(*h).MaxLuminance)
	c.f(&(*h).MaxAverageLuminance)
	c.f(&(*h).MinLuminance)
}
func (c *displayCodec) rect(r **DisplayRect) {
	yes := *r != nil
	c.flag(&yes)
	if c.err != nil {
		return
	}
	if !yes {
		if c.read {
			*r = nil
		}
		return
	}
	if c.read {
		*r = &DisplayRect{}
	}
	c.i(&(*r).Left)
	c.i(&(*r).Top)
	c.i(&(*r).Right)
	c.i(&(*r).Bottom)
}
func (c *displayCodec) cutout(d **DisplayCutout) {
	v := int32(-1)
	if *d != nil {
		v = (*d).Variant
	}
	c.i(&v)
	if c.err != nil {
		return
	}
	if v == -1 {
		if c.read {
			*d = nil
		}
		return
	}
	if v != 0 && v != 1 {
		c.err = fmt.Errorf("unknown cutout variant %d", v)
		return
	}
	if c.read {
		*d = &DisplayCutout{Variant: v}
	}
	if v == 0 {
		return
	}
	x := *d
	c.rect(&x.SafeInsets)
	n := len(x.Bounds)
	c.count(&n, false)
	if c.err != nil {
		return
	}
	if n != 4 {
		c.err = fmt.Errorf("cutout bounds count %d", n)
		return
	}
	if c.read {
		x.Bounds = make([]*DisplayRect, n)
	}
	for i := range x.Bounds {
		c.rect(&x.Bounds[i])
	}
	if c.api == 34 {
		c.rect(&x.WaterfallInsets)
		c.i(&x.DisplayWidth)
		c.i(&x.DisplayHeight)
		c.i(&x.PhysicalDisplayWidth)
		c.i(&x.PhysicalDisplayHeight)
		c.f(&x.Density)
		c.s(&x.CutoutSpec, false)
		c.i(&x.Rotation)
		c.f(&x.Scale)
		c.f(&x.PhysicalPixelDisplaySizeRatio)
	}
}
func (c *displayCodec) corners(d **DisplayRoundedCorners) {
	yes := *d != nil
	c.flag(&yes)
	if c.err != nil {
		return
	}
	if !yes {
		if c.read {
			*d = nil
		}
		return
	}
	if c.read {
		*d = &DisplayRoundedCorners{}
	}
	x := *d
	c.i(&x.Variant)
	if c.err != nil {
		return
	}
	if x.Variant == 0 {
		return
	}
	if x.Variant != 1 {
		c.err = fmt.Errorf("unknown rounded corners variant %d", x.Variant)
		return
	}
	n := len(x.Corners)
	c.count(&n, false)
	if c.err != nil {
		return
	}
	if n != 4 {
		c.err = fmt.Errorf("rounded corner count %d", n)
		return
	}
	if c.read {
		x.Corners = make([]*DisplayRoundedCorner, n)
	}
	for i := range x.Corners {
		yes = x.Corners[i] != nil
		c.flag(&yes)
		if c.read && yes {
			x.Corners[i] = &DisplayRoundedCorner{}
		}
		if yes && c.err == nil {
			r := x.Corners[i]
			c.i(&r.Position)
			c.i(&r.Radius)
			c.i(&r.CenterX)
			c.i(&r.CenterY)
		}
	}
}
func (c *displayCodec) shape(d **DisplayInfoShape) {
	yes := *d != nil
	c.flag(&yes)
	if c.err != nil {
		return
	}
	if !yes {
		if c.read {
			*d = nil
		}
		return
	}
	if c.read {
		*d = &DisplayInfoShape{}
	}
	x := *d
	c.s(&x.Spec, true)
	c.i(&x.Width)
	c.i(&x.Height)
	c.f(&x.PhysicalPixelDisplaySizeRatio)
	c.i(&x.Rotation)
	c.i(&x.OffsetX)
	c.i(&x.OffsetY)
	c.f(&x.Scale)
}
func (c *displayCodec) rangeObject(d **DisplayRefreshRateRange) {
	yes := *d != nil
	c.flag(&yes)
	if c.err != nil {
		return
	}
	if !yes {
		if c.read {
			*d = nil
		}
		return
	}
	if c.read {
		*d = &DisplayRefreshRateRange{}
	}
	c.f(&(*d).Min)
	c.f(&(*d).Max)
}

func (c *displayCodec) valueString(v **string) {
	typ := int32(-1)
	if *v != nil {
		typ = 0
	}
	c.i(&typ)
	if c.err != nil {
		return
	}
	switch typ {
	case -1:
		if c.read {
			*v = nil
		}
	case 0:
		if c.read {
			*v = new(string)
		}
		c.s(*v, false)
	default:
		c.err = fmt.Errorf("unexpected string value type %d", typ)
	}
}
func (c *displayCodec) valueInt(v **int32) {
	typ := int32(-1)
	if *v != nil {
		typ = 1
	}
	c.i(&typ)
	if c.err != nil {
		return
	}
	switch typ {
	case -1:
		if c.read {
			*v = nil
		}
	case 1:
		if c.read {
			*v = new(int32)
		}
		c.i(*v)
	default:
		c.err = fmt.Errorf("unexpected integer value type %d", typ)
	}
}
func (c *displayCodec) valueParcelable(present bool, name string, payload func(*displayCodec)) bool {
	typ := int32(-1)
	if present {
		typ = 4
	}
	c.i(&typ)
	if c.err != nil {
		return false
	}
	if typ == -1 {
		return false
	}
	if typ != 4 {
		c.err = fmt.Errorf("unexpected parcelable value type %d", typ)
		return false
	}
	if c.read {
		var length int32
		c.i(&length)
		if c.err != nil {
			return false
		}
		start := c.p.Position()
		if length < 0 || int(length) > c.p.Len()-start {
			c.err = fmt.Errorf("invalid parcelable value length %d", length)
			return false
		}
		var got string
		c.class(&got)
		if got != name {
			c.err = fmt.Errorf("unexpected parcelable class %q", got)
			return false
		}
		payload(c)
		if c.err == nil && c.p.Position() != start+int(length) {
			c.err = fmt.Errorf("parcelable value length mismatch")
		}
		return true
	}
	temp := parcel.New()
	temp.SetTargetAPILevel(c.api)
	sub := displayCodec{p: temp, api: c.api}
	sub.class(&name)
	payload(&sub)
	if sub.err != nil {
		c.err = sub.err
		return false
	}
	length := int32(temp.Len())
	c.i(&length)
	c.p.WriteRawBytes(temp.Data())
	return true
}
func (c *displayCodec) product(p **DeviceProductInfo) {
	name := ""
	if *p != nil {
		name = "android.hardware.display.DeviceProductInfo"
	}
	c.class(&name)
	if c.err != nil {
		return
	}
	if name == "" {
		if c.read {
			*p = nil
		}
		return
	}
	if name != "android.hardware.display.DeviceProductInfo" {
		c.err = fmt.Errorf("unknown device product info class %q", name)
		return
	}
	if c.read {
		*p = &DeviceProductInfo{}
	}
	x := *p
	c.nullableString(&x.Name, false)
	c.s(&x.ManufacturerPnpId, false)
	c.valueString(&x.ProductId)
	c.valueInt(&x.ModelYear)
	present := x.ManufactureDate != nil
	if c.read {
		x.ManufactureDate = &DisplayManufactureDate{}
	}
	if !c.valueParcelable(present, "android.hardware.display.DeviceProductInfo$ManufactureDate", func(v *displayCodec) { v.valueInt(&x.ManufactureDate.Week); v.valueInt(&x.ManufactureDate.Year) }) && c.read {
		x.ManufactureDate = nil
	}
	c.i(&x.ConnectionToSinkType)
}
func (c *displayCodec) thermal(a *[]DisplayThermalRefreshRate) {
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
		*a = make([]DisplayThermalRefreshRate, n)
	}
	for i := range *a {
		x := &(*a)[i]
		c.i(&x.Status)
		present := x.Range != nil
		if c.read {
			x.Range = &DisplayRefreshRateRange{}
		}
		if !c.valueParcelable(present, "android.view.SurfaceControl$RefreshRateRange", func(v *displayCodec) { v.f(&x.Range.Min); v.f(&x.Range.Max) }) && c.read {
			x.Range = nil
		}
	}
}
