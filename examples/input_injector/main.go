// Inject input events via InputManager's binder interface.
//
// Demonstrates:
//   - Enumerating input devices
//   - Reading / changing mouse pointer speed
//   - Injecting a key event (KEYCODE_BACK) by building the raw
//     KeyEvent parcel, bypassing the empty InputEvent stub
//
// Build:
//
//	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o build/input_injector ./examples/input_injector/
//	adb push build/input_injector /data/local/tmp/ && adb shell /data/local/tmp/input_injector
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/AndroidGoLab/binder/android/hardware/input"
	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/kernelbinder"
	"github.com/AndroidGoLab/binder/binder/versionaware"
	"github.com/AndroidGoLab/binder/parcel"
	"github.com/AndroidGoLab/binder/servicemanager"
)

// Android KeyEvent constants.
// Values from frameworks/base/core/java/android/view/KeyEvent.java
// and frameworks/base/core/java/android/view/InputDevice.java.
const (
	actionDown = 0
	actionUp   = 1

	keycodeHome       = 3
	keycodeBack       = 4
	keycodeVolumeUp   = 24
	keycodeVolumeDown = 25

	// InputDevice.SOURCE_KEYBOARD = 0x00000100 | SOURCE_CLASS_BUTTON(0x1)
	sourceKeyboard = 0x00000101

	// InputEvent.PARCEL_TOKEN_KEY_EVENT (discriminator written before
	// KeyEvent fields so the receiver knows which subclass follows).
	parcelTokenKeyEvent = 2

	// InputManager.INJECT_INPUT_EVENT_MODE_ASYNC
	injectModeAsync = 0
	// InputManager.INJECT_INPUT_EVENT_MODE_WAIT_FOR_FINISH
	injectModeWaitForFinish = 2
)

func main() {
	keycode := flag.Int("keycode", int(keycodeBack), "Android keycode to inject (default: KEYCODE_BACK=4)")
	speed := flag.Int("speed", -1, "set mouse pointer speed (1-10, -1 to skip)")
	flag.Parse()

	ctx := context.Background()

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

	svc, err := sm.GetService(ctx, servicemanager.InputService)
	if err != nil {
		fmt.Fprintf(os.Stderr, "get input service: %v\n", err)
		os.Exit(1)
	}

	im := input.NewInputManagerProxy(svc)

	// ---- List input devices ----
	ids, err := im.GetInputDeviceIds(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "GetInputDeviceIds: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Found %d input devices:\n", len(ids))
	for _, id := range ids {
		dev, err := im.GetInputDevice(ctx, id)
		if err != nil {
			fmt.Printf("  [id=%d] error: %v\n", id, err)
			continue
		}
		enabled, _ := im.IsInputDeviceEnabled(ctx, id)
		fmt.Printf("  [id=%d] %s (enabled=%v, sources=0x%x)\n",
			id, dev.Name, enabled, dev.Sources)
	}

	// ---- Mouse pointer speed ----
	currentSpeed, err := im.GetMousePointerSpeed(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nGetMousePointerSpeed: %v\n", err)
	} else {
		fmt.Printf("\nMouse pointer speed: %d\n", currentSpeed)
	}

	if *speed >= 0 {
		fmt.Printf("Setting pointer speed to %d (via TryPointerSpeed)...\n", *speed)
		if err := im.TryPointerSpeed(ctx, int32(*speed)); err != nil {
			fmt.Fprintf(os.Stderr, "TryPointerSpeed: %v\n", err)
		} else {
			fmt.Println("TryPointerSpeed succeeded")
			newSpeed, err := im.GetMousePointerSpeed(ctx)
			if err == nil {
				fmt.Printf("Mouse pointer speed is now: %d\n", newSpeed)
			}
		}
	}

	// ---- Inject key event ----
	fmt.Printf("\nInjecting keycode %d (ACTION_DOWN + ACTION_UP)...\n", *keycode)

	nowMs := time.Now().UnixMilli()
	if err := injectKeyEvent(ctx, im, int32(*keycode), actionDown, nowMs); err != nil {
		fmt.Fprintf(os.Stderr, "inject ACTION_DOWN: %v\n", err)
		os.Exit(1)
	}
	if err := injectKeyEvent(ctx, im, int32(*keycode), actionUp, nowMs); err != nil {
		fmt.Fprintf(os.Stderr, "inject ACTION_UP: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Key event injected successfully")
}

// injectKeyEvent builds a raw KeyEvent parcel and sends it via the
// injectInputEvent transaction.
//
// The generated InjectInputEvent proxy accepts view.InputEvent which is an
// empty parcelable (InputEvent is abstract in Java). Android's wire format
// expects a discriminator token (PARCEL_TOKEN_KEY_EVENT = 2) followed by
// the KeyEvent fields. We construct this manually.
func injectKeyEvent(
	ctx context.Context,
	im *input.InputManagerProxy,
	keycode int32,
	action int32,
	eventTimeMs int64,
) error {
	data := parcel.New()
	defer data.Recycle()

	data.WriteInterfaceToken(input.DescriptorIInputManager)

	// Non-null indicator for the InputEvent parcelable.
	data.WriteInt32(1)

	// --- KeyEvent wire format (matches KeyEvent.writeToParcel) ---
	data.WriteInt32(parcelTokenKeyEvent) // discriminator
	data.WriteInt32(0)                   // id (mId)
	data.WriteInt32(0)                   // deviceId (virtual)
	data.WriteInt32(sourceKeyboard)      // source
	data.WriteInt32(-1)                  // displayId (DEFAULT_DISPLAY = -1 targets focused)
	data.WriteByteArray(nil)             // hmac (null)
	data.WriteInt32(action)              // action
	data.WriteInt32(keycode)             // keyCode
	data.WriteInt32(0)                   // repeatCount
	data.WriteInt32(0)                   // metaState
	data.WriteInt32(0)                   // scanCode
	data.WriteInt32(0)                   // flags
	data.WriteInt64(eventTimeMs)         // downTime
	data.WriteInt64(eventTimeMs)         // eventTime
	data.WriteNullString16()             // characters (null)

	// Injection mode: INJECT_INPUT_EVENT_MODE_WAIT_FOR_FINISH
	data.WriteInt32(injectModeWaitForFinish)

	code, err := im.Remote.ResolveCode(
		ctx,
		input.DescriptorIInputManager,
		input.MethodIInputManagerInjectInputEvent,
	)
	if err != nil {
		return fmt.Errorf("resolving injectInputEvent: %w", err)
	}

	reply, err := im.Remote.Transact(ctx, code, 0, data)
	if err != nil {
		return fmt.Errorf("transact injectInputEvent: %w", err)
	}
	defer reply.Recycle()

	if err := binder.ReadStatus(reply); err != nil {
		return fmt.Errorf("injectInputEvent status: %w", err)
	}

	injected, err := reply.ReadBool()
	if err != nil {
		return fmt.Errorf("reading inject result: %w", err)
	}
	if !injected {
		return fmt.Errorf("injection rejected by InputManager (permission denied or policy)")
	}

	return nil
}
