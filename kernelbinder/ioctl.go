//go:build linux

package kernelbinder

// ioctlDirection represents the direction bits of an ioctl request code.
type ioctlDirection uintptr

// ioctl direction bits.
const (
	iocNone  ioctlDirection = 0
	iocWrite ioctlDirection = 1
	iocRead  ioctlDirection = 2
)

// ioctl field widths and shifts.
const (
	iocNRBits   = 8
	iocTypeBits = 8
	iocSizeBits = 14
	iocDirBits  = 2

	iocNRShift   = 0
	iocTypeShift = iocNRShift + iocNRBits     // 8
	iocSizeShift = iocTypeShift + iocTypeBits  // 16
	iocDirShift  = iocSizeShift + iocSizeBits  // 30
)

func ioc(
	dir ioctlDirection,
	typ uintptr,
	nr uintptr,
	size uintptr,
) uintptr {
	return (uintptr(dir) << iocDirShift) | (typ << iocTypeShift) | (nr << iocNRShift) | (size << iocSizeShift)
}

func iow(
	typ uintptr,
	nr uintptr,
	size uintptr,
) uintptr {
	return ioc(iocWrite, typ, nr, size)
}

func ior(
	typ uintptr,
	nr uintptr,
	size uintptr,
) uintptr {
	return ioc(iocRead, typ, nr, size)
}

func iowr(
	typ uintptr,
	nr uintptr,
	size uintptr,
) uintptr {
	return ioc(iocRead|iocWrite, typ, nr, size)
}
