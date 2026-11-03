//go:build android && arm64

package gralloc

import (
	"errors"
	"testing"

	"github.com/go-webgpu/goffi/ffi"
	"golang.org/x/sys/unix"
)

// TestAHBOwnerUnlockFailureRetainsResources verifies the real owner quarantines
// a locked allocation and its descriptors until a later unlock succeeds.
//
// Example: a frame remains owned after its CPU unlock fails.
func TestAHBOwnerUnlockFailureRetainsResources(t *testing.T) {
	buffers, err := allocateNativeRGBA(8, 8, 1)
	if err != nil {
		t.Fatal(err)
	}
	b := buffers[0]
	o := b.native.(*ahbOwner)
	originalLib := o.lib
	defer func() {
		o.lib = originalLib
		if err := b.Close(); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	}()
	if len(o.fds) == 0 {
		t.Fatal("allocation has no transported descriptors")
	}
	fds := append([]int32(nil), o.fds...)

	libc, err := ffi.LoadLibrary("libc.so")
	if err != nil {
		t.Fatal(err)
	}
	getpid, err := ffi.GetSymbol(libc, "getpid")
	if err != nil {
		t.Fatal(err)
	}
	fakeLib := *originalLib
	fakeLib.unlock.symbol = getpid
	o.lib = &fakeLib
	if err := o.copyInto(make([]byte, 8*8*4)); err == nil {
		t.Fatal("copy accepted failed unlock")
	}
	if !o.locked || o.ptr == nil {
		t.Fatal("failed unlock did not retain the locked allocation")
	}
	if err := b.Close(); err == nil {
		t.Fatal("close accepted failed unlock")
	}
	for _, fd := range fds {
		if _, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0); err != nil {
			t.Fatalf("descriptor %d closed before unlock: %v", fd, err)
		}
	}

	o.lib = originalLib
	if err := b.FinishCPUAccess(); err != nil {
		t.Fatalf("retry unlock: %v", err)
	}
	if o.locked {
		t.Fatal("successful retry left allocation locked")
	}
	if err := b.Close(); err != nil {
		t.Fatalf("close after unlock: %v", err)
	}
	if err := b.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
	for _, fd := range fds {
		if _, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0); !errors.Is(err, unix.EBADF) {
			t.Fatalf("descriptor %d still open after close: %v", fd, err)
		}
	}
}
