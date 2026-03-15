package location

import (
	"context"
	"fmt"

	"github.com/xaionaro-go/binder/servicemanager"
)

// GetLastKnownLocation returns the last known location from the given
// provider ("fused", "gps", "network", or "passive").
//
// Returns nil if no cached location is available for the provider.
func GetLastKnownLocation(
	ctx context.Context,
	sm *servicemanager.ServiceManager,
	provider string,
) (*Location, error) {
	mgr, err := GetLocationManager(ctx, sm)
	if err != nil {
		return nil, fmt.Errorf("GetLastKnownLocation: %w", err)
	}

	loc, err := mgr.GetLastLocation(ctx, provider, LastLocationRequest{}, "com.android.shell", "")
	if err != nil {
		return nil, fmt.Errorf("GetLastKnownLocation: %w", err)
	}

	// A zero FieldsMask with empty Provider indicates null Location
	// (server returned null indicator, Go deserialized as zero value).
	if loc.Provider == "" && loc.FieldsMask == 0 {
		return nil, nil
	}

	return &loc, nil
}
