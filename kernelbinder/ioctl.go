//go:build linux

package kernelbinder

// ioctl direction bits.
const (
	iocNone  = 0
	iocWrite = 1
	iocRead  = 2
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

func ioc(dir, typ, nr, size uintptr) uintptr {
	return (dir << iocDirShift) | (typ << iocTypeShift) | (nr << iocNRShift) | (size << iocSizeShift)
}

func iow(typ, nr, size uintptr) uintptr  { return ioc(iocWrite, typ, nr, size) }
func ior(typ, nr, size uintptr) uintptr  { return ioc(iocRead, typ, nr, size) }
func iowr(typ, nr, size uintptr) uintptr { return ioc(iocRead|iocWrite, typ, nr, size) }
