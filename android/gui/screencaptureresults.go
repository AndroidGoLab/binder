package gui

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/AndroidGoLab/binder/gralloc"
	"github.com/AndroidGoLab/binder/parcel"
	"golang.org/x/sys/unix"
)

// ScreenCaptureResults holds a SurfaceFlinger capture and owns its descriptors.
// Copies share ownership: closing one invalidates all copies. Fields and buffer
// handles must not be changed concurrently with Close or CopyRGBA.
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
	owner                *screenCaptureOwner
}

// screenCaptureOwner serializes use and release shared by result copies.
//
// Example: listener arguments copied by value share one owner.
type screenCaptureOwner struct {
	mu         sync.Mutex
	closed     bool
	fenceOwned bool
}

var _ parcel.Parcelable = (*ScreenCaptureResults)(nil)

// MarshalParcel is not implemented for native capture results.
//
// Example: results are received from SurfaceFlinger, not sent to it.
func (s *ScreenCaptureResults) MarshalParcel(
	p *parcel.Parcel,
) error {
	return fmt.Errorf("ScreenCaptureResults.MarshalParcel: not implemented")
}

// Close releases the fence and the buffer descriptors, including mapped memory.
// It waits for any CopyRGBA call on this result or a copy to finish.
//
// Example: defer result.Close() after a successful decode.
func (s *ScreenCaptureResults) Close() error {
	if s.owner == nil {
		return nil
	}
	s.owner.mu.Lock()
	defer s.owner.mu.Unlock()
	if s.owner.closed {
		return nil
	}
	s.owner.closed = true
	var errs []error
	if s.owner.fenceOwned {
		s.owner.fenceOwned = false
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
// On failure it releases descriptors already read; received parcels retain unread ones.
//
// Example: decode a received callback parcel before inspecting its buffer.
func (s *ScreenCaptureResults) UnmarshalParcel(
	p *parcel.Parcel,
) (_err error) {
	if err := s.Close(); err != nil {
		return fmt.Errorf("closing previous result: %w", err)
	}
	*s = ScreenCaptureResults{FenceFD: -1, owner: &screenCaptureOwner{}}
	defer // Release any descriptors transferred before a decoding error.
	//
	// Example: truncated metadata follows a valid buffer.
	func() {
		if _err != nil {
			_err = errors.Join(_err, s.Close())
		}
	}()

	hasBuffer, err := p.ReadBool()
	if err != nil {
		return fmt.Errorf("reading hasBuffer: %w", err)
	}
	if hasBuffer {
		s.Buffer, err = gralloc.ReadOwnedGraphicBuffer(p)
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
		s.FenceFD, err = p.ReadOwnedFileDescriptor()
		if err != nil {
			return fmt.Errorf("reading fence fd: %w", err)
		}
		s.owner.fenceOwned = true
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
		s.GainMap, err = gralloc.ReadOwnedGraphicBuffer(p)
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

// CopyRGBA waits for capture completion and returns independently owned, tightly packed RGBA pixels.
// The caller retains ownership and must close the result.
//
// Example: copy pixels before releasing a listener result.
func (s *ScreenCaptureResults) CopyRGBA(
	ctx context.Context,
) ([]byte, error) {
	if s.owner == nil {
		return nil, fmt.Errorf("capture result has no owned buffer")
	}
	s.owner.mu.Lock()
	defer s.owner.mu.Unlock()
	if s.owner.closed {
		return nil, fmt.Errorf("capture result is closed")
	}
	if s.FenceStatus != 0 {
		return nil, fmt.Errorf("capture failed: status %d", s.FenceStatus)
	}
	if s.Buffer == nil {
		return nil, fmt.Errorf("capture result has no buffer")
	}
	if err := gralloc.WaitFence(ctx, s.FenceFD); err != nil {
		return nil, fmt.Errorf("wait for capture fence: %w", err)
	}
	return gralloc.CopyRGBA(s.Buffer)
}
