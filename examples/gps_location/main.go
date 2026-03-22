// Live GPS location via binder IPC: register a callback, receive a fix, print coordinates.
//
// Build:
//
//	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o build/gps_location ./examples/gps_location/
//	adb push build/gps_location /data/local/tmp/ && adb shell /data/local/tmp/gps_location
package main

import (
	"context"
	"fmt"
	"math"
	"os"
	"time"

	"github.com/xaionaro-go/binder/android/location"
	androidos "github.com/xaionaro-go/binder/android/os"
	"github.com/xaionaro-go/binder/binder"
	"github.com/xaionaro-go/binder/binder/versionaware"
	"github.com/xaionaro-go/binder/kernelbinder"
	"github.com/xaionaro-go/binder/servicemanager"
)

const gpsTimeout = 30 * time.Second

// gpsListener receives location callbacks from the LocationManager.
type gpsListener struct {
	fixCh chan location.Location
}

func (l *gpsListener) OnLocationChanged(
	_ context.Context,
	locations []location.Location,
	_ androidos.IRemoteCallback,
) error {
	for _, loc := range locations {
		select {
		case l.fixCh <- loc:
		default:
		}
	}
	return nil
}

func (l *gpsListener) OnProviderEnabledChanged(
	_ context.Context,
	_ string,
	_ bool,
) error {
	return nil
}

func (l *gpsListener) OnFlushComplete(_ context.Context, _ int32) error {
	return nil
}

func main() {
	ctx := context.Background()

	drv, err := kernelbinder.Open(ctx, binder.WithMapSize(128*1024))
	if err != nil {
		fmt.Fprintf(os.Stderr, "open binder: %v\n", err)
		os.Exit(1)
	}
	defer drv.Close(ctx)

	transport, err := versionaware.NewTransport(ctx, drv, 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "version-aware transport: %v\n", err)
		os.Exit(1)
	}

	sm := servicemanager.New(transport)

	lm, err := location.GetLocationManager(ctx, sm)
	if err != nil {
		fmt.Fprintf(os.Stderr, "get location manager: %v\n", err)
		os.Exit(1)
	}

	// Create the listener stub.
	impl := &gpsListener{fixCh: make(chan location.Location, 1)}
	listener := location.NewLocationListenerStub(impl)

	request := location.LocationRequest{
		Provider:               location.GpsProvider,
		IntervalMillis:         1000,
		ExpireAtRealtimeMillis: math.MaxInt64,
		DurationMillis:         math.MaxInt64,
	}

	packageName := binder.DefaultCallerIdentity().PackageName
	fmt.Println("Registering GPS listener...")
	err = lm.RegisterLocationListener(ctx, location.GpsProvider, request, listener, packageName, "gps-example")
	if err != nil {
		fmt.Fprintf(os.Stderr, "register listener: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Waiting for GPS fix (timeout %s)...\n", gpsTimeout)
	select {
	case loc := <-impl.fixCh:
		fmt.Printf("Lat:      %.6f\n", loc.LatitudeDegrees)
		fmt.Printf("Lon:      %.6f\n", loc.LongitudeDegrees)
		fmt.Printf("Alt:      %.1f m\n", loc.AltitudeMeters)
		fmt.Printf("Speed:    %.1f m/s\n", loc.SpeedMetersPerSecond)
		fmt.Printf("Bearing:  %.1f deg\n", loc.BearingDegrees)
		fmt.Printf("Accuracy: %.1f m\n", loc.HorizontalAccuracyMeters)
	case <-time.After(gpsTimeout):
		fmt.Fprintf(os.Stderr, "timed out waiting for GPS fix\n")
	}

	fmt.Println("Unregistering listener...")
	if err := lm.UnregisterLocationListener(ctx, listener); err != nil {
		fmt.Fprintf(os.Stderr, "unregister listener: %v\n", err)
	}
}
