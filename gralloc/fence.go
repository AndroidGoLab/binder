package gralloc

import (
	"context"
	"errors"
	"fmt"

	"golang.org/x/sys/unix"
)

// WaitFence waits for a sync fence to signal or the context to be canceled.
// The caller retains ownership of fd. A descriptor of -1 denotes no fence.
//
// Example: wait for rendering before reading a captured graphics buffer.
func WaitFence(
	ctx context.Context,
	fd int32,
) (resultErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if fd == -1 {
		return nil
	}
	if fd < -1 {
		return fmt.Errorf("invalid fence descriptor %d", fd)
	}
	// Duplicate before creating the wake pipe so an invalid descriptor cannot
	// be silently reused as the pipe, and the fence stays stable during poll.
	fenceFD, err := unix.FcntlInt(uintptr(fd), unix.F_DUPFD_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("duplicate fence descriptor: %w", err)
	}
	defer // Release the duplicate without taking ownership of the caller's fd.
	//
	// Example: the caller can reuse its fence after the wait.
	func() {
		if err := unix.Close(fenceFD); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("close fence duplicate: %w", err))
		}
	}()
	var cancelFDs [2]int
	if err := unix.Pipe2(cancelFDs[:], unix.O_CLOEXEC|unix.O_NONBLOCK); err != nil {
		return fmt.Errorf("create fence cancellation pipe: %w", err)
	}
	done := make(chan struct{})
	var wakeErr error
	stop := context.AfterFunc(ctx,
		// Wake a blocked poll when the caller cancels.
		//
		// Example: cancellation interrupts an unsignaled fence.
		func() {
			defer close(done)
			for {
				_, wakeErr = unix.Write(cancelFDs[1], []byte{1})
				if !errors.Is(wakeErr, unix.EINTR) {
					return
				}
			}
		},
	)
	defer // Synchronize with a pending cancellation callback before closing its pipe.
	//
	// Example: no cancellation write can target a reused descriptor.
	func() {
		if !stop() {
			<-done
		}
		if wakeErr != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("wake fence wait: %w", wakeErr))
		}
		if err := unix.Close(cancelFDs[0]); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("close fence cancellation reader: %w", err))
		}
		if err := unix.Close(cancelFDs[1]); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("close fence cancellation writer: %w", err))
		}
	}()
	fds := []unix.PollFd{{Fd: int32(fenceFD), Events: unix.POLLIN}, {Fd: int32(cancelFDs[0]), Events: unix.POLLIN}}
	for {
		_, err := unix.Poll(fds, -1)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return fmt.Errorf("wait for fence: %w", err)
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		if fds[0].Revents&(unix.POLLERR|unix.POLLNVAL) != 0 {
			return fmt.Errorf("fence poll failed: %#x", fds[0].Revents)
		}
		if fds[0].Revents&unix.POLLIN != 0 {
			return nil
		}
		if fds[0].Revents != 0 {
			return fmt.Errorf("unexpected fence poll events: %#x", fds[0].Revents)
		}
	}
}
