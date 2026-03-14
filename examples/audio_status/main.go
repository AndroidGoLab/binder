// Query audio state: volume levels, mute status, audio mode.
//
// Build:
//
//	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o audio_status ./examples/audio_status/
//	adb push audio_status /data/local/tmp/ && adb shell /data/local/tmp/audio_status
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/xaionaro-go/aidl/binder"
	"github.com/xaionaro-go/aidl/android/media"
	"github.com/xaionaro-go/aidl/kernelbinder"
	"github.com/xaionaro-go/aidl/servicemanager"
)

const (
	streamVoice        = 0
	streamSystem       = 1
	streamRing         = 2
	streamMusic        = 3
	streamAlarm        = 4
	streamNotification = 5
)

var streamNames = map[int32]string{
	streamVoice:        "Voice",
	streamSystem:       "System",
	streamRing:         "Ring",
	streamMusic:        "Music",
	streamAlarm:        "Alarm",
	streamNotification: "Notification",
}

func main() {
	ctx := context.Background()

	driver, err := kernelbinder.Open(ctx, binder.WithMapSize(128*1024))
	if err != nil {
		fmt.Fprintf(os.Stderr, "open binder: %v\n", err)
		os.Exit(1)
	}
	defer driver.Close(ctx)

	sm := servicemanager.New(driver)

	svc, err := sm.GetService(ctx, "audio")
	if err != nil {
		fmt.Fprintf(os.Stderr, "get audio service: %v\n", err)
		os.Exit(1)
	}

	audio := media.NewAudioServiceProxy(svc)

	mode, err := audio.GetMode(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "GetMode: %v\n", err)
	} else {
		modeName := "unknown"
		switch mode {
		case 0:
			modeName = "normal"
		case 1:
			modeName = "ringtone"
		case 2:
			modeName = "in call"
		case 3:
			modeName = "in communication"
		}
		fmt.Printf("Audio mode: %s (%d)\n", modeName, mode)
	}

	micMuted, err := audio.IsMicrophoneMuted(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "IsMicrophoneMuted: %v\n", err)
	} else {
		fmt.Printf("Microphone muted: %v\n", micMuted)
	}

	fmt.Println("\nVolume levels:")
	for stream := int32(0); stream <= streamNotification; stream++ {
		name := streamNames[stream]
		vol, err := audio.GetStreamVolume(ctx, stream)
		if err != nil {
			continue
		}
		maxVol, err := audio.GetStreamMaxVolume(ctx, stream)
		if err != nil {
			continue
		}
		muted, _ := audio.IsStreamMute(ctx, stream)

		muteStr := ""
		if muted {
			muteStr = " [MUTED]"
		}
		fmt.Printf("  %-12s %d / %d%s\n", name, vol, maxVol, muteStr)
	}
}
