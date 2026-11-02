package view

import (
	"bytes"
	"fmt"
	"github.com/AndroidGoLab/binder/parcel"
	"testing"
)

// The fixture follows DisplayInfo.writeToParcel in Android 10 and 14.
func TestDisplayInfoWireLayout(t *testing.T) {
	for _, api := range []int{29, 34} {
		t.Run(string(rune(api)), func(t *testing.T) {
			p := parcel.New()
			p.SetTargetAPILevel(api)
			for _, v := range []int32{1, 2, 3, 4} {
				p.WriteInt32(v)
			}
			if api == 34 {
				p.WriteInt32(5)
			}
			p.WriteNullString16()
			if api == 34 {
				p.WriteNullString16()
				p.WriteString("panel")
			} else {
				p.WriteString16("panel")
			}
			for _, v := range []int32{100, 200, 101, 201, 102, 202, 103, 203} {
				p.WriteInt32(v)
			}
			if api == 29 {
				for i := 0; i < 4; i++ {
					p.WriteInt32(int32(i + 7))
				}
			}
			p.WriteInt32(-1)
			p.WriteInt32(2)
			p.WriteInt32(8)
			if api == 34 {
				p.WriteFloat32(60)
			}
			p.WriteInt32(9)
			if api == 34 {
				p.WriteInt32(10)
			}
			p.WriteInt32(0)
			p.WriteInt32(11)
			p.WriteInt32(2)
			p.WriteInt32(12)
			p.WriteInt32(13)
			p.WriteNullString16()
			if api == 34 {
				p.WriteBool(true)
			}
			p.WriteInt32(420)
			p.WriteFloat32(421)
			p.WriteFloat32(422)
			p.WriteInt64(23)
			p.WriteInt64(24)
			p.WriteInt32(25)
			if api == 34 {
				p.WriteInt32(26)
			}
			p.WriteInt32(27)
			if api == 34 {
				p.WriteString("owner")
				p.WriteString("id")
			} else {
				p.WriteString16("owner")
				p.WriteString16("id")
			}
			p.WriteInt32(28)
			if api == 34 {
				for _, v := range []float32{29, 30, 31, 32} {
					p.WriteFloat32(v)
				}
				p.WriteInt32(0)
				p.WriteInt32(0)
				p.WriteInt32(33)
				p.WriteInt32(0)
				p.WriteInt32(0)
				p.WriteFloat32(34)
				p.WriteInt32(-1)
				p.WriteString("thermal")
			}
			p.WriteInt32(0x12345678)
			p.SetPosition(0)
			var d DisplayInfo
			if err := d.UnmarshalParcel(p); err != nil {
				t.Fatal(err)
			}
			if d.LogicalWidth != 103 || d.Rotation != 2 || d.LogicalDensityDpi != 420 || d.OwnerPackageName != "owner" || d.RemoveMode != 28 {
				t.Fatalf("bad fields: %+v", d)
			}
			if api == 34 && (d.HdrSdrRatio != 34 || d.ThermalBrightnessThrottlingDataId != "thermal") {
				t.Fatalf("bad tail: %+v", d)
			}
			if v, err := p.ReadInt32(); err != nil || v != 0x12345678 {
				t.Fatalf("sentinel %x, %v", v, err)
			}
		})
	}
}

func TestDisplayInfoNestedRoundTripAndTruncation(t *testing.T) {
	for _, api := range []int{29, 34} {
		t.Run(fmt.Sprint(api), func(t *testing.T) {
			v := DisplayInfo{Name: "display", Address: &DisplayAddress{Class: "android.view.DisplayAddress$Physical", PhysicalId: 123}, DisplayCutout: &DisplayCutout{Variant: 1, SafeInsets: &DisplayRect{Left: 4}, Bounds: []*DisplayRect{{Left: 1}, nil, {Right: 2}, {Bottom: 3}}, WaterfallInsets: &DisplayRect{Top: 5}, CutoutSpec: "cutout", Scale: 1}, SupportedModes: []DisplayInfoMode{{Id: 1, Width: 1080, Height: 2400, RefreshRate: 60, VsyncRate: 120, AlternativeRefreshRates: []float32{30}, SupportedHdrTypes: []int32{2}}}, SupportedColorModes: []int32{1, 2}, HdrCapabilities: &DisplayHdrCapabilities{SupportedHdrTypes: []int32{2}, MaxLuminance: 500}, LogicalDensityDpi: 420, OwnerPackageName: "owner", RemoveMode: 3}
			if api == 34 {
				year := int32(2024)
				product := "model"
				v.DeviceProductInfo = &DeviceProductInfo{Name: "panel", ProductId: &product, ModelYear: &year, ManufactureDate: &DisplayManufactureDate{Year: &year}}
				v.RoundedCorners = &DisplayRoundedCorners{Variant: 1, Corners: []*DisplayRoundedCorner{{Radius: 3}, nil, nil, nil}}
				v.DisplayShape = &DisplayInfoShape{Spec: "shape", Scale: 1}
				v.LayoutLimitedRefreshRate = &DisplayRefreshRateRange{Min: 30, Max: 60}
				v.ThermalRefreshRateThrottling = []DisplayThermalRefreshRate{{Status: 2, Range: &DisplayRefreshRateRange{Min: 30, Max: 60}}}
				v.ThermalBrightnessThrottlingDataId = "tail"
			}
			p := parcel.New()
			p.SetTargetAPILevel(api)
			if err := v.MarshalParcel(p); err != nil {
				t.Fatal(err)
			}
			wire := append([]byte(nil), p.Data()...)
			p.WriteInt32(0x12345678)
			p.SetPosition(0)
			var got DisplayInfo
			if err := got.UnmarshalParcel(p); err != nil {
				t.Fatal(err)
			}
			if got.Address.PhysicalId != 123 || got.DisplayCutout.Bounds[2].Right != 2 || got.SupportedModes[0].Width != 1080 || got.HdrCapabilities.MaxLuminance != 500 || got.RemoveMode != 3 {
				t.Fatalf("bad nested values: %+v", got)
			}
			if api == 34 && (got.DeviceProductInfo.ManufactureDate.Year == nil || *got.DeviceProductInfo.ManufactureDate.Year != 2024 || got.ThermalRefreshRateThrottling[0].Range.Max != 60 || got.ThermalBrightnessThrottlingDataId != "tail") {
				t.Fatalf("bad tail: %+v", got)
			}
			if sent, err := p.ReadInt32(); err != nil || sent != 0x12345678 {
				t.Fatalf("sentinel: %x %v", sent, err)
			}
			for i := 0; i < len(wire); i += 4 {
				q := parcel.FromBytes(wire[:i])
				q.SetTargetAPILevel(api)
				var partial DisplayInfo
				if err := partial.UnmarshalParcel(q); err == nil {
					t.Fatalf("truncation at %d accepted", i)
				}
			}
		})
	}
}

func TestDisplayInfoUnsupportedAPI(t *testing.T) {
	p := parcel.New()
	var d DisplayInfo
	if err := d.UnmarshalParcel(p); err == nil {
		t.Fatal("unknown API accepted")
	}
	if err := d.MarshalParcel(p); err == nil {
		t.Fatal("unknown API accepted for marshal")
	}
}

// This fixture is written directly from the API 34 Java parcel order.
func TestDisplayInfoAPI34NestedWire(t *testing.T) {
	p := parcel.New()
	p.SetTargetAPILevel(34)
	for _, v := range []int32{1, 2, 3, 4, 5} {
		p.WriteInt32(v)
	}
	p.WriteString16("android.view.DisplayAddress$Network")
	p.WriteString16("ab:cd")
	p.WriteString16("android.hardware.display.DeviceProductInfo")
	p.WriteString16("panel")
	p.WriteString16("ABC")
	p.WriteInt32(0)
	p.WriteString16("product")
	p.WriteInt32(1)
	p.WriteInt32(2024)
	p.WriteInt32(-1)
	p.WriteInt32(2)
	p.WriteString("screen")
	for i := 0; i < 8; i++ {
		p.WriteInt32(int32(10 + i))
	}
	p.WriteInt32(1)
	p.WriteInt32(1)
	for _, v := range []int32{1, 2, 3, 4} {
		p.WriteInt32(v)
	}
	p.WriteInt32(4)
	for i := 0; i < 4; i++ {
		p.WriteInt32(0)
	}
	p.WriteInt32(0)
	for _, v := range []int32{1080, 2400, 1080, 2400} {
		p.WriteInt32(v)
	}
	p.WriteFloat32(3)
	p.WriteString16("spec")
	p.WriteInt32(0)
	p.WriteFloat32(1)
	p.WriteFloat32(1)
	p.WriteInt32(1)
	p.WriteInt32(2)
	p.WriteFloat32(60)
	p.WriteInt32(3)
	p.WriteInt32(4)
	p.WriteInt32(1)
	p.WriteInt32(2)
	p.WriteInt32(1080)
	p.WriteInt32(2400)
	p.WriteFloat32(60)
	p.WriteFloat32(120)
	p.WriteInt32(1)
	p.WriteFloat32(30)
	p.WriteInt32(1)
	p.WriteInt32(2)
	p.WriteInt32(7)
	p.WriteInt32(1)
	p.WriteInt32(8)
	p.WriteString16("android.view.Display$HdrCapabilities")
	p.WriteInt32(1)
	p.WriteInt32(2)
	p.WriteFloat32(500)
	p.WriteFloat32(300)
	p.WriteFloat32(1)
	p.WriteInt32(1)
	p.WriteInt32(420)
	p.WriteFloat32(421)
	p.WriteFloat32(422)
	p.WriteInt64(10)
	p.WriteInt64(20)
	p.WriteInt32(1)
	p.WriteInt32(2)
	p.WriteInt32(3)
	p.WriteString("owner")
	p.WriteString("unique")
	p.WriteInt32(4)
	for _, v := range []float32{60, 1, 2, 3} {
		p.WriteFloat32(v)
	}
	p.WriteInt32(0)
	p.WriteInt32(0)
	p.WriteInt32(5)
	p.WriteInt32(0)
	p.WriteInt32(0)
	p.WriteFloat32(1.5)
	p.WriteInt32(-1)
	p.WriteString("tail")
	wire := append([]byte(nil), p.Data()...)
	p.WriteInt32(12345)
	p.SetPosition(0)
	var d DisplayInfo
	if err := d.UnmarshalParcel(p); err != nil {
		t.Fatal(err)
	}
	if d.Address.MacAddress != "ab:cd" || *d.DeviceProductInfo.ProductId != "product" || d.DisplayCutout.SafeInsets.Bottom != 4 || d.SupportedModes[0].VsyncRate != 120 || d.HdrCapabilities.MaxLuminance != 500 || d.ThermalBrightnessThrottlingDataId != "tail" {
		t.Fatalf("bad decoded fields: %+v", d)
	}
	out := parcel.New()
	out.SetTargetAPILevel(34)
	if err := d.MarshalParcel(out); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out.Data(), wire) {
		t.Fatal("marshal differs from independent fixture")
	}
	if v, err := p.ReadInt32(); err != nil || v != 12345 {
		t.Fatalf("sentinel %d %v", v, err)
	}
}
