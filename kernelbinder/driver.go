//go:build linux

package kernelbinder

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"unsafe"

	"github.com/facebookincubator/go-belt/tool/logger"
	"github.com/xaionaro-go/aidl/binder"
	aidlerrors "github.com/xaionaro-go/aidl/errors"
	"github.com/xaionaro-go/aidl/parcel"
	"golang.org/x/sys/unix"
)

// ioctl numbers for the binder driver.
var (
	binderVersionIoctl   = iowr('b', 9, unsafe.Sizeof(int32(0)))
	binderWriteReadIoctl = iowr('b', 1, unsafe.Sizeof(binderWriteRead{}))
	binderSetMaxThreads  = iow('b', 5, unsafe.Sizeof(uint32(0)))
)

// readBufferSize is the size of the read buffer for BINDER_WRITE_READ ioctl responses.
const readBufferSize = 256

// Driver implements binder.Transport using /dev/binder.
type Driver struct {
	fd              int
	mapped          []byte // mmap'd region, kept alive for munmap
	mapSize         uint32
	mu              sync.Mutex
	acquiredHandles map[uint32]bool // tracks handles acquired via BC_INCREFS + BC_ACQUIRE
}

// Compile-time interface check.
var _ binder.Transport = (*Driver)(nil)

// Open opens /dev/binder and initializes the driver.
func Open(
	ctx context.Context,
	opts ...binder.Option,
) (_driver *Driver, _err error) {
	logger.Tracef(ctx, "Open")
	defer func() { logger.Tracef(ctx, "/Open: %v", _err) }()

	cfg := binder.Options(opts).Config()

	fd, err := unix.Open("/dev/binder", unix.O_RDWR|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, &aidlerrors.BinderError{Op: "open", Err: err}
	}

	defer func() {
		if _err != nil {
			_ = unix.Close(fd)
		}
	}()

	// Verify protocol version.
	var version int32
	_, _, errno := unix.Syscall(
		unix.SYS_IOCTL,
		uintptr(fd),
		binderVersionIoctl,
		uintptr(unsafe.Pointer(&version)),
	)
	if errno != 0 {
		return nil, &aidlerrors.BinderError{Op: "ioctl(BINDER_VERSION)", Err: errno}
	}
	if version != binderCurrentProtocolVersion {
		return nil, fmt.Errorf(
			"binder: unsupported protocol version %d (expected %d)",
			version,
			binderCurrentProtocolVersion,
		)
	}

	// Set max threads.
	maxThreads := cfg.MaxThreads
	_, _, errno = unix.Syscall(
		unix.SYS_IOCTL,
		uintptr(fd),
		binderSetMaxThreads,
		uintptr(unsafe.Pointer(&maxThreads)),
	)
	if errno != 0 {
		return nil, &aidlerrors.BinderError{Op: "ioctl(BINDER_SET_MAX_THREADS)", Err: errno}
	}

	// mmap the binder buffer.
	mapSize := cfg.MapSize
	mapped, err := unix.Mmap(
		fd,
		0,
		int(mapSize),
		unix.PROT_READ,
		unix.MAP_PRIVATE|unix.MAP_NORESERVE,
	)
	if err != nil {
		return nil, &aidlerrors.BinderError{Op: "mmap", Err: err}
	}

	d := &Driver{
		fd:              fd,
		mapped:          mapped,
		mapSize:         mapSize,
		acquiredHandles: make(map[uint32]bool),
	}

	return d, nil
}

// Close releases all acquired binder handles, the mmap, and the file descriptor.
func (d *Driver) Close(
	ctx context.Context,
) (_err error) {
	logger.Tracef(ctx, "Close")
	defer func() { logger.Tracef(ctx, "/Close: %v", _err) }()

	var errs []error

	// Release all acquired binder handles before closing the fd,
	// so the kernel does not leak handle references.
	for handle := range d.acquiredHandles {
		logger.Debugf(ctx, "releasing handle %d on close", handle)
		buf := make([]byte, 16)
		binary.LittleEndian.PutUint32(buf[0:4], bcRelease)
		binary.LittleEndian.PutUint32(buf[4:8], handle)
		binary.LittleEndian.PutUint32(buf[8:12], bcDecRefs)
		binary.LittleEndian.PutUint32(buf[12:16], handle)
		if err := d.writeCommand(ctx, buf); err != nil {
			errs = append(errs, fmt.Errorf("release handle %d: %w", handle, err))
		}
	}
	d.acquiredHandles = nil

	if d.mapped != nil {
		if err := unix.Munmap(d.mapped); err != nil {
			errs = append(errs, &aidlerrors.BinderError{Op: "munmap", Err: err})
		}
		d.mapped = nil
	}

	if d.fd >= 0 {
		if err := unix.Close(d.fd); err != nil {
			errs = append(errs, &aidlerrors.BinderError{Op: "close", Err: err})
		}
		d.fd = -1
	}

	return errors.Join(errs...)
}

// mapBase returns the base address of the mmap'd region.
func (d *Driver) mapBase() uintptr {
	return uintptr(unsafe.Pointer(&d.mapped[0]))
}

// Transact performs a synchronous binder transaction.
func (d *Driver) Transact(
	ctx context.Context,
	handle uint32,
	code binder.TransactionCode,
	flags binder.TransactionFlags,
	data *parcel.Parcel,
) (_reply *parcel.Parcel, _err error) {
	logger.Tracef(ctx, "Transact")
	defer func() { logger.Tracef(ctx, "/Transact: %v", _err) }()

	// Binder kernel routes replies by thread ID, so we must pin this goroutine.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	d.mu.Lock()
	defer d.mu.Unlock()

	dataBytes := data.Data()
	objects := data.Objects()

	var dataPtr, offsetsPtr uint64
	if len(dataBytes) > 0 {
		dataPtr = uint64(uintptr(unsafe.Pointer(&dataBytes[0])))
	}

	var offsetsBuf []byte
	if len(objects) > 0 {
		offsetsBuf = make([]byte, len(objects)*8)
		for i, off := range objects {
			binary.LittleEndian.PutUint64(offsetsBuf[i*8:], off)
		}
		offsetsPtr = uint64(uintptr(unsafe.Pointer(&offsetsBuf[0])))
	}

	txn := binderTransactionData{
		target:        uint64(handle),
		code:          uint32(code),
		flags:         uint32(flags),
		dataSize:      uint64(len(dataBytes)),
		offsetsSize:   uint64(len(objects) * 8),
		dataBuffer:    dataPtr,
		offsetsBuffer: offsetsPtr,
	}

	// Build write buffer: uint32 command code + binderTransactionData.
	txnSize := unsafe.Sizeof(txn)
	writeBuf := make([]byte, 4+txnSize)
	binary.LittleEndian.PutUint32(writeBuf[0:4], bcTransaction)
	copyStructToBytes(writeBuf[4:], unsafe.Pointer(&txn), txnSize)

	// Allocate read buffer for the response.
	readBuf := make([]byte, readBufferSize)

	bwr := binderWriteRead{
		writeSize:   uint64(len(writeBuf)),
		writeBuffer: uint64(uintptr(unsafe.Pointer(&writeBuf[0]))),
		readSize:    uint64(len(readBuf)),
		readBuffer:  uint64(uintptr(unsafe.Pointer(&readBuf[0]))),
	}

	_, _, errno := unix.Syscall(
		unix.SYS_IOCTL,
		uintptr(d.fd),
		binderWriteReadIoctl,
		uintptr(unsafe.Pointer(&bwr)),
	)
	if errno != 0 {
		return nil, &aidlerrors.BinderError{Op: "ioctl(BINDER_WRITE_READ)", Err: errno}
	}

	// Parse the read buffer for BR codes.
	reply, err := d.parseReadBuffer(ctx, readBuf[:bwr.readConsumed])
	if err != nil {
		return nil, err
	}

	return reply, nil
}

// parseReadBuffer processes BR_* codes from the read buffer and returns the reply parcel.
func (d *Driver) parseReadBuffer(
	ctx context.Context,
	buf []byte,
) (_reply *parcel.Parcel, _err error) {
	logger.Tracef(ctx, "parseReadBuffer")
	defer func() { logger.Tracef(ctx, "/parseReadBuffer: %v", _err) }()

	offset := 0
	gotTransactionComplete := false

	for offset < len(buf) {
		if offset+4 > len(buf) {
			return nil, fmt.Errorf("binder: truncated BR code at offset %d", offset)
		}

		cmd := binary.LittleEndian.Uint32(buf[offset:])
		offset += 4

		switch cmd {
		case brNoop:
			logger.Debugf(ctx, "BR_NOOP")
			continue

		case brTransactionComplete:
			logger.Debugf(ctx, "BR_TRANSACTION_COMPLETE")
			gotTransactionComplete = true
			continue

		case brReply:
			logger.Debugf(ctx, "BR_REPLY")
			txnSize := int(unsafe.Sizeof(binderTransactionData{}))
			if offset+txnSize > len(buf) {
				return nil, fmt.Errorf("binder: truncated BR_REPLY data at offset %d", offset)
			}

			var txn binderTransactionData
			copyBytesToStruct(unsafe.Pointer(&txn), buf[offset:], unsafe.Sizeof(txn))

			if txn.dataSize == 0 {
				return parcel.New(), nil
			}

			// Copy data from the mmap'd region into a new parcel.
			replyData := d.copyFromMapped(txn.dataBuffer, txn.dataSize)

			// Acquire references for any binder handles in the reply
			// BEFORE freeing the buffer, otherwise the kernel drops
			// the handle references.
			d.acquireReplyHandles(ctx, replyData, txn.offsetsBuffer, txn.offsetsSize)

			// Free the mmap'd buffer.
			if err := d.freeBuffer(ctx, txn.dataBuffer); err != nil {
				logger.Warnf(ctx, "failed to free binder buffer: %v", err)
			}

			return parcel.FromBytes(replyData), nil

		case brDeadReply:
			logger.Debugf(ctx, "BR_DEAD_REPLY")
			return nil, &aidlerrors.TransactionError{
				Code: aidlerrors.TransactionErrorDeadObject,
			}

		case brFailedReply:
			logger.Debugf(ctx, "BR_FAILED_REPLY")
			return nil, &aidlerrors.TransactionError{
				Code: aidlerrors.TransactionErrorFailedTransaction,
			}

		case brError:
			if offset+4 > len(buf) {
				return nil, fmt.Errorf("binder: truncated BR_ERROR data")
			}
			errCode := int32(binary.LittleEndian.Uint32(buf[offset:]))
			return nil, fmt.Errorf("binder: BR_ERROR %d", errCode)

		case brSpawnLooper:
			logger.Debugf(ctx, "BR_SPAWN_LOOPER (ignored)")
			continue

		default:
			logger.Warnf(ctx, "binder: unknown BR code 0x%08x at offset %d", cmd, offset-4)
			return nil, fmt.Errorf("binder: unknown BR code 0x%08x", cmd)
		}
	}

	if !gotTransactionComplete {
		return nil, fmt.Errorf("binder: did not receive BR_TRANSACTION_COMPLETE")
	}

	// BR_TRANSACTION_COMPLETE without BR_REPLY means a one-way transaction.
	return parcel.New(), nil
}

// acquireReplyHandles scans the reply's flat_binder_objects (located via the
// offsets array) and sends BC_INCREFS + BC_ACQUIRE for each BINDER_TYPE_HANDLE.
// This must be called BEFORE BC_FREE_BUFFER, because the kernel drops handle
// references when the transaction buffer is freed.
func (d *Driver) acquireReplyHandles(
	ctx context.Context,
	replyData []byte,
	offsetsAddr uint64,
	offsetsSize uint64,
) {
	if offsetsSize == 0 {
		return
	}

	numOffsets := int(offsetsSize / 8)
	offsetsBuf := d.copyFromMapped(offsetsAddr, offsetsSize)

	for i := 0; i < numOffsets; i++ {
		objOffset := binary.LittleEndian.Uint64(offsetsBuf[i*8:])

		// Each offset points to a flat_binder_object in the reply data.
		// flat_binder_object: uint32 type + uint32 flags + uint32 handle/binder + ...
		if objOffset+12 > uint64(len(replyData)) {
			continue
		}

		objType := binary.LittleEndian.Uint32(replyData[objOffset:])
		if objType == binderTypeHandle {
			handle := binary.LittleEndian.Uint32(replyData[objOffset+8:])
			logger.Debugf(ctx, "acquiring handle %d from reply", handle)

			// Send BC_INCREFS + BC_ACQUIRE in a single write.
			buf := make([]byte, 16)
			binary.LittleEndian.PutUint32(buf[0:4], bcIncRefs)
			binary.LittleEndian.PutUint32(buf[4:8], handle)
			binary.LittleEndian.PutUint32(buf[8:12], bcAcquire)
			binary.LittleEndian.PutUint32(buf[12:16], handle)

			if err := d.writeCommand(ctx, buf); err != nil {
				logger.Warnf(ctx, "failed to acquire handle %d: %v", handle, err)
				continue
			}
			d.acquiredHandles[handle] = true
		}
	}
}

// copyFromMapped copies data from the mmap'd binder region by computing an offset
// relative to the mapped slice base, avoiding unsafe.Pointer(uintptr) conversions.
func (d *Driver) copyFromMapped(
	addr uint64,
	size uint64,
) []byte {
	base := d.mapBase()
	relOffset := uintptr(addr) - base
	dst := make([]byte, size)
	copy(dst, d.mapped[relOffset:relOffset+uintptr(size)])
	return dst
}

// AcquireHandle increments the strong reference count for a binder handle.
func (d *Driver) AcquireHandle(
	ctx context.Context,
	handle uint32,
) (_err error) {
	logger.Tracef(ctx, "AcquireHandle")
	defer func() { logger.Tracef(ctx, "/AcquireHandle: %v", _err) }()

	if err := d.writeHandleCommand(ctx, bcAcquire, handle); err != nil {
		return err
	}
	d.acquiredHandles[handle] = true
	return nil
}

// ReleaseHandle decrements the strong reference count for a binder handle.
func (d *Driver) ReleaseHandle(
	ctx context.Context,
	handle uint32,
) (_err error) {
	logger.Tracef(ctx, "ReleaseHandle")
	defer func() { logger.Tracef(ctx, "/ReleaseHandle: %v", _err) }()

	if err := d.writeHandleCommand(ctx, bcRelease, handle); err != nil {
		return err
	}
	delete(d.acquiredHandles, handle)
	return nil
}

// RequestDeathNotification registers a death notification for a binder handle.
func (d *Driver) RequestDeathNotification(
	ctx context.Context,
	handle uint32,
	recipient binder.DeathRecipient,
) (_err error) {
	logger.Tracef(ctx, "RequestDeathNotification")
	defer func() { logger.Tracef(ctx, "/RequestDeathNotification: %v", _err) }()

	return d.writeDeathCommand(ctx, bcRequestDeathNotif, handle, recipient)
}

// ClearDeathNotification clears a death notification for a binder handle.
func (d *Driver) ClearDeathNotification(
	ctx context.Context,
	handle uint32,
	recipient binder.DeathRecipient,
) (_err error) {
	logger.Tracef(ctx, "ClearDeathNotification")
	defer func() { logger.Tracef(ctx, "/ClearDeathNotification: %v", _err) }()

	return d.writeDeathCommand(ctx, bcClearDeathNotif, handle, recipient)
}

// writeHandleCommand writes a BC command that takes a uint32 handle argument.
func (d *Driver) writeHandleCommand(
	ctx context.Context,
	cmd uint32,
	handle uint32,
) (_err error) {
	buf := make([]byte, 4+4)
	binary.LittleEndian.PutUint32(buf[0:4], cmd)
	binary.LittleEndian.PutUint32(buf[4:8], handle)

	return d.writeCommand(ctx, buf)
}

// writeDeathCommand writes a BC death notification command (handle + cookie).
func (d *Driver) writeDeathCommand(
	ctx context.Context,
	cmd uint32,
	handle uint32,
	recipient binder.DeathRecipient,
) (_err error) {
	// BC_REQUEST_DEATH_NOTIFICATION / BC_CLEAR_DEATH_NOTIFICATION:
	// uint32 command + uint32 handle + uint64 cookie (pointer to recipient interface)
	buf := make([]byte, 4+4+8)
	binary.LittleEndian.PutUint32(buf[0:4], cmd)
	binary.LittleEndian.PutUint32(buf[4:8], handle)

	// Store the recipient interface pointer as the cookie so we can recover it later.
	cookie := uint64(uintptr(unsafe.Pointer(&recipient)))
	binary.LittleEndian.PutUint64(buf[8:16], cookie)

	return d.writeCommand(ctx, buf)
}

// writeCommand issues a write-only BINDER_WRITE_READ ioctl.
func (d *Driver) writeCommand(
	ctx context.Context,
	writeBuf []byte,
) (_err error) {
	logger.Tracef(ctx, "writeCommand")
	defer func() { logger.Tracef(ctx, "/writeCommand: %v", _err) }()

	bwr := binderWriteRead{
		writeSize:   uint64(len(writeBuf)),
		writeBuffer: uint64(uintptr(unsafe.Pointer(&writeBuf[0]))),
	}

	_, _, errno := unix.Syscall(
		unix.SYS_IOCTL,
		uintptr(d.fd),
		binderWriteReadIoctl,
		uintptr(unsafe.Pointer(&bwr)),
	)
	if errno != 0 {
		return &aidlerrors.BinderError{Op: "ioctl(BINDER_WRITE_READ)", Err: errno}
	}

	return nil
}

// copyStructToBytes copies a struct's raw memory into a byte slice.
func copyStructToBytes(
	dst []byte,
	src unsafe.Pointer,
	size uintptr,
) {
	srcSlice := unsafe.Slice((*byte)(src), size)
	copy(dst, srcSlice)
}

// copyBytesToStruct copies raw bytes into a struct's memory.
func copyBytesToStruct(
	dst unsafe.Pointer,
	src []byte,
	size uintptr,
) {
	dstSlice := unsafe.Slice((*byte)(dst), size)
	copy(dstSlice, src)
}
