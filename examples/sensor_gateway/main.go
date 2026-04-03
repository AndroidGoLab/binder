// Stream live sensor events via the SensorManager event queue callback.
//
// Demonstrates the ISensorManager.CreateEventQueue API: registers an
// IEventQueueCallback stub, enables the accelerometer at 100 ms sampling,
// and prints incoming sensor events for a fixed duration.
//
// This is the callback-driven counterpart to sensor_reader (which only
// lists sensor metadata).
//
// Build:
//
//	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o build/sensor_gateway ./examples/sensor_gateway/
//	adb push build/sensor_gateway /data/local/tmp/ && adb shell /data/local/tmp/sensor_gateway
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/AndroidGoLab/binder/android/frameworks/sensorservice"
	"github.com/AndroidGoLab/binder/android/hardware/sensors"
	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/binder/versionaware"
	"github.com/AndroidGoLab/binder/kernelbinder"
	"github.com/AndroidGoLab/binder/servicemanager"
)

const (
	// samplingPeriodUs is the desired sensor sampling interval.
	// 100 000 us = 100 ms = 10 Hz.
	samplingPeriodUs int32 = 100_000

	// streamDuration is how long to collect events before exiting.
	streamDuration = 5 * time.Second
)

// eventHandler implements IEventQueueCallbackServer, receiving sensor
// events from the HAL via binder oneway transactions.
type eventHandler struct {
	count atomic.Int64
}

func (h *eventHandler) OnEvent(
	_ context.Context,
	event sensors.Event,
) error {
	n := h.count.Add(1)
	ts := time.Duration(event.Timestamp) * time.Nanosecond

	switch event.Payload.Tag {
	case sensors.EventEventPayloadTagVec3:
		v := event.Payload.Vec3
		fmt.Printf("#%-4d  t=%12s  sensor=%d  type=%-3d  x=%+9.4f  y=%+9.4f  z=%+9.4f\n",
			n, ts.Truncate(time.Millisecond), event.SensorHandle, event.SensorType,
			v.X, v.Y, v.Z)
	case sensors.EventEventPayloadTagScalar:
		fmt.Printf("#%-4d  t=%12s  sensor=%d  type=%-3d  value=%+9.4f\n",
			n, ts.Truncate(time.Millisecond), event.SensorHandle, event.SensorType,
			event.Payload.Scalar)
	default:
		fmt.Printf("#%-4d  t=%12s  sensor=%d  type=%-3d  payload_tag=%d\n",
			n, ts.Truncate(time.Millisecond), event.SensorHandle, event.SensorType,
			event.Payload.Tag)
	}
	return nil
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	driver, err := kernelbinder.Open(ctx, binder.WithMapSize(128*1024))
	if err != nil {
		fmt.Fprintf(os.Stderr, "open binder: %v\n", err)
		os.Exit(1)
	}
	defer driver.Close(ctx)

	transport, err := versionaware.NewTransport(ctx, driver, 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "version-aware transport: %v\n", err)
		os.Exit(1)
	}

	sm := servicemanager.New(transport)

	svc, err := sm.GetService(ctx, "android.frameworks.sensorservice.ISensorManager/default")
	if err != nil {
		fmt.Fprintf(os.Stderr, "get sensor service: %v\n", err)
		os.Exit(1)
	}

	sensorMgr := sensorservice.NewSensorManagerProxy(svc)

	// Find the accelerometer so we know its handle.
	accel, err := sensorMgr.GetDefaultSensor(ctx, sensors.SensorTypeACCELEROMETER)
	if err != nil {
		fmt.Fprintf(os.Stderr, "GetDefaultSensor(ACCELEROMETER): %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Streaming accelerometer: %s (handle=%d, vendor=%s)\n",
		accel.Name, accel.SensorHandle, accel.Vendor)
	fmt.Printf("Sampling period: %d us, duration: %s\n\n", samplingPeriodUs, streamDuration)

	// Create a callback stub that receives sensor events.
	handler := &eventHandler{}
	callbackStub := sensorservice.NewEventQueueCallbackStub(handler)

	// Create an event queue bound to our callback.
	eventQueue, err := sensorMgr.CreateEventQueue(ctx, callbackStub)
	if err != nil {
		fmt.Fprintf(os.Stderr, "CreateEventQueue: %v\n", err)
		os.Exit(1)
	}

	// Enable the accelerometer on this queue.
	err = eventQueue.EnableSensor(ctx, accel.SensorHandle, samplingPeriodUs, 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "EnableSensor: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Sensor enabled, waiting for events...")

	// Wait for events until duration elapses or signal arrives.
	select {
	case <-ctx.Done():
	case <-time.After(streamDuration):
	}

	// Disable the sensor before exiting.
	if disableErr := eventQueue.DisableSensor(ctx, accel.SensorHandle); disableErr != nil {
		fmt.Fprintf(os.Stderr, "DisableSensor: %v\n", disableErr)
	}

	fmt.Printf("\nReceived %d events total.\n", handler.count.Load())
}
