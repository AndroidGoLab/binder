package screencapture

import (
	"context"
	"fmt"

	"golang.org/x/sys/unix"
)

// waitFence waits for a native fence or context cancellation.
//
// Example: stop waiting if the capture request is canceled.
func waitFence(
	ctx context.Context,
	fd int32,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if fd < 0 {
		return nil
	}
	var cancelFDs [2]int
	if err := unix.Pipe2(cancelFDs[:], unix.O_CLOEXEC|unix.O_NONBLOCK); err != nil {
		return fmt.Errorf("create cancellation pipe: %w", err)
	}
	done := make(chan struct{})
	stop := context.AfterFunc(ctx,
		// Wake the poll when the context is canceled.
		//
		// Example: a canceled request interrupts an unsignaled fence.
		func() { _, _ = unix.Write(cancelFDs[1], []byte{1}); close(done) },
	)
	defer // Wait for a concurrent cancellation write before closing the pipe.
	//
	// Example: a late cancellation cannot write to a reused descriptor.
	func() {
		if !stop() {
			<-done
		}
		_ = unix.Close(cancelFDs[0])
		_ = unix.Close(cancelFDs[1])
	}()
	pollFDs := []unix.PollFd{{Fd: fd, Events: unix.POLLIN}, {Fd: int32(cancelFDs[0]), Events: unix.POLLIN}}
	for {
		_, err := unix.Poll(pollFDs, -1)
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			return fmt.Errorf("wait for capture fence: %w", err)
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if pollFDs[0].Revents&(unix.POLLERR|unix.POLLNVAL) != 0 {
			return fmt.Errorf("capture fence poll failed: %#x", pollFDs[0].Revents)
		}
		if pollFDs[0].Revents&unix.POLLIN != 0 {
			return nil
		}
		if pollFDs[0].Revents != 0 {
			return fmt.Errorf("capture fence poll failed: %#x", pollFDs[0].Revents)
		}
	}
}
