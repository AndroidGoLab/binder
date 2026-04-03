package gui

import (
	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/parcel"
)

// LayerCaptureArgs contains arguments for capturing a layer and its children.
//
// Wire format matches the custom C++ serialization in
// frameworks/native/libs/gui/LayerState.cpp LayerCaptureArgs::writeToParcel.
// This is a cpp_header parcelable with flat layout: no parcelable
// header/footer, no null indicators. The C++ struct inherits from
// CaptureArgs, so writeToParcel first calls CaptureArgs::writeToParcel
// (inline, not wrapped in a null indicator), then writes its own fields.
type LayerCaptureArgs struct {
	CaptureArgs  CaptureArgs
	LayerHandle  binder.IBinder
	ChildrenOnly bool
}

var _ parcel.Parcelable = (*LayerCaptureArgs)(nil)

func (s *LayerCaptureArgs) MarshalParcel(
	p *parcel.Parcel,
) error {
	// Flat layout matching LayerCaptureArgs::writeToParcel in C++.
	// CaptureArgs fields are inlined (C++ inheritance), not wrapped.
	if err := s.CaptureArgs.MarshalParcel(p); err != nil {
		return err
	}

	if s.LayerHandle == nil {
		p.WriteNullStrongBinder()
	} else {
		p.WriteStrongBinder(s.LayerHandle.Handle())
	}
	p.WriteBool(s.ChildrenOnly)
	return nil
}

func (s *LayerCaptureArgs) UnmarshalParcel(
	p *parcel.Parcel,
) error {
	// CaptureArgs fields are inlined (C++ inheritance).
	if err := s.CaptureArgs.UnmarshalParcel(p); err != nil {
		return err
	}

	handle, err := p.ReadStrongBinder()
	if err != nil {
		return err
	}
	if handle != 0 {
		s.LayerHandle = binder.NewProxyBinder(nil, binder.CallerIdentity{}, handle)
	}

	s.ChildrenOnly, err = p.ReadBool()
	if err != nil {
		return err
	}

	return nil
}
