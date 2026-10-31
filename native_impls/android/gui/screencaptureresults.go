package gui

import (
	"errors"
	"fmt"

	"github.com/AndroidGoLab/binder/gralloc"
	"github.com/AndroidGoLab/binder/parcel"
	"golang.org/x/sys/unix"
)

// ScreenCaptureResults holds a SurfaceFlinger capture and owns its received descriptors.
//
// Example: call Close after using Buffer and waiting for FenceFD.
type ScreenCaptureResults struct {
	Buffer               *gralloc.Buffer
	GainMap              *gralloc.Buffer
	FenceFD              int32
	FenceStatus          int32
	CapturedSecureLayers bool
	CapturedHdrLayers    bool
	CapturedDataspace    uint32
	HdrSdrRatio          float32
	fenceOwned           bool
}

var _ parcel.Parcelable = (*ScreenCaptureResults)(nil)

// MarshalParcel is not implemented for native capture results.
//
// Example: results are received from SurfaceFlinger, not sent to it.
func (s *ScreenCaptureResults) MarshalParcel(p *parcel.Parcel) error {
	return fmt.Errorf("ScreenCaptureResults.MarshalParcel: not implemented")
}

// Close releases the fence and the buffer descriptors, including mapped memory.
//
// Example: defer result.Close() after a successful decode.
func (s *ScreenCaptureResults) Close() error {
	var errs []error
	if s.fenceOwned {
		s.fenceOwned = false
		errs = append(errs, unix.Close(int(s.FenceFD)))
	}
	s.FenceFD = -1
	for _, buffer := range []*gralloc.Buffer{s.Buffer, s.GainMap} {
		if buffer == nil {
			continue
		}
		buffer.Munmap()
		for _, fd := range buffer.Handle.Fds {
			errs = append(errs, unix.Close(int(fd)))
		}
		buffer.Handle.Fds = nil
	}
	s.Buffer = nil
	s.GainMap = nil
	return errors.Join(errs...)
}

// UnmarshalParcel decodes a capture result and takes ownership of its descriptors.
// On failure it releases descriptors already read; the parcel owns any unread ones.
//
// Example: decode a received callback parcel before inspecting its buffer.
func (s *ScreenCaptureResults) UnmarshalParcel(p *parcel.Parcel) (_err error) {
	if err := s.Close(); err != nil {
		return fmt.Errorf("closing previous result: %w", err)
	}
	*s = ScreenCaptureResults{FenceFD: -1}
	defer func() {
		if _err != nil {
			_ = s.Close()
		}
	}()

	hasBuffer, err := p.ReadBool()
	if err != nil {
		return fmt.Errorf("reading hasBuffer: %w", err)
	}
	if hasBuffer {
		s.Buffer, err = gralloc.ReadGraphicBuffer(p)
		if err != nil {
			return fmt.Errorf("reading GraphicBuffer: %w", err)
		}
	}
	hasFence, err := p.ReadBool()
	if err != nil {
		return fmt.Errorf("reading hasFence: %w", err)
	}
	if hasFence {
		size, err := p.ReadInt32()
		if err != nil {
			return fmt.Errorf("reading fence size: %w", err)
		}
		count, err := p.ReadInt32()
		if err != nil {
			return fmt.Errorf("reading fence fd count: %w", err)
		}
		if size != 4 || count != 1 {
			return fmt.Errorf("invalid fence layout: size %d, fd count %d", size, count)
		}
		if _, err := p.Read(int(size)); err != nil {
			return fmt.Errorf("reading fence data: %w", err)
		}
		s.FenceFD, err = p.ReadFileDescriptor()
		if err != nil {
			return fmt.Errorf("reading fence fd: %w", err)
		}
		s.fenceOwned = true
	} else {
		s.FenceStatus, err = p.ReadInt32()
		if err != nil {
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
	s.CapturedDataspace, err = p.ReadUint32()
	if err != nil {
		return fmt.Errorf("reading capturedDataspace: %w", err)
	}
	if p.Position() >= p.Len() {
		return nil
	}
	hasGainMap, err := p.ReadBool()
	if err != nil {
		return fmt.Errorf("reading hasGainMap: %w", err)
	}
	if hasGainMap {
		s.GainMap, err = gralloc.ReadGraphicBuffer(p)
		if err != nil {
			return fmt.Errorf("reading gain map: %w", err)
		}
	}
	s.HdrSdrRatio, err = p.ReadFloat32()
	if err != nil {
		return fmt.Errorf("reading hdrSdrRatio: %w", err)
	}
	return nil
}
