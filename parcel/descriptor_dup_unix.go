//go:build unix

package parcel

import "golang.org/x/sys/unix"

// duplicateFileDescriptor creates a close-on-exec duplicate of a borrowed FD.
//
// Example: give a decoded native object an independently owned descriptor.
func duplicateFileDescriptor(fd int32) (int32, error) {
	duplicated, err := unix.FcntlInt(uintptr(fd), unix.F_DUPFD_CLOEXEC, 0)
	return int32(duplicated), err
}
