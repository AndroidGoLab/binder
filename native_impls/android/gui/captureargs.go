package gui

import (
	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/parcel"
)

// CaptureArgs contains common arguments for screen capture operations.
//
// Wire format matches the custom C++ serialization in
// frameworks/native/libs/gui/LayerState.cpp CaptureArgs::writeToParcel.
// This is a cpp_header parcelable with flat (non-AIDL-structured) layout:
// no parcelable header/footer, no null indicators for embedded types.
// The C++ Rect (sourceCrop) is written as 4 raw int32s, not as an
// AIDL structured parcelable.
type CaptureArgs struct {
	PixelFormat              int32
	SourceCrop               ARect
	FrameScaleX              float32
	FrameScaleY              float32
	CaptureSecureLayers      bool
	Uid                      int32
	Dataspace                int32
	AllowProtected           bool
	Grayscale                bool
	ExcludeHandles           []binder.IBinder
	HintForSeamlessTransition bool
}

var _ parcel.Parcelable = (*CaptureArgs)(nil)

func (s *CaptureArgs) MarshalParcel(
	p *parcel.Parcel,
) error {
	// Flat layout matching CaptureArgs::writeToParcel in C++.
	p.WriteInt32(s.PixelFormat)

	// sourceCrop: C++ Parcel::write(Rect) writes 4 raw int32s.
	p.WriteInt32(s.SourceCrop.Left)
	p.WriteInt32(s.SourceCrop.Top)
	p.WriteInt32(s.SourceCrop.Right)
	p.WriteInt32(s.SourceCrop.Bottom)

	p.WriteFloat32(s.FrameScaleX)
	p.WriteFloat32(s.FrameScaleY)
	p.WriteBool(s.CaptureSecureLayers)
	p.WriteInt32(s.Uid)
	p.WriteInt32(s.Dataspace)
	p.WriteBool(s.AllowProtected)
	p.WriteBool(s.Grayscale)

	// excludeHandles: C++ writes size (int32) then N strong binders.
	// An empty set writes 0, not -1.
	if s.ExcludeHandles == nil {
		p.WriteInt32(0)
	} else {
		p.WriteInt32(int32(len(s.ExcludeHandles)))
		for _, h := range s.ExcludeHandles {
			if h == nil {
				p.WriteNullStrongBinder()
			} else {
				p.WriteStrongBinder(h.Handle())
			}
		}
	}

	p.WriteBool(s.HintForSeamlessTransition)
	return nil
}

func (s *CaptureArgs) UnmarshalParcel(
	p *parcel.Parcel,
) error {
	var err error

	s.PixelFormat, err = p.ReadInt32()
	if err != nil {
		return err
	}

	// sourceCrop: 4 raw int32s (C++ Parcel::read(Rect)).
	s.SourceCrop.Left, err = p.ReadInt32()
	if err != nil {
		return err
	}
	s.SourceCrop.Top, err = p.ReadInt32()
	if err != nil {
		return err
	}
	s.SourceCrop.Right, err = p.ReadInt32()
	if err != nil {
		return err
	}
	s.SourceCrop.Bottom, err = p.ReadInt32()
	if err != nil {
		return err
	}

	s.FrameScaleX, err = p.ReadFloat32()
	if err != nil {
		return err
	}
	s.FrameScaleY, err = p.ReadFloat32()
	if err != nil {
		return err
	}
	s.CaptureSecureLayers, err = p.ReadBool()
	if err != nil {
		return err
	}
	s.Uid, err = p.ReadInt32()
	if err != nil {
		return err
	}
	s.Dataspace, err = p.ReadInt32()
	if err != nil {
		return err
	}
	s.AllowProtected, err = p.ReadBool()
	if err != nil {
		return err
	}
	s.Grayscale, err = p.ReadBool()
	if err != nil {
		return err
	}

	n, err := p.ReadInt32()
	if err != nil {
		return err
	}
	if n > 0 {
		s.ExcludeHandles = make([]binder.IBinder, n)
		for i := range s.ExcludeHandles {
			handle, handleErr := p.ReadStrongBinder()
			if handleErr != nil {
				return handleErr
			}
			if handle != 0 {
				s.ExcludeHandles[i] = binder.NewProxyBinder(nil, binder.CallerIdentity{}, handle)
			}
		}
	}

	s.HintForSeamlessTransition, err = p.ReadBool()
	if err != nil {
		return err
	}

	return nil
}
