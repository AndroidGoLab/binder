package gui

import (
	"fmt"

	"github.com/AndroidGoLab/binder/gralloc"
	"github.com/AndroidGoLab/binder/parcel"
)

// ScreenCaptureResults holds the result of a SurfaceFlinger screen
// capture. The wire format is defined in C++ (ScreenCaptureResults.cpp),
// not AIDL.
type ScreenCaptureResults struct {
	// Buffer contains the captured pixels as a gralloc GraphicBuffer.
	// Nil if capture failed.
	Buffer *gralloc.Buffer

	// CapturedSecureLayers indicates if secure content was captured.
	CapturedSecureLayers bool

	// CapturedHdrLayers indicates if HDR content was captured.
	CapturedHdrLayers bool

	// CapturedDataspace is the dataspace of the captured content.
	CapturedDataspace uint32

	// HdrSdrRatio is the HDR/SDR luminance ratio.
	HdrSdrRatio float32
}

var _ parcel.Parcelable = (*ScreenCaptureResults)(nil)

func (s *ScreenCaptureResults) MarshalParcel(
	p *parcel.Parcel,
) error {
	return fmt.Errorf("ScreenCaptureResults.MarshalParcel: not implemented")
}

func (s *ScreenCaptureResults) UnmarshalParcel(
	p *parcel.Parcel,
) error {
	// GraphicBuffer (nullable).
	hasBuffer, err := p.ReadBool()
	if err != nil {
		return fmt.Errorf("reading hasBuffer: %w", err)
	}
	if hasBuffer {
		buf, err := gralloc.ReadGraphicBuffer(p)
		if err != nil {
			return fmt.Errorf("reading GraphicBuffer: %w", err)
		}
		s.Buffer = buf
	}

	// Fence (nullable). We skip it — just need to advance the parcel.
	hasFence, err := p.ReadBool()
	if err != nil {
		return fmt.Errorf("reading hasFence: %w", err)
	}
	if hasFence {
		// Fence is flattened as: int32(flattenedSize) + int32(fdCount)
		// + raw[flattenedSize] + fds. For a valid fence, flattenedSize=4,
		// fdCount=1, raw=uint32(0), fd=fenceFd.
		fenceSize, err := p.ReadInt32()
		if err != nil {
			return fmt.Errorf("reading fence size: %w", err)
		}
		fenceFdCount, err := p.ReadInt32()
		if err != nil {
			return fmt.Errorf("reading fence fd count: %w", err)
		}
		if fenceSize > 0 {
			if _, err := p.Read(int(fenceSize)); err != nil {
				return fmt.Errorf("reading fence data: %w", err)
			}
		}
		for i := int32(0); i < fenceFdCount; i++ {
			if _, err := p.ReadFileDescriptor(); err != nil {
				return fmt.Errorf("reading fence fd: %w", err)
			}
		}
	} else {
		// No fence — read the status int32.
		if _, err := p.ReadInt32(); err != nil {
			return fmt.Errorf("reading fence status: %w", err)
		}
	}

	s.CapturedSecureLayers, err = p.ReadBool()
	if err != nil {
		return fmt.Errorf("reading capturedSecureLayers: %w", err)
	}

	s.CapturedHdrLayers, err = p.ReadBool()
	if err != nil {
		return fmt.Errorf("reading capturedHdrLayers: %w", err)
	}

	dataspace, err := p.ReadUint32()
	if err != nil {
		return fmt.Errorf("reading capturedDataspace: %w", err)
	}
	s.CapturedDataspace = dataspace

	// gainMap and hdrSdrRatio were added in Android 15 (API 35).
	// Older versions end the parcel after capturedDataspace.
	if p.Position() >= p.Len() {
		return nil
	}

	// Optional gain map (nullable GraphicBuffer).
	hasGainMap, err := p.ReadBool()
	if err != nil {
		return fmt.Errorf("reading hasGainMap: %w", err)
	}
	if hasGainMap {
		// Skip the gain map buffer — we don't need it.
		if _, err := gralloc.ReadGraphicBuffer(p); err != nil {
			return fmt.Errorf("reading gain map: %w", err)
		}
	}

	s.HdrSdrRatio, err = p.ReadFloat32()
	if err != nil {
		return fmt.Errorf("reading hdrSdrRatio: %w", err)
	}

	return nil
}
