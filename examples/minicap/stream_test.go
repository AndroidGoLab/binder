package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"image/jpeg"
	"io"
	"net"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestBanner verifies the documented minicap v1 wire layout.
func TestBanner(t *testing.T) {
	b := streamBanner(42, projection{realWidth: 1080, realHeight: 2400, width: 360, height: 800, rotation: 90})
	require.Equal(t, []byte{1, 24}, b[:2])
	require.EqualValues(t, 42, binary.LittleEndian.Uint32(b[2:6]))
	require.EqualValues(t, 1080, binary.LittleEndian.Uint32(b[6:10]))
	require.EqualValues(t, 2400, binary.LittleEndian.Uint32(b[10:14]))
	require.EqualValues(t, 360, binary.LittleEndian.Uint32(b[14:18]))
	require.EqualValues(t, 800, binary.LittleEndian.Uint32(b[18:22]))
	require.EqualValues(t, 1, b[22])
	require.EqualValues(t, 2, b[23])
}

// TestJPEGFrame validates frame length and that payload is a decodable JPEG.
func TestJPEGFrame(t *testing.T) {
	var out bytes.Buffer
	pixels := []byte{255, 0, 0, 255, 0, 255, 0, 255, 0, 0, 255, 255, 255, 255, 255, 255}
	require.NoError(t, writeJPEGFrame(&out, pixels, 2, 2, 80))
	data := out.Bytes()
	require.Greater(t, len(data), 4)
	require.EqualValues(t, len(data)-4, binary.LittleEndian.Uint32(data[:4]))
	img, err := jpeg.Decode(bytes.NewReader(data[4:]))
	require.NoError(t, err)
	require.Equal(t, 2, img.Bounds().Dx())
	require.Equal(t, 2, img.Bounds().Dy())
	require.Error(t, writeJPEGFrame(&bytes.Buffer{}, pixels[:4], 2, 2, 80))
}

// TestSocketStreamCancellation closes a waiting client write and listener.
func TestSocketStreamCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s, err := newSocketStream(ctx, fmt.Sprintf("binder-test-%d", os.Getpid()), projection{2, 2, 2, 2, 0}, 2, 2, 80, 123)
	require.NoError(t, err)
	defer s.Close()
	done := make(chan error, 1)
	go func() {
		_, err := s.WaitForClient()
		if err == nil {
			_, err = s.Write(make([]byte, 16))
		}
		done <- err
	}()
	c, err := net.Dial("unix", "@"+fmt.Sprintf("binder-test-%d", os.Getpid()))
	require.NoError(t, err)
	banner := make([]byte, 24)
	_, err = io.ReadFull(c, banner)
	require.NoError(t, err)
	require.Equal(t, byte(1), banner[0])
	nbuf := make([]byte, 4)
	_, err = io.ReadFull(c, nbuf)
	require.NoError(t, err)
	frame := make([]byte, binary.LittleEndian.Uint32(nbuf))
	_, err = io.ReadFull(c, frame)
	require.NoError(t, err)
	_, err = jpeg.Decode(bytes.NewReader(frame))
	require.NoError(t, err)
	require.NoError(t, <-done)
	done = make(chan error, 1)
	go func() { _, err := s.Write(make([]byte, 16)); done <- err }()
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
	require.NoError(t, c.Close())
}

// TestSingleFrameModes checks screenshot JPEG and test-mode output.
func TestSingleFrameModes(t *testing.T) {
	var screenshot bytes.Buffer
	w := singleFrameWriter{out: &screenshot, width: 2, height: 2, quality: 80}
	n, err := w.Write(make([]byte, 16))
	require.Equal(t, 16, n)
	require.ErrorIs(t, err, errCaptureDone)
	_, err = jpeg.Decode(bytes.NewReader(screenshot.Bytes()))
	require.NoError(t, err)
	var testOutput bytes.Buffer
	w = singleFrameWriter{out: &testOutput, test: true, width: 2, height: 2, quality: 80}
	_, err = w.Write(make([]byte, 16))
	require.ErrorIs(t, err, errCaptureDone)
	require.Equal(t, "OK\n", testOutput.String())
}

// TestSocketStreamReconnect sends a new banner and JPEG after a client closes.
func TestSocketStreamReconnect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	name := fmt.Sprintf("binder-reconnect-%d", os.Getpid())
	s, err := newSocketStream(ctx, name, projection{2, 2, 2, 2, 0}, 2, 2, 80, 123)
	require.NoError(t, err)
	defer s.Close()
	send := func() <-chan error {
		done := make(chan error, 1)
		go func() {
			_, err := s.WaitForClient()
			if err == nil {
				_, err = s.Write(make([]byte, 16))
			}
			done <- err
		}()
		return done
	}
	receive := func() net.Conn {
		c, err := net.Dial("unix", "@"+name)
		require.NoError(t, err)
		banner := make([]byte, 24)
		_, err = io.ReadFull(c, banner)
		require.NoError(t, err)
		require.Equal(t, byte(1), banner[0])
		prefix := make([]byte, 4)
		_, err = io.ReadFull(c, prefix)
		require.NoError(t, err)
		data := make([]byte, binary.LittleEndian.Uint32(prefix))
		_, err = io.ReadFull(c, data)
		require.NoError(t, err)
		_, err = jpeg.Decode(bytes.NewReader(data))
		require.NoError(t, err)
		return c
	}
	first := send()
	c := receive()
	require.NoError(t, <-first)
	require.NoError(t, c.Close())
	_, err = s.Write(make([]byte, 16))
	require.ErrorIs(t, err, errPeerDisconnected)
	second := send()
	c = receive()
	require.NoError(t, <-second)
	require.NoError(t, c.Close())
}

// TestRawPipeCancellation interrupts a raw frame write when its reader stalls.
func TestRawPipeCancellation(t *testing.T) {
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	defer reader.Close()
	ctx, cancel := context.WithCancel(context.Background())
	w := newRawFileWriter(ctx, writer)
	defer w.Close()
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() { close(started); _, err := w.Write(make([]byte, 4<<20)); done <- err }()
	<-started
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
}
