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

	"github.com/xaionaro-go/binder/binder"
	"github.com/xaionaro-go/binder/logger"
	aidlerrors "github.com/xaionaro-go/binder/errors"
	"github.com/xaionaro-go/binder/parcel"
	"golang.org/x/sys/unix"
)

// ioctl numbers for the binder driver.
var (
	binderVersionIoctl   = iowr('b', 9, unsafe.Sizeof(int32(0)))
	binderWriteReadIoctl = iowr('b', 1, unsafe.Sizeof(binderWriteRead{}))
	binderSetMaxThreads  = iow('b', 5, unsafe.Sizeof(uint32(0)))
)

// readBufferSize is the size of the read buffer for BINDER_WRITE_READ ioctl responses.
// Must be large enough to hold multiple BR_* responses in a single ioctl read
// (e.g., BR_TRANSACTION_COMPLETE + BR_INCREFS + BR_ACQUIRE + BR_REPLY).
const readBufferSize = 1024

// receiverEntry holds a registered TransactionReceiver together with the
// heap-allocated anchor whose address is used as the binder cookie.
// Storing the entry as a map value (*receiverEntry) keeps it reachable by the
// GC, so its address remains valid for the kernel binder driver.
type receiverEntry struct {
	receiver binder.TransactionReceiver
}

// replyWriteBufSize is the size of the pre-allocated buffer for BC_REPLY:
// 4 bytes (command) + 64 bytes (binderTransactionData) = 68.
const replyWriteBufSize = 4 + 64

// freeBufferBufSize is the size of the pre-allocated buffer for BC_FREE_BUFFER:
// 4 bytes (command) + 8 bytes (pointer) = 12.
const freeBufferBufSize = 4 + 8

// Driver implements binder.Transport using /dev/binder.
type Driver struct {
	fd              int
	mapped          []byte // mmap'd region, kept alive for munmap
	mapSize         uint32
	mu              sync.Mutex
	acquiredHandles map[uint32]bool // tracks handles acquired via BC_INCREFS + BC_ACQUIRE

	// receivers maps cookie values (heap addresses of receiverEntry) to
	// the entries themselves. Using *receiverEntry as the map value keeps
	// each entry reachable, preventing the GC from collecting the object
	// whose address the kernel holds as a cookie.
	receivers   map[uintptr]*receiverEntry
	receiversMu sync.RWMutex

	readLoopOnce sync.Once
	readLoopDone chan struct{} // closed when the read loop exits
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
		receivers:       make(map[uintptr]*receiverEntry),
		readLoopDone:    make(chan struct{}),
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
	d.mu.Lock()
	handles := d.acquiredHandles
	d.acquiredHandles = nil
	d.mu.Unlock()

	for handle := range handles {
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

	// Lock only around each individual ioctl call, not across the entire
	// transaction. Holding the mutex during a blocking read-wait would
	// prevent the read loop from acknowledging BR_INCREFS/BR_ACQUIRE
	// callbacks, causing deadlock when the kernel expects acknowledgment
	// before sending BR_REPLY.
	if err := d.doIoctl(&bwr); err != nil {
		return nil, err
	}

	// Parse the read buffer for BR codes. The kernel may split
	// BR_TRANSACTION_COMPLETE and BR_REPLY across separate ioctl reads.
	// Additionally, when the remote service makes a callback into our
	// process during the transaction (e.g. INTERFACE_TRANSACTION), the
	// kernel delivers BR_TRANSACTION to this thread. After we handle
	// that inline and send BC_REPLY, the kernel may send more events
	// before our original BR_REPLY arrives. We track
	// BR_TRANSACTION_COMPLETE across reads because it might appear in
	// an earlier buffer than BR_REPLY.
	isOneway := flags&binder.FlagOneway != 0
	gotTC := false
	for {
		reply, gotReply, tc, err := d.parseReadBuffer(ctx, readBuf[:bwr.readConsumed])
		if err != nil {
			return nil, err
		}
		if tc {
			gotTC = true
		}

		// If we received BR_REPLY (even with empty data), or this is
		// a oneway transaction (only needs TC), return the result.
		switch {
		case gotReply:
			return reply, nil
		case isOneway && gotTC:
			return reply, nil
		}

		// We haven't received BR_REPLY yet. Issue a read-only ioctl to
		// wait for more events from the kernel.
		logger.Tracef(ctx, "Transact: waiting for BR_REPLY, reading again (gotTC=%v)", gotTC)
		bwr.writeSize = 0
		bwr.writeConsumed = 0
		bwr.readConsumed = 0
		if err := d.doIoctl(&bwr); err != nil {
			return nil, err
		}
	}
}

// parseReadBuffer processes BR_* codes from the read buffer.
// Returns:
//   - reply: the reply parcel (valid only when gotReply is true)
//   - gotReply: whether BR_REPLY was seen
//   - gotTC: whether BR_TRANSACTION_COMPLETE was seen
//   - err: any parse error
//
// The kernel may deliver BR_INCREFS, BR_ACQUIRE, BR_RELEASE, BR_DECREFS,
// and even BR_TRANSACTION to the transacting thread before BR_REPLY.
// All of these are handled inline to prevent deadlock.
func (d *Driver) parseReadBuffer(
	ctx context.Context,
	buf []byte,
) (_reply *parcel.Parcel, _gotReply bool, _gotTC bool, _err error) {
	logger.Tracef(ctx, "parseReadBuffer")
	defer func() { logger.Tracef(ctx, "/parseReadBuffer: %v", _err) }()

	offset := 0
	gotTransactionComplete := false
	txnSize := int(unsafe.Sizeof(binderTransactionData{}))
	ptrCookieSize := int(binderPtrCookieSize)

	for offset < len(buf) {
		if offset+4 > len(buf) {
			return nil, false, gotTransactionComplete, fmt.Errorf("binder: truncated BR code at offset %d", offset)
		}

		cmd := binary.LittleEndian.Uint32(buf[offset:])
		offset += 4

		switch cmd {
		case brNoop:
			logger.Tracef(ctx, "BR_NOOP")

		case brTransactionComplete:
			logger.Tracef(ctx, "BR_TRANSACTION_COMPLETE")
			gotTransactionComplete = true

		case brReply:
			logger.Tracef(ctx, "BR_REPLY")
			if offset+txnSize > len(buf) {
				return nil, true, gotTransactionComplete, fmt.Errorf("binder: truncated BR_REPLY data at offset %d", offset)
			}

			var txn binderTransactionData
			copyBytesToStruct(unsafe.Pointer(&txn), buf[offset:], unsafe.Sizeof(txn))
			offset += txnSize

			logger.Tracef(ctx, "BR_REPLY: flags=0x%x dataSize=%d offsetsSize=%d", txn.flags, txn.dataSize, txn.offsetsSize)

			if txn.dataSize == 0 {
				return parcel.New(), true, gotTransactionComplete, nil
			}

			replyData := d.copyFromMapped(txn.dataBuffer, txn.dataSize)

			if txn.flags&tfStatusCode != 0 {
				if freeErr := d.freeBuffer(ctx, txn.dataBuffer); freeErr != nil {
					logger.Warnf(ctx, "failed to free binder buffer: %v", freeErr)
				}
				if len(replyData) >= 4 {
					statusCode := int32(binary.LittleEndian.Uint32(replyData))
					if statusCode != 0 {
						return nil, true, gotTransactionComplete, fmt.Errorf("binder: kernel status error: %d (0x%x)", statusCode, uint32(statusCode))
					}
				}
				return parcel.New(), true, gotTransactionComplete, nil
			}

			d.acquireReplyHandles(ctx, replyData, txn.offsetsBuffer, txn.offsetsSize)

			if err := d.freeBuffer(ctx, txn.dataBuffer); err != nil {
				logger.Warnf(ctx, "failed to free binder buffer: %v", err)
			}

			return parcel.FromBytes(replyData), true, gotTransactionComplete, nil

		case brTransaction:
			logger.Tracef(ctx, "parseReadBuffer: BR_TRANSACTION (inline)")
			if offset+txnSize > len(buf) {
				return nil, false, gotTransactionComplete, fmt.Errorf("binder: truncated BR_TRANSACTION at offset %d", offset)
			}

			var txn binderTransactionData
			copyBytesToStruct(unsafe.Pointer(&txn), buf[offset:], unsafe.Sizeof(txn))
			offset += txnSize

			// Handle the incoming transaction inline (same as the read loop).
			d.handleIncomingTransaction(ctx, &txn)

		case brIncRefs:
			logger.Tracef(ctx, "parseReadBuffer: BR_INCREFS")
			if offset+ptrCookieSize > len(buf) {
				return nil, false, gotTransactionComplete, fmt.Errorf("binder: truncated BR_INCREFS at offset %d", offset)
			}
			d.handleRefCommand(ctx, bcIncRefsDone, buf[offset:offset+ptrCookieSize])
			offset += ptrCookieSize

		case brAcquire:
			logger.Tracef(ctx, "parseReadBuffer: BR_ACQUIRE")
			if offset+ptrCookieSize > len(buf) {
				return nil, false, gotTransactionComplete, fmt.Errorf("binder: truncated BR_ACQUIRE at offset %d", offset)
			}
			d.handleRefCommand(ctx, bcAcquireDone, buf[offset:offset+ptrCookieSize])
			offset += ptrCookieSize

		case brRelease:
			logger.Tracef(ctx, "parseReadBuffer: BR_RELEASE")
			if offset+ptrCookieSize > len(buf) {
				return nil, false, gotTransactionComplete, fmt.Errorf("binder: truncated BR_RELEASE at offset %d", offset)
			}
			offset += ptrCookieSize

		case brDecrefs:
			logger.Tracef(ctx, "parseReadBuffer: BR_DECREFS")
			if offset+ptrCookieSize > len(buf) {
				return nil, false, gotTransactionComplete, fmt.Errorf("binder: truncated BR_DECREFS at offset %d", offset)
			}
			offset += ptrCookieSize

		case brDeadReply:
			logger.Tracef(ctx, "BR_DEAD_REPLY")
			return nil, true, gotTransactionComplete, &aidlerrors.TransactionError{
				Code: aidlerrors.TransactionErrorDeadObject,
			}

		case brFailedReply:
			logger.Tracef(ctx, "BR_FAILED_REPLY")
			return nil, true, gotTransactionComplete, &aidlerrors.TransactionError{
				Code: aidlerrors.TransactionErrorFailedTransaction,
			}

		case brError:
			if offset+4 > len(buf) {
				return nil, false, gotTransactionComplete, fmt.Errorf("binder: truncated BR_ERROR data")
			}
			errCode := int32(binary.LittleEndian.Uint32(buf[offset:]))
			offset += 4
			return nil, false, gotTransactionComplete, fmt.Errorf("binder: BR_ERROR %d", errCode)

		case brSpawnLooper:
			logger.Tracef(ctx, "BR_SPAWN_LOOPER (ignored)")

		default:
			logger.Warnf(ctx, "binder: unknown BR code 0x%08x at offset %d", cmd, offset-4)
			return nil, false, gotTransactionComplete, fmt.Errorf("binder: unknown BR code 0x%08x", cmd)
		}
	}

	// Return what we found. The caller (Transact) tracks gotTC across
	// reads and decides when to issue another ioctl.
	return parcel.New(), false, gotTransactionComplete, nil
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
			logger.Tracef(ctx, "acquiring handle %d from reply", handle)

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
			d.mu.Lock()
			d.acquiredHandles[handle] = true
			d.mu.Unlock()
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
	d.mu.Lock()
	d.acquiredHandles[handle] = true
	d.mu.Unlock()
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
	d.mu.Lock()
	delete(d.acquiredHandles, handle)
	d.mu.Unlock()
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
	_ context.Context,
	writeBuf []byte,
) error {
	bwr := binderWriteRead{
		writeSize:   uint64(len(writeBuf)),
		writeBuffer: uint64(uintptr(unsafe.Pointer(&writeBuf[0]))),
	}

	for {
		_, _, errno := unix.Syscall(
			unix.SYS_IOCTL,
			uintptr(d.fd),
			binderWriteReadIoctl,
			uintptr(unsafe.Pointer(&bwr)),
		)
		switch errno {
		case 0:
			return nil
		case unix.EINTR:
			// Retry on interrupted system call — the command was not
			// processed and must be resent.
			continue
		default:
			return &aidlerrors.BinderError{Op: "ioctl(BINDER_WRITE_READ)", Err: errno}
		}
	}
}

// doIoctl executes a BINDER_WRITE_READ ioctl, retrying on EINTR.
// The binder fd supports concurrent ioctl calls from different OS threads
// (each thread has its own transaction state in the kernel), so no
// process-wide mutex is held around the syscall. Holding a mutex across a
// blocking ioctl would deadlock when the kernel needs the read-loop thread
// to acknowledge BR_INCREFS/BR_ACQUIRE before delivering BR_REPLY to the
// transacting thread.
func (d *Driver) doIoctl(
	bwr *binderWriteRead,
) error {
	for {
		_, _, errno := unix.Syscall(
			unix.SYS_IOCTL,
			uintptr(d.fd),
			binderWriteReadIoctl,
			uintptr(unsafe.Pointer(bwr)),
		)
		switch errno {
		case 0:
			return nil
		case unix.EINTR:
			// Retry on interrupted system call.
			// Reset write buffer (already consumed) but keep read buffer.
			bwr.writeSize = 0
			continue
		default:
			return &aidlerrors.BinderError{Op: "ioctl(BINDER_WRITE_READ)", Err: errno}
		}
	}
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
