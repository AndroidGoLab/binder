// Set and read clipboard text via the Android clipboard binder service.
//
// Demonstrates actual clipboard copy-paste through the IClipboard binder
// interface: sets a plain-text clip using MarshalPlainTextClipData, reads
// it back with UnmarshalClipDataText, and verifies the round-trip.
//
// The generated ClipData.MarshalParcel writes an incomplete ClipDescription
// (null label, null mimeTypes), so we build the SetPrimaryClip transaction
// parcel manually using the correct wire format from clipdata_plaintext.go.
//
// Build:
//
//	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o build/clipboard_monitor ./examples/clipboard_monitor/
//	adb push build/clipboard_monitor /data/local/tmp/ && adb shell /data/local/tmp/clipboard_monitor
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/AndroidGoLab/binder/android/content"
	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/binder/versionaware"
	"github.com/AndroidGoLab/binder/kernelbinder"
	"github.com/AndroidGoLab/binder/parcel"
	"github.com/AndroidGoLab/binder/servicemanager"
)

// defaultDeviceID is the primary display device.
const defaultDeviceID int32 = 0

func main() {
	text := flag.String("text", "Hello from Go binder!", "text to place on the clipboard")
	flag.Parse()

	if err := run(context.Background(), *text); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(
	ctx context.Context,
	text string,
) error {
	driver, err := kernelbinder.Open(ctx, binder.WithMapSize(128*1024))
	if err != nil {
		return fmt.Errorf("open binder: %w", err)
	}
	defer driver.Close(ctx)

	transport, err := versionaware.NewTransport(ctx, driver, 0)
	if err != nil {
		return fmt.Errorf("version-aware transport: %w", err)
	}

	sm := servicemanager.New(transport)

	svc, err := sm.GetService(ctx, servicemanager.ClipboardService)
	if err != nil {
		return fmt.Errorf("get clipboard service: %w", err)
	}

	cb := content.NewClipboardProxy(svc)

	// Set clipboard text.
	fmt.Printf("Setting clipboard text: %q\n", text)
	if err := setPrimaryClipText(ctx, svc, "go-binder", text); err != nil {
		return fmt.Errorf("set clipboard: %w", err)
	}
	fmt.Println("Clipboard text set successfully.")

	// Verify the clipboard has content.
	hasPrimary, err := cb.HasPrimaryClip(ctx, defaultDeviceID)
	if err != nil {
		return fmt.Errorf("HasPrimaryClip: %w", err)
	}
	fmt.Printf("Has primary clip: %v\n", hasPrimary)

	hasText, err := cb.HasClipboardText(ctx, defaultDeviceID)
	if err != nil {
		return fmt.Errorf("HasClipboardText: %w", err)
	}
	fmt.Printf("Has clipboard text: %v\n", hasText)

	// Read the clipboard text back.
	clipText, err := getPrimaryClipText(ctx, svc)
	if err != nil {
		return fmt.Errorf("get clipboard: %w", err)
	}

	fmt.Printf("Read back %d item(s):\n", len(clipText.Items))
	fmt.Printf("  Label:      %q\n", clipText.Label)
	fmt.Printf("  MIME types: %s\n", strings.Join(clipText.MIMETypes, ", "))
	for i, item := range clipText.Items {
		fmt.Printf("  Item[%d]:    %q\n", i, item)
	}

	// Verify round-trip.
	if len(clipText.Items) > 0 && clipText.Items[0] == text {
		fmt.Println("Round-trip verified: clipboard text matches.")
	} else {
		fmt.Println("WARNING: clipboard text does not match what was set.")
	}

	return nil
}

// setPrimaryClipText places a plain-text clip on the clipboard by building
// the SetPrimaryClip transaction parcel manually. This bypasses the generated
// ClipData.MarshalParcel (which writes an incomplete ClipDescription) and
// instead uses MarshalPlainTextClipData for the correct wire format.
func setPrimaryClipText(
	ctx context.Context,
	remote binder.IBinder,
	label string,
	text string,
) error {
	identity := remote.Identity()

	data := parcel.New()
	defer data.Recycle()

	data.WriteInterfaceToken(content.DescriptorIClipboard)

	// Resolve the method signature to handle version-aware parameter ordering.
	sig := binder.ResolveMethodSignature(
		remote, ctx,
		content.DescriptorIClipboard,
		content.MethodIClipboardSetPrimaryClip,
	)

	compiledDescs := []string{
		"Landroid/content/ClipData;",
		"Ljava/lang/String;",
		"Ljava/lang/String;",
		"I",
		"I",
	}

	if sig == nil || binder.SignatureMatches(compiledDescs, sig) {
		data.WriteInt32(1) // non-null ClipData
		content.MarshalPlainTextClipData(data, label, text)
		data.WriteString16(identity.PackageName)
		data.WriteString16(identity.AttributionTag)
		data.WriteInt32(identity.UserID)
		data.WriteInt32(defaultDeviceID)
	} else {
		paramMap := binder.MatchParamsToSignature(compiledDescs, sig)
		for _, pi := range paramMap {
			switch pi {
			case 0:
				data.WriteInt32(1) // non-null ClipData
				content.MarshalPlainTextClipData(data, label, text)
			case 1:
				data.WriteString16(identity.PackageName)
			case 2:
				data.WriteString16(identity.AttributionTag)
			case 3:
				data.WriteInt32(identity.UserID)
			case 4:
				data.WriteInt32(defaultDeviceID)
			}
		}
	}

	code, err := remote.ResolveCode(
		ctx,
		content.DescriptorIClipboard,
		content.MethodIClipboardSetPrimaryClip,
	)
	if err != nil {
		return fmt.Errorf("resolving %s.%s: %w",
			content.DescriptorIClipboard,
			content.MethodIClipboardSetPrimaryClip, err)
	}

	reply, err := remote.Transact(ctx, code, 0, data)
	if err != nil {
		return err
	}
	defer reply.Recycle()

	return binder.ReadStatus(reply)
}

// getPrimaryClipText reads the primary clip from the clipboard and extracts
// text content. Uses a raw GetPrimaryClip transaction so we can parse the
// reply with UnmarshalClipDataText instead of the generated (incomplete)
// ClipData.UnmarshalParcel.
func getPrimaryClipText(
	ctx context.Context,
	remote binder.IBinder,
) (content.ClipDataText, error) {
	var empty content.ClipDataText
	identity := remote.Identity()

	data := parcel.New()
	defer data.Recycle()

	data.WriteInterfaceToken(content.DescriptorIClipboard)

	sig := binder.ResolveMethodSignature(
		remote, ctx,
		content.DescriptorIClipboard,
		content.MethodIClipboardGetPrimaryClip,
	)

	compiledDescs := []string{
		"Ljava/lang/String;",
		"Ljava/lang/String;",
		"I",
		"I",
	}

	if sig == nil || binder.SignatureMatches(compiledDescs, sig) {
		data.WriteString16(identity.PackageName)
		data.WriteString16(identity.AttributionTag)
		data.WriteInt32(identity.UserID)
		data.WriteInt32(defaultDeviceID)
	} else {
		paramMap := binder.MatchParamsToSignature(compiledDescs, sig)
		for _, pi := range paramMap {
			switch pi {
			case 0:
				data.WriteString16(identity.PackageName)
			case 1:
				data.WriteString16(identity.AttributionTag)
			case 2:
				data.WriteInt32(identity.UserID)
			case 3:
				data.WriteInt32(defaultDeviceID)
			}
		}
	}

	code, err := remote.ResolveCode(
		ctx,
		content.DescriptorIClipboard,
		content.MethodIClipboardGetPrimaryClip,
	)
	if err != nil {
		return empty, fmt.Errorf("resolving %s.%s: %w",
			content.DescriptorIClipboard,
			content.MethodIClipboardGetPrimaryClip, err)
	}

	reply, err := remote.Transact(ctx, code, 0, data)
	if err != nil {
		return empty, err
	}
	defer reply.Recycle()

	if err = binder.ReadStatus(reply); err != nil {
		return empty, err
	}

	nullIndicator, err := reply.ReadInt32()
	if err != nil {
		return empty, fmt.Errorf("reading null indicator: %w", err)
	}
	if nullIndicator == 0 {
		return empty, fmt.Errorf("clipboard returned null ClipData")
	}

	return content.UnmarshalClipDataText(reply)
}
