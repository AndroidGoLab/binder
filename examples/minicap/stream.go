package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"net"
	"os"
	"sync"
	"syscall"

	"golang.org/x/sys/unix"
)

// projection describes source and requested output sizes and display rotation.
//
// Example: scale a 1080x2400 display into a 360x800 frame.
type projection struct {
	realWidth, realHeight, width, height int32
	rotation                             int
}

// streamBanner constructs the minicap v1 connection header.
//
// Example: advertise the frame geometry before sending JPEGs.
func streamBanner(pid int, p projection) [24]byte {
	var b [24]byte
	b[0], b[1] = 1, 24
	binary.LittleEndian.PutUint32(b[2:6], uint32(pid))
	binary.LittleEndian.PutUint32(b[6:10], uint32(p.realWidth))
	binary.LittleEndian.PutUint32(b[10:14], uint32(p.realHeight))
	binary.LittleEndian.PutUint32(b[14:18], uint32(p.width))
	binary.LittleEndian.PutUint32(b[18:22], uint32(p.height))
	b[22], b[23] = byte(p.rotation/90), 2
	return b
}

// encodeJPEG encodes tightly packed RGBA pixels at the requested quality.
//
// Example: encode a frame for a minicap client.
func encodeJPEG(w io.Writer, pixels []byte, width, height int32, quality int) error {
	if width <= 0 || height <= 0 || int64(width)*int64(height)*4 != int64(len(pixels)) {
		return errors.New("invalid RGBA frame size")
	}
	if quality < 0 || quality > 100 {
		return fmt.Errorf("invalid JPEG quality %d", quality)
	}
	img := &image.RGBA{Pix: pixels, Stride: int(width) * 4, Rect: image.Rect(0, 0, int(width), int(height))}
	return jpeg.Encode(w, img, &jpeg.Options{Quality: quality})
}

// writeJPEGFrame writes a little-endian size followed by one JPEG image.
//
// Example: send a frame to an established minicap client.
func writeJPEGFrame(w io.Writer, pixels []byte, width, height int32, quality int) error {
	var b bytes.Buffer
	return writeJPEGFrameBuffer(w, &b, pixels, width, height, quality)
}

// writeJPEGFrameBuffer reuses scratch space while writing one framed image.
//
// Example: reuse a socket stream's buffer across frames.
func writeJPEGFrameBuffer(w io.Writer, b *bytes.Buffer, pixels []byte, width, height int32, quality int) error {
	b.Reset()
	if err := encodeJPEG(b, pixels, width, height, quality); err != nil {
		return err
	}
	var prefix [4]byte
	binary.LittleEndian.PutUint32(prefix[:], uint32(b.Len()))
	if err := writeFrame(w, prefix[:]); err != nil {
		return err
	}
	return writeFrame(w, b.Bytes())
}

// socketStream sends JPEG frames to sequential clients on an abstract socket.
//
// Example: reconnect a client without restarting capture.
type socketStream struct {
	ctx           context.Context
	listener      *net.UnixListener
	mu            sync.Mutex
	conn          *net.UnixConn
	banner        [24]byte
	width, height int32
	quality       int
	jpegBuffer    bytes.Buffer
	stop          func() bool
}

// newSocketStream starts a cancelable local abstract socket server.
//
// Example: listen at localabstract:minicap.
func newSocketStream(ctx context.Context, name string, p projection, width, height int32, quality, pid int) (*socketStream, error) {
	if name == "" {
		return nil, errors.New("empty socket name")
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: "@" + name, Net: "unix"})
	if err != nil {
		return nil, err
	}
	s := &socketStream{ctx: ctx, listener: listener, banner: streamBanner(pid, p), width: width, height: height, quality: quality}
	s.stop = context.AfterFunc(ctx, func() { s.closeConnections() })
	return s, nil
}

// closeConnections interrupts pending accepts and writes.
//
// Example: unblock a slow client when capture is canceled.
func (s *socketStream) closeConnections() {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.listener.Close()
	if s.conn != nil {
		_ = s.conn.Close()
	}
}

// Close releases the listener and active connection.
//
// Example: remove the socket when capture ends.
func (s *socketStream) Close() error { s.stop(); s.closeConnections(); return nil }

// WaitForClient accepts a client before the next frame is acquired.
//
// Example: avoid retaining an old frame while waiting for a client.
func (s *socketStream) WaitForClient() (bool, error) {
	for {
		if err := s.ctx.Err(); err != nil {
			return false, err
		}
		s.mu.Lock()
		conn := s.conn
		s.mu.Unlock()
		if conn != nil {
			return false, nil
		}
		{
			c, err := s.listener.AcceptUnix()
			if err != nil {
				if s.ctx.Err() != nil {
					return false, s.ctx.Err()
				}
				return false, err
			}
			s.mu.Lock()
			if err := s.ctx.Err(); err != nil {
				s.mu.Unlock()
				_ = c.Close()
				return false, err
			}
			s.conn = c
			s.mu.Unlock()
			if err = writeFrame(c, s.banner[:]); err != nil {
				if err = s.disconnected(err); !errors.Is(err, errPeerDisconnected) {
					return false, err
				}
				continue
			}
			return true, nil
		}
	}
}

// errPeerDisconnected requests a new connection before acquiring another frame.
//
// Example: do not reuse a frame captured before reconnection.
var errPeerDisconnected = errors.New("client disconnected")

// Write sends one frame to the current client.
//
// Example: report a disconnected client without retaining its frame.
func (s *socketStream) Write(pixels []byte) (int, error) {
	if err := s.ctx.Err(); err != nil {
		return 0, err
	}
	s.mu.Lock()
	conn := s.conn
	s.mu.Unlock()
	if conn == nil {
		return 0, errPeerDisconnected
	}
	if err := writeJPEGFrameBuffer(conn, &s.jpegBuffer, pixels, s.width, s.height, s.quality); err != nil {
		return 0, s.disconnected(err)
	}
	return len(pixels), nil
}

// disconnected releases a closed peer and returns unexpected errors.
//
// Example: accept a new peer after a broken pipe.
func (s *socketStream) disconnected(err error) error {
	s.mu.Lock()
	c := s.conn
	s.conn = nil
	s.mu.Unlock()
	if c != nil {
		_ = c.Close()
	}
	if s.ctx.Err() != nil {
		return s.ctx.Err()
	}
	if errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ECONNRESET) {
		return errPeerDisconnected
	}
	return err
}

// errCaptureDone signals that a single-frame mode completed successfully.
//
// Example: end capture after writing a screenshot.
var errCaptureDone = errors.New("capture complete")

// singleFrameWriter outputs one JPEG or a successful capture test result.
//
// Example: write a screenshot to standard output.
type singleFrameWriter struct {
	out           io.Writer
	width, height int32
	quality       int
	test          bool
}

// Write handles one captured frame and terminates the capture loop.
//
// Example: output a bare JPEG with no stream framing.
func (w singleFrameWriter) Write(p []byte) (int, error) {
	if w.test {
		if _, err := io.WriteString(w.out, "OK\n"); err != nil {
			return 0, err
		}
	} else {
		if err := encodeJPEG(w.out, p, w.width, w.height, w.quality); err != nil {
			return 0, err
		}
	}
	return len(p), errCaptureDone
}

// rawFileWriter interrupts a blocked raw-output file when capture is canceled.
//
// Example: stop a write when the reader of a stdout pipe exits.
type rawFileWriter struct {
	ctx      context.Context
	file     *os.File
	original *os.File
	flags    int
	stop     func() bool
	done     chan struct{}
	closeErr error
}

// newRawFileWriter arranges for cancellation to close the owned output file.
//
// Example: unblock stdout when SIGTERM cancels capture.
func newRawFileWriter(ctx context.Context, file *os.File) (*rawFileWriter, error) {
	fd := int(file.Fd())
	flags, err := unix.FcntlInt(uintptr(fd), unix.F_GETFL, 0)
	if err != nil {
		return nil, fmt.Errorf("read stdout flags: %w", err)
	}
	dup, err := unix.FcntlInt(uintptr(fd), unix.F_DUPFD_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("duplicate stdout: %w", err)
	}
	if err := unix.SetNonblock(dup, true); err != nil {
		_ = unix.Close(dup)
		return nil, fmt.Errorf("set stdout nonblocking: %w", err)
	}
	stream := os.NewFile(uintptr(dup), "raw-output")
	w := &rawFileWriter{ctx: ctx, file: stream, original: file, flags: flags, done: make(chan struct{})}
	w.stop = context.AfterFunc(ctx, func() { defer close(w.done); w.closeErr = stream.Close() })
	return w, nil
}

// Write forwards bytes and reports cancellation after an interrupted write.
//
// Example: retain partial-write counts for the capture loop.
func (w *rawFileWriter) Write(p []byte) (int, error) {
	n, err := w.file.Write(p)
	if err != nil && w.ctx.Err() != nil {
		return n, w.ctx.Err()
	}
	return n, err
}

// Close synchronizes with cancellation without closing a still-active file.
//
// Example: avoid a late callback closing a reused file descriptor.
func (w *rawFileWriter) Close() error {
	if w.stop() {
		w.closeErr = w.file.Close()
	} else {
		<-w.done
	}
	fd := w.original.Fd()
	flags, err := unix.FcntlInt(fd, unix.F_GETFL, 0)
	if err != nil {
		return errors.Join(w.closeErr, err)
	}
	flags = flags&^unix.O_NONBLOCK | w.flags&unix.O_NONBLOCK
	_, err = unix.FcntlInt(fd, unix.F_SETFL, flags)
	return errors.Join(w.closeErr, err)
}
