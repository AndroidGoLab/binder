package gralloc

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/AndroidGoLab/binder/android/hardware/common"
	"github.com/AndroidGoLab/binder/android/hardware/graphics/allocator"
	gfx "github.com/AndroidGoLab/binder/android/hardware/graphics/common"
	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/parcel"
	"github.com/AndroidGoLab/binder/servicemanager"
	"golang.org/x/sys/unix"
)

const (
	// pixelDescriptorBytes is the Android 14 gchips gralloc4 descriptor size.
	//
	// Example: its final byte contains the empty descriptor name.
	pixelDescriptorBytes = 37
	// pixelDescriptorVersion identifies the gchips gralloc4 wire ABI.
	//
	// Example: the first descriptor word contains version 40.
	pixelDescriptorVersion = 40
	// pixelRGBAUsage requests CPU reads and GPU/compositor output.
	//
	// Example: a virtual display renders into a CPU-readable RGBA buffer.
	pixelRGBAUsage = uint64(gfx.BufferUsageCpuReadOften | gfx.BufferUsageGpuTexture | gfx.BufferUsageGpuRenderTarget | gfx.BufferUsageComposerOverlay)
)

// pixelDescriptor serializes the Android 14 gchips allocator descriptor.
//
// Example: encode a 360 by 800 RGBA8888 frame request.
func pixelDescriptor(width, height int32) ([]byte, error) {
	if width <= 0 || height <= 0 {
		return nil, errors.New("buffer dimensions must be positive")
	}
	desc := make([]byte, pixelDescriptorBytes)
	binary.LittleEndian.PutUint32(desc, pixelDescriptorVersion)
	binary.LittleEndian.PutUint32(desc[4:], uint32(width))
	binary.LittleEndian.PutUint32(desc[8:], uint32(height))
	binary.LittleEndian.PutUint32(desc[12:], 1)
	binary.LittleEndian.PutUint32(desc[16:], uint32(gfx.PixelFormatRgba8888))
	binary.LittleEndian.PutUint64(desc[20:], pixelRGBAUsage)
	return desc, nil
}

// AllocatePixelRGBA allocates Android 14 gchips linear RGBA capture buffers.
// The caller owns the returned native descriptors and must call CloseBuffers.
//
// Example: allocate four buffers for a Pixel 6a virtual display queue.
func AllocatePixelRGBA(ctx context.Context, sm *servicemanager.ServiceManager, api int, width, height int32, count int) (_ []*Buffer, resultErr error) {
	if api != 34 {
		return nil, fmt.Errorf("Pixel gralloc4 allocator requires Android API 34, detected %d", api)
	}
	if sm == nil || count < 1 || count > 64 {
		return nil, errors.New("invalid service manager or buffer count")
	}
	desc, err := pixelDescriptor(width, height)
	if err != nil {
		return nil, err
	}
	svc, err := sm.GetService(ctx, "android.hardware.graphics.allocator.IAllocator/default")
	if err != nil {
		return nil, fmt.Errorf("get Pixel allocator: %w", err)
	}
	req := parcel.New()
	req.WriteInterfaceToken(allocator.DescriptorIAllocator)
	req.WriteByteArray(desc)
	req.WriteInt32(int32(count))
	reply, err := svc.Transact(ctx, allocator.TransactionIAllocatorAllocate, 0, req)
	if err != nil {
		return nil, fmt.Errorf("allocate Pixel buffers: %w", err)
	}
	defer // Release unread descriptors and the transaction parcel.
	//
	// Example: reject a truncated reply without leaking its remaining FDs.
	func() { resultErr = errors.Join(resultErr, reply.Recycle()) }()
	if err = binder.ReadStatus(reply); err != nil {
		return nil, fmt.Errorf("Pixel allocator status: %w", err)
	}
	present, err := reply.ReadInt32()
	if err != nil || present != 1 {
		return nil, errors.Join(errors.New("invalid Pixel allocation result"), err)
	}
	stride, handles, err := readPixelAllocation(reply, count)
	if err != nil {
		return nil, err
	}
	buffers := make([]*Buffer, 0, len(handles))
	for _, h := range handles {
		buffers = append(buffers, &Buffer{Handle: h, Stride: stride, Width: uint32(width), Height: uint32(height), Format: int32(gfx.PixelFormatRgba8888), Usage: pixelRGBAUsage})
	}
	for _, b := range buffers {
		if _, err = pixelRGBALayout(b); err != nil {
			return nil, errors.Join(fmt.Errorf("unsupported Pixel allocation: %w", err), CloseBuffers(buffers))
		}
	}
	return buffers, nil
}

// readPixelAllocation transfers the allocation's descriptors with error cleanup.
//
// Example: a malformed second handle closes descriptors from the first handle.
func readPixelAllocation(p *parcel.Parcel, expected int) (_ int32, _ []common.NativeHandle, resultErr error) {
	var owned []int32
	defer // Close transferred descriptors unless ownership is returned to the caller.
	//
	// Example: truncated native handles do not leak already decoded FDs.
	func() {
		if resultErr != nil {
			for _, fd := range owned {
				resultErr = errors.Join(resultErr, unix.Close(int(fd)))
			}
		}
	}()
	end, err := parcel.ReadParcelableHeader(p)
	if err != nil {
		return 0, nil, err
	}
	defer parcel.SkipToParcelableEnd(p, end)
	stride, err := p.ReadInt32()
	if err != nil {
		return 0, nil, err
	}
	n, err := p.ReadInt32()
	if err != nil {
		return 0, nil, err
	}
	if n != int32(expected) {
		return 0, nil, fmt.Errorf("allocator returned %d buffers, expected %d", n, expected)
	}
	handles := make([]common.NativeHandle, 0, expected)
	for i := 0; i < expected; i++ {
		present, err := p.ReadInt32()
		if err != nil || present != 1 {
			return 0, nil, errors.Join(errors.New("invalid allocation handle"), err)
		}
		handleEnd, err := parcel.ReadParcelableHeader(p)
		if err != nil {
			return 0, nil, err
		}
		nf, err := p.ReadInt32()
		if err != nil {
			return 0, nil, err
		}
		if nf < 0 || nf > 64 {
			return 0, nil, errors.New("invalid allocation descriptor count")
		}
		h := common.NativeHandle{}
		for j := int32(0); j < nf; j++ {
			fd, err := readOwnedParcelDescriptor(p)
			if err != nil {
				return 0, nil, err
			}
			if fd < 0 {
				return 0, nil, errors.New("null allocation descriptor")
			}
			owned = append(owned, fd)
			h.Fds = append(h.Fds, fd)
		}
		ni, err := p.ReadInt32()
		if err != nil {
			return 0, nil, err
		}
		if ni < 0 || ni > 1024 {
			return 0, nil, errors.New("invalid allocation integer count")
		}
		for j := int32(0); j < ni; j++ {
			v, err := p.ReadInt32()
			if err != nil {
				return 0, nil, err
			}
			h.Ints = append(h.Ints, v)
		}
		parcel.SkipToParcelableEnd(p, handleEnd)
		handles = append(handles, h)
	}
	return stride, handles, nil
}

// CloseBuffers closes descriptors held by buffers allocated for display capture.
//
// Example: close buffers only after the frame producer and all leases have stopped.
func CloseBuffers(buffers []*Buffer) error {
	var errs []error
	for _, b := range buffers {
		if b == nil {
			continue
		}
		for _, fd := range b.Handle.Fds {
			if fd >= 0 {
				if err := unix.Close(int(fd)); err != nil {
					errs = append(errs, fmt.Errorf("close buffer descriptor: %w", err))
				}
			}
		}
		b.Handle.Fds = nil
	}
	return errors.Join(errs...)
}

// readOwnedParcelDescriptor transfers or duplicates a ParcelFileDescriptor.
//
// Example: decoded buffers remain valid after a borrowed reply is released.
func readOwnedParcelDescriptor(p *parcel.Parcel) (_ int32, resultErr error) {
	present, err := p.ReadInt32()
	if err != nil {
		return -1, err
	}
	if present != 1 {
		return -1, errors.New("null allocation descriptor")
	}
	hasComm, err := p.ReadInt32()
	if err != nil {
		return -1, err
	}
	if hasComm != 0 && hasComm != 1 {
		return -1, errors.New("invalid communication descriptor flag")
	}
	fd, err := p.ReadOwnedFileDescriptor()
	if err != nil {
		return -1, err
	}
	defer // Close the primary descriptor if the optional channel fails to decode.
	//
	// Example: malformed communication metadata does not leak the pixel FD.
	func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, unix.Close(int(fd)))
		}
	}()
	if hasComm == 1 {
		comm, err := p.ReadOwnedFileDescriptor()
		if err != nil {
			return -1, err
		}
		if err = unix.Close(int(comm)); err != nil {
			return -1, err
		}
	}
	return fd, nil
}
