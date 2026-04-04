package gralloc

import (
	"context"
	"fmt"

	gfxCommon "github.com/AndroidGoLab/binder/android/hardware/graphics/common"
	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/gralloc/hidlmapper"
	"github.com/AndroidGoLab/binder/kernelbinder"
	"github.com/AndroidGoLab/binder/logger"

	"golang.org/x/sys/unix"
)

// hidlMapper implements Mapper via HIDL IMapper@3.0 over hwbinder.
//
// The mapper's lock() triggers rcReadColorBuffer on goldfish/ranchu emulators,
// which fetches pixel data from the host GPU into the shared goldfish address
// space memory. After lock()+unlock(), the pixel data is readable from
// the mmap'd region (fast path) or via pread at the goldfish offset
// (fallback when mmap fails with EPERM).
type hidlMapper struct {
	transport binder.Transport
	mapper    *hidlmapper.Mapper
}

var _ Mapper = (*hidlMapper)(nil)

func newHIDLMapper(ctx context.Context) (*hidlMapper, error) {
	hwDriver, err := kernelbinder.Open(ctx,
		binder.WithDevicePath("/dev/hwbinder"),
		binder.WithMapSize(256*1024),
	)
	if err != nil {
		return nil, fmt.Errorf("open hwbinder: %w", err)
	}

	mapper, err := hidlmapper.New(ctx, hwDriver)
	if err != nil {
		if closeErr := hwDriver.Close(ctx); closeErr != nil {
			logger.Warnf(ctx, "close hwbinder after mapper init failure: %v", closeErr)
		}
		return nil, fmt.Errorf("hidlmapper.New: %w", err)
	}

	return &hidlMapper{
		transport: hwDriver,
		mapper:    mapper,
	}, nil
}

// LockBuffer triggers the IMapper to fetch pixel data from the host GPU
// into the buffer's shared memory, then reads the data.
//
// The sequence is:
//  1. importBuffer -- register the native handle with the mapper
//  2. lock or lockYCbCr -- triggers rcReadColorBuffer on the host GPU,
//     writing pixel data into the shared goldfish address space memory
//  3. unlock -- release the CPU lock
//  4. freeBuffer -- release the imported handle
//  5. read pixel data -- either from the mmap'd region (fast path) or
//     via pread from the goldfish FD at the buffer's offset (fallback
//     when mmap failed due to kernel EPERM)
func (m *hidlMapper) LockBuffer(ctx context.Context, b *Buffer) ([]byte, error) {
	if len(b.Handle.Fds) == 0 || len(b.Handle.Ints) == 0 {
		return nil, fmt.Errorf("empty handle")
	}

	bufToken, err := m.mapper.ImportBuffer(ctx, b.Handle.Fds, b.Handle.Ints)
	if err != nil {
		return nil, fmt.Errorf("importBuffer: %w", err)
	}
	defer func() {
		if freeErr := m.mapper.FreeBuffer(ctx, bufToken); freeErr != nil {
			logger.Warnf(ctx, "freeBuffer: %v", freeErr)
		}
	}()

	// Choose lock variant based on pixel format.
	switch gfxCommon.PixelFormat(b.Format) {
	case gfxCommon.PixelFormatYcbcr420888:
		err = m.mapper.LockYCbCr(ctx, bufToken, int32(b.Width), int32(b.Height))
	default:
		err = m.mapper.Lock(ctx, bufToken, int32(b.Width), int32(b.Height))
	}
	if err != nil {
		return nil, fmt.Errorf("lock: %w", err)
	}

	defer func() {
		if unlockErr := m.mapper.Unlock(ctx, bufToken); unlockErr != nil {
			logger.Warnf(ctx, "unlock: %v", unlockErr)
		}
	}()

	// The lock triggered rcReadColorBuffer, populating the shared
	// goldfish address space memory. Read pixels from the best
	// available source.

	// Fast path: mmap is available -- copy directly.
	if b.MmapData != nil {
		out := make([]byte, len(b.MmapData))
		copy(out, b.MmapData)
		return out, nil
	}

	// Fallback: mmap failed (common on goldfish where the kernel denies
	// mmap with EPERM). Use pread on the goldfish FD at the buffer's
	// address space offset to read the pixel data that rcReadColorBuffer
	// wrote into the shared region.
	return m.preadGoldfishPixels(ctx, b)
}

// preadGoldfishPixels reads pixel data from a goldfish address space FD
// via pread at the buffer's claimed offset. This is the fallback path
// when mmap of the goldfish FD fails but the shared region was
// successfully claimed.
func (m *hidlMapper) preadGoldfishPixels(ctx context.Context, b *Buffer) ([]byte, error) {
	if !b.goldfishClaimed {
		return nil, fmt.Errorf("buffer not mmap'd and not a claimed goldfish buffer")
	}

	fd := int(b.Handle.Fds[0])
	bufSize := b.BufferSize()
	out := make([]byte, bufSize)

	n, err := unix.Pread(fd, out, int64(b.goldfishOffset))
	if err != nil {
		return nil, fmt.Errorf("pread goldfish fd=%d offset=0x%x size=%d: %w",
			fd, b.goldfishOffset, bufSize, err)
	}
	logger.Debugf(ctx, "pread goldfish buffer: %d/%d bytes read", n, bufSize)

	return out[:n], nil
}
