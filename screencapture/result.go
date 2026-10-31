package screencapture

import (
	"errors"
	"fmt"

	"github.com/AndroidGoLab/binder/android/gui"
	"github.com/AndroidGoLab/binder/parcel"
)

// nativeFrame is the resource-owning SurfaceFlinger result.
//
// Example: close a decoded result after copying its pixels.
type nativeFrame = gui.ScreenCaptureResults

// readResult decodes and validates a SurfaceFlinger capture result.
//
// Example: a nonzero fence status becomes an error instead of an empty frame.
func readResult(p *parcel.Parcel) (_ *nativeFrame, resultErr error) {
	frame := &nativeFrame{}
	if err := frame.UnmarshalParcel(p); err != nil {
		return nil, fmt.Errorf("decode capture result: %w", err)
	}
	defer // Release a decoded result if its capture status is invalid.
	//
	// Example: a failed capture does not retain its buffer.
	func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, frame.Close())
		}
	}()
	if frame.FenceStatus != 0 {
		return nil, fmt.Errorf("capture failed: status %d", frame.FenceStatus)
	}
	if frame.Buffer == nil {
		return nil, errors.New("capture returned no buffer")
	}
	return frame, nil
}
