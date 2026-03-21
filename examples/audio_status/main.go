// Query audio state: volume levels, mute status, audio mode.
//
// Build:
//
//	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o build/audio_status ./examples/audio_status/
//	adb push audio_status /data/local/tmp/ && adb shell /data/local/tmp/audio_status
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/xaionaro-go/binder/binder"
	"github.com/xaionaro-go/binder/binder/versionaware"
	"github.com/xaionaro-go/binder/android/media"
	"github.com/xaionaro-go/binder/kernelbinder"
	"github.com/xaionaro-go/binder/servicemanager"
)

type audioStream int32

const (
	streamVoice        audioStream = iota
	streamSystem
	streamRing
	streamMusic
	streamAlarm
	streamNotification
)

type audioMode int32

const (
	audioModeNormal          audioMode = 0
	audioModeRingtone        audioMode = 1
	audioModeInCall          audioMode = 2
	audioModeInCommunication audioMode = 3
)

var streamNames = map[audioStream]string{
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

	transport, err := versionaware.NewTransport(ctx, driver, 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "version-aware transport: %v\n", err)
		os.Exit(1)
	}

	sm := servicemanager.New(transport)

	svc, err := sm.GetService(ctx, servicemanager.AudioService)
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
		switch audioMode(mode) {
		case audioModeNormal:
			modeName = "normal"
		case audioModeRingtone:
			modeName = "ringtone"
		case audioModeInCall:
			modeName = "in call"
		case audioModeInCommunication:
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
	for stream := streamVoice; stream <= streamNotification; stream++ {
		name := streamNames[stream]
		vol, err := audio.GetStreamVolume(ctx, int32(stream))
		if err != nil {
			continue
		}
		maxVol, err := audio.GetStreamMaxVolume(ctx, int32(stream))
		if err != nil {
			continue
		}
		muted, _ := audio.IsStreamMute(ctx, int32(stream))

		muteStr := ""
		if muted {
			muteStr = " [MUTED]"
		}
		fmt.Printf("  %-12s %d / %d%s\n", name, vol, maxVol, muteStr)
	}
}
