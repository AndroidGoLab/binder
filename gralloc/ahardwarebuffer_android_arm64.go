//go:build android && arm64

package gralloc

import (
	"errors"
	"fmt"
	"math"
	"runtime"
	"sync"
	"unsafe"

	common "github.com/AndroidGoLab/binder/android/hardware/common"
	"github.com/go-webgpu/goffi/ffi"
	"github.com/go-webgpu/goffi/types"
	"golang.org/x/sys/unix"
)

// ahbDesc mirrors the Android AHardwareBuffer_Desc ABI.
//
// Example: describe an allocated RGBA buffer.
type ahbDesc struct {
	Width, Height, Layers, Format uint32
	Usage                         uint64
	Stride, Reserved              uint32
	Reserved2                     uint64
}

// ahbFunction retains a prepared native function call.
//
// Example: invoke the native lock function.
type ahbFunction struct {
	symbol unsafe.Pointer
	cif    types.CallInterface
}

// ahbLibrary holds native buffer entry points.
//
// Example: share one loaded library across allocations.
type ahbLibrary struct{ allocate, describe, release, lock, unlock, send ahbFunction }

// ahbOnce serializes process-wide library loading.
var ahbOnce sync.Once

// ahbLib holds the process-wide native entry points.
var ahbLib *ahbLibrary

// ahbErr records the first library loading failure.
var ahbErr error

// loadAHB loads and prepares the Android hardware buffer functions once.
//
// Example: reuse the prepared functions for multiple allocations.
func loadAHB() (*ahbLibrary, error) {
	ahbOnce.Do( // Initialize native symbols exactly once.
		//
		// Example: subsequent callers reuse the same entry points.
		func() {
			lib, err := ffi.LoadLibrary("libnativewindow.so")
			if err != nil {
				ahbErr = err
				return
			}
			l := new(ahbLibrary)
			p, i, u := types.PointerTypeDescriptor, types.SInt32TypeDescriptor, types.UInt64TypeDescriptor
			defs := []struct {
				dst    *ahbFunction
				name   string
				result *types.TypeDescriptor
				args   []*types.TypeDescriptor
			}{
				{&l.allocate, "AHardwareBuffer_allocate", i, []*types.TypeDescriptor{p, p}},
				{&l.describe, "AHardwareBuffer_describe", types.VoidTypeDescriptor, []*types.TypeDescriptor{p, p}},
				{&l.release, "AHardwareBuffer_release", types.VoidTypeDescriptor, []*types.TypeDescriptor{p}},
				{&l.lock, "AHardwareBuffer_lock", i, []*types.TypeDescriptor{p, u, i, p, p}},
				{&l.unlock, "AHardwareBuffer_unlock", i, []*types.TypeDescriptor{p, p}},
				{&l.send, "AHardwareBuffer_sendHandleToUnixSocket", i, []*types.TypeDescriptor{p, i}},
			}
			for _, d := range defs {
				d.dst.symbol, err = ffi.GetSymbol(lib, d.name)
				if err == nil {
					err = ffi.PrepareCallInterface(&d.dst.cif, types.DefaultCall, d.result, d.args)
				}
				if err != nil {
					ahbErr = fmt.Errorf("%s: %w", d.name, err)
					return
				}
			}
			ahbLib = l
		})
	return ahbLib, ahbErr
}

// call invokes a prepared function and returns FFI errors.
//
// Example: interpret an Android status separately from FFI errors.
func (f *ahbFunction) call(result unsafe.Pointer, args ...unsafe.Pointer) error {
	_, err := ffi.CallFunction(&f.cif, f.symbol, result, args)
	return err
}

// ahbOwner owns an allocation, transported descriptors, and CPU lock state.
//
// Example: retain an allocation until every frame lease ends.
type ahbOwner struct {
	mu     sync.Mutex
	lib    *ahbLibrary
	ptr    unsafe.Pointer
	desc   ahbDesc
	raw    []byte
	fds    []int32
	locked bool
}

// flattened copies the original serialized GraphicBuffer metadata.
//
// Example: preserve the native identity during Binder transport.
func (o *ahbOwner) flattened() []byte { return append([]byte(nil), o.raw...) }

// unlock finishes CPU access while the owner mutex is held.
//
// Example: keep the allocation quarantined if unlocking fails.
func (o *ahbOwner) unlock() error {
	var status int32
	var null unsafe.Pointer
	if err := o.lib.unlock.call(unsafe.Pointer(&status), unsafe.Pointer(&o.ptr), unsafe.Pointer(&null)); err != nil {
		return err
	}
	if status != 0 {
		return fmt.Errorf("AHardwareBuffer_unlock: %d", status)
	}
	o.locked = false
	return nil
}

// finish retries outstanding CPU unlock before buffer reuse.
//
// Example: refuse to release a frame when unlock still fails.
func (o *ahbOwner) finish() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.locked {
		return o.unlock()
	}
	return nil
}

// close releases native and descriptor resources once CPU access ends.
//
// Example: preserve a locked buffer for a later cleanup attempt.
func (o *ahbOwner) close() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.ptr == nil {
		return nil
	}
	if o.locked {
		if err := o.unlock(); err != nil {
			return err
		}
	}
	if err := o.lib.release.call(nil, unsafe.Pointer(&o.ptr)); err != nil {
		return err
	}
	o.ptr = nil
	var errs []error
	for _, fd := range o.fds {
		if err := unix.Close(int(fd)); err != nil {
			errs = append(errs, err)
		}
	}
	o.fds = nil
	return errors.Join(errs...)
}

// copyInto locks, copies packed RGBA rows, and unlocks the allocation.
//
// Example: remove padding between native buffer rows.
func (o *ahbOwner) copyInto(dst []byte) (result error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.ptr == nil {
		return errors.New("buffer closed")
	}
	if o.locked {
		return errors.New("buffer CPU access remains locked")
	}
	if err := validateRGBASize(o.desc.Width, o.desc.Height, dst); err != nil {
		return err
	}
	if o.desc.Stride < o.desc.Width || uint64(o.desc.Stride)*uint64(o.desc.Height) > uint64(math.MaxInt)/4 {
		return errors.New("invalid buffer stride")
	}
	usage := uint64(3)
	fence := int32(-1)
	var null, pixels unsafe.Pointer
	out := unsafe.Pointer(&pixels)
	var status int32
	if err := o.lib.lock.call(unsafe.Pointer(&status), unsafe.Pointer(&o.ptr), unsafe.Pointer(&usage), unsafe.Pointer(&fence), unsafe.Pointer(&null), unsafe.Pointer(&out)); err != nil {
		return err
	}
	if status != 0 {
		return fmt.Errorf("AHardwareBuffer_lock: %d", status)
	}
	o.locked = true
	defer // Preserve an unlock error even when copying also fails.
	//
	// Example: quarantine the allocation after a failed unlock.
	func() { result = errors.Join(result, o.unlock()) }()
	if pixels == nil {
		return errors.New("AHardwareBuffer_lock returned nil pixels")
	}
	src := unsafe.Slice((*byte)(pixels), int(uint64(o.desc.Stride)*uint64(o.desc.Height)*4))
	row := int(o.desc.Width) * 4
	stride := int(o.desc.Stride) * 4
	for y := 0; y < int(o.desc.Height); y++ {
		copy(dst[y*row:(y+1)*row], src[y*stride:y*stride+row])
	}
	runtime.KeepAlive(o)
	return nil
}

// allocateNativeRGBA allocates CPU-readable display buffers through Android.
//
// Example: return a batch whose native handles survive Binder transport.
func allocateNativeRGBA(width, height int32, count int) (_ []*Buffer, result error) {
	if width <= 0 || height <= 0 || count < 1 || count > 64 {
		return nil, errors.New("invalid buffer allocation dimensions or count")
	}
	lib, err := loadAHB()
	if err != nil {
		return nil, err
	}
	var buffers []*Buffer
	defer // Release the whole partial batch on allocation failure.
	//
	// Example: close earlier buffers if a later allocation fails.
	func() {
		if result != nil {
			result = errors.Join(result, CloseBuffers(buffers))
		}
	}()
	for range count {
		d := ahbDesc{Width: uint32(width), Height: uint32(height), Layers: 1, Format: 1, Usage: 0xb03}
		var ptr unsafe.Pointer
		dp, out := unsafe.Pointer(&d), unsafe.Pointer(&ptr)
		var status int32
		if err := lib.allocate.call(unsafe.Pointer(&status), unsafe.Pointer(&dp), unsafe.Pointer(&out)); err != nil {
			return nil, err
		}
		runtime.KeepAlive(&d)
		if status != 0 || ptr == nil {
			return nil, fmt.Errorf("AHardwareBuffer_allocate: %d", status)
		}
		o := &ahbOwner{lib: lib, ptr: ptr}
		b := &Buffer{native: o}
		buffers = append(buffers, b)
		dp = unsafe.Pointer(&o.desc)
		if err := lib.describe.call(nil, unsafe.Pointer(&ptr), unsafe.Pointer(&dp)); err != nil {
			return nil, err
		}
		d = o.desc
		if d.Width != uint32(width) || d.Height != uint32(height) || d.Layers != 1 || d.Format != 1 || d.Stride < d.Width || d.Stride > math.MaxInt32 || d.Usage&3 == 0 {
			return nil, fmt.Errorf("unexpected AHardwareBuffer description: %+v", d)
		}
		pair, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_SEQPACKET|unix.SOCK_CLOEXEC, 0)
		if err != nil {
			return nil, err
		}
		fd := int32(pair[0])
		err = lib.send.call(unsafe.Pointer(&status), unsafe.Pointer(&ptr), unsafe.Pointer(&fd))
		if err == nil && status != 0 {
			err = fmt.Errorf("AHardwareBuffer_sendHandleToUnixSocket: %d", status)
		}
		if err != nil {
			unix.Close(pair[0])
			unix.Close(pair[1])
			return nil, err
		}
		data := make([]byte, 65536)
		control := make([]byte, 65536)
		n, on, flags, _, recvErr := unix.Recvmsg(pair[1], data, control, unix.MSG_CMSG_CLOEXEC)
		unix.Close(pair[0])
		unix.Close(pair[1])
		msgs, parseErr := unix.ParseSocketControlMessage(control[:on])
		for _, m := range msgs {
			rights, e := unix.ParseUnixRights(&m)
			if e != nil {
				parseErr = errors.Join(parseErr, e)
				continue
			}
			for _, f := range rights {
				o.fds = append(o.fds, int32(f))
			}
		}
		if recvErr != nil || parseErr != nil || flags&(unix.MSG_TRUNC|unix.MSG_CTRUNC) != 0 {
			return nil, fmt.Errorf("receive AHardwareBuffer handle: %w", errors.Join(recvErr, parseErr, errors.New("truncated or invalid message")))
		}
		data = data[:n]
		ints, err := validateAHBMetadata(data, ahbMetadata{width: d.Width, height: d.Height, layers: d.Layers, format: d.Format, stride: d.Stride, usage: d.Usage}, len(o.fds))
		if err != nil {
			return nil, err
		}
		o.raw = append([]byte(nil), data...)
		b.Handle = common.NativeHandle{Fds: o.fds, Ints: ints}
		b.Width = d.Width
		b.Height = d.Height
		b.Stride = int32(d.Stride)
		b.Format = int32(d.Format)
		b.Usage = d.Usage
	}
	return buffers, nil
}
