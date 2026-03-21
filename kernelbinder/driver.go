//go:build linux

package kernelbinder

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
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

// replyWriteBufSize is the size of the pre-allocated buffer for BC_REPLY:
// 4 bytes (command) + 64 bytes (binderTransactionData) = 68.
const replyWriteBufSize = 4 + 64

// freeBufferBufSize is the size of the pre-allocated buffer for BC_FREE_BUFFER:
// 4 bytes (command) + 8 bytes (pointer) = 12.
const freeBufferBufSize = 4 + 8

// ErrDriverClosed is returned when an operation is attempted on a closed Driver.
var ErrDriverClosed = errors.New("binder: driver is closed")

// Driver implements binder.Transport using /dev/binder.
type Driver struct {
	fd              int
	mapped          []byte // mmap'd region, kept alive for munmap
	mapSize         uint32
	closed          bool
	mu              sync.Mutex
	// fdMu protects d.fd and d.mapped from concurrent access during
	// Close. Read paths (doIoctl, writeCommand, Reply, copyFromMapped,
	// the read loop ioctl) hold RLock; Close holds Lock when
	// invalidating the fields, which blocks until all in-flight
	// syscalls complete.
	fdMu            sync.RWMutex
	acquiredHandles map[uint32]bool // tracks handles acquired via BC_INCREFS + BC_ACQUIRE

	// receivers maps cookie values (heap addresses of receiverEntry) to
	// the entries themselves. Using *receiverEntry as the map value keeps
	// each entry reachable, preventing the GC from collecting the object
	// whose address the kernel holds as a cookie.
	receivers   map[uintptr]*receiverEntry
	receiversMu sync.RWMutex

	// deathRecipients maps cookie values (heap addresses of
	// deathRecipientEntry) to the entries themselves, keeping them
	// reachable so the GC does not collect them while the kernel holds
	// the cookie. A second index by handle allows ClearDeathNotification
	// to find the entry without the caller retaining the cookie.
	deathRecipients       map[uintptr]*deathRecipientEntry
	deathRecipientsByHndl map[uint32]*deathRecipientEntry
	deathRecipientsMu     sync.Mutex

	// readLoopOnce ensures the read loop goroutine is started at most once.
	// It is not reset on Close because a closed Driver must not be reused;
	// Open always creates a new Driver instance.
	readLoopOnce    sync.Once
	readLoopDone    chan struct{} // closed when the read loop exits
	readLoopStarted atomic.Bool  // true after the read loop goroutine is launched
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
		fd:                    fd,
		mapped:                mapped,
		mapSize:               mapSize,
		acquiredHandles:       make(map[uint32]bool),
		receivers:             make(map[uintptr]*receiverEntry),
		deathRecipients:       make(map[uintptr]*deathRecipientEntry),
		deathRecipientsByHndl: make(map[uint32]*deathRecipientEntry),
		readLoopDone:          make(chan struct{}),
	}

	return d, nil
}

// checkClosed returns ErrDriverClosed if the driver has been closed.
// The caller must hold d.mu.
func (d *Driver) checkClosed() error {
	if d.closed {
		return ErrDriverClosed
	}
	return nil
}

// Close releases all death notifications, acquired binder handles, the mmap,
// and the file descriptor.
func (d *Driver) Close(
	ctx context.Context,
) (_err error) {
	logger.Tracef(ctx, "Close")
	defer func() { logger.Tracef(ctx, "/Close: %v", _err) }()

	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return ErrDriverClosed
	}
	d.closed = true
	handles := d.acquiredHandles
	d.acquiredHandles = nil
	d.mu.Unlock()

	var errs []error

	// Clear all registered death notifications BEFORE invalidating the
	// fd, because writeDeathCmd -> writeCommand checks d.fd and returns
	// ErrDriverClosed when it is -1.
	d.deathRecipientsMu.Lock()
	deathEntries := d.deathRecipientsByHndl
	d.deathRecipients = nil
	d.deathRecipientsByHndl = nil
	d.deathRecipientsMu.Unlock()

	for handle, entry := range deathEntries {
		cookie := uintptr(unsafe.Pointer(entry))
		logger.Debugf(ctx, "clearing death notification for handle %d on close", handle)
		if err := d.writeDeathCmd(ctx, bcClearDeathNotif, handle, cookie); err != nil {
			errs = append(errs, fmt.Errorf("clear death notification handle %d: %w", handle, err))
		}
	}

	// Release all acquired binder handles before closing the fd,
	// so the kernel does not leak handle references.
	for handle := range handles {
		logger.Debugf(ctx, "releasing handle %d on close", handle)
		buf := make([]byte, 16)
		binary.LittleEndian.PutUint32(buf[0:4], uint32(bcRelease))
		binary.LittleEndian.PutUint32(buf[4:8], handle)
		binary.LittleEndian.PutUint32(buf[8:12], uint32(bcDecRefs))
		binary.LittleEndian.PutUint32(buf[12:16], handle)
		if err := d.writeCommand(ctx, buf); err != nil {
			errs = append(errs, fmt.Errorf("release handle %d: %w", handle, err))
		}
	}

	// Take fdMu write lock to wait for all in-flight ioctls (which
	// hold RLock) to finish, then invalidate fd/mapped so no new
	// operations can use them. The actual munmap and close happen
	// outside both locks using local copies.
	d.fdMu.Lock()
	fd := d.fd
	mapped := d.mapped
	d.fd = -1
	d.mapped = nil
	d.fdMu.Unlock()

	if mapped != nil {
		if err := unix.Munmap(mapped); err != nil {
			errs = append(errs, &aidlerrors.BinderError{Op: "munmap", Err: err})
		}
	}

	if fd >= 0 {
		if err := unix.Close(fd); err != nil {
			errs = append(errs, &aidlerrors.BinderError{Op: "close", Err: err})
		}
	}

	// Wait for the read loop goroutine to exit after closing the fd.
	// The read loop detects the closed fd via EBADF and returns.
	// Only wait if the read loop was actually started; otherwise
	// readLoopDone is never closed and we would block forever.
	if d.readLoopStarted.Load() {
		select {
		case <-d.readLoopDone:
		case <-time.After(5 * time.Second):
			logger.Warnf(ctx, "timed out waiting for read loop to exit")
		}
	}

	return errors.Join(errs...)
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

	d.mu.Lock()
	err := d.checkClosed()
	d.mu.Unlock()
	if err != nil {
		return nil, err
	}

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
	binary.LittleEndian.PutUint32(writeBuf[0:4], uint32(bcTransaction))
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
	runtime.KeepAlive(dataBytes)
	runtime.KeepAlive(offsetsBuf)
	runtime.KeepAlive(writeBuf)
	runtime.KeepAlive(readBuf)

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
	txnSecctxSize := int(binderTransactionDataSecctxSize)
	ptrCookieSize := int(binderPtrCookieSize)

	for offset < len(buf) {
		if offset+4 > len(buf) {
			return nil, false, gotTransactionComplete, fmt.Errorf("binder: truncated BR code at offset %d", offset)
		}

		cmd := binderReturn(binary.LittleEndian.Uint32(buf[offset:]))
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

			logger.Tracef(ctx, "BR_REPLY: flags=0x%x dataSize=%d offsetsSize=%d", txn.flags, txn.dataSize, txn.offsetsSize)

			if txn.dataSize == 0 {
				// The kernel requires BC_FREE_BUFFER for ALL BR_REPLY
				// buffers regardless of dataSize.
				if freeErr := d.freeBuffer(ctx, txn.dataBuffer); freeErr != nil {
					logger.Warnf(ctx, "failed to free binder buffer: %v", freeErr)
				}
				return parcel.New(), true, gotTransactionComplete, nil
			}

			replyData, copyErr := d.copyFromMapped(txn.dataBuffer, txn.dataSize)
			if copyErr != nil {
				return nil, true, gotTransactionComplete, copyErr
			}

			if transactionFlag(txn.flags)&tfStatusCode != 0 {
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

		case brTransactionSecCtx:
			// BR_TRANSACTION_SEC_CTX carries a binder_transaction_data_secctx:
			// the normal binder_transaction_data followed by a binder_uintptr_t
			// security context pointer. We read both but ignore the secctx.
			logger.Tracef(ctx, "parseReadBuffer: BR_TRANSACTION_SEC_CTX (inline)")
			if offset+txnSecctxSize > len(buf) {
				return nil, false, gotTransactionComplete, fmt.Errorf("binder: truncated BR_TRANSACTION_SEC_CTX at offset %d", offset)
			}

			var txn binderTransactionData
			copyBytesToStruct(unsafe.Pointer(&txn), buf[offset:], unsafe.Sizeof(txn))
			offset += txnSecctxSize // consume both txn data and secctx pointer

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

		case brDeadBinder:
			logger.Tracef(ctx, "parseReadBuffer: BR_DEAD_BINDER")
			cookieSize := int(unsafe.Sizeof(uintptr(0)))
			if offset+cookieSize > len(buf) {
				return nil, false, gotTransactionComplete, fmt.Errorf("binder: truncated BR_DEAD_BINDER at offset %d", offset)
			}
			cookie := readUintptr(buf[offset:])
			offset += cookieSize
			d.handleDeadBinder(ctx, cookie)

		case brClearDeathNotificationDone:
			logger.Tracef(ctx, "parseReadBuffer: BR_CLEAR_DEATH_NOTIFICATION_DONE")
			cookieSize := int(unsafe.Sizeof(uintptr(0)))
			if offset+cookieSize > len(buf) {
				return nil, false, gotTransactionComplete, fmt.Errorf("binder: truncated BR_CLEAR_DEATH_NOTIFICATION_DONE at offset %d", offset)
			}
			// Read and discard the cookie; this is just an acknowledgment.
			offset += cookieSize

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
	offsetsBuf, err := d.copyFromMapped(offsetsAddr, offsetsSize)
	if err != nil {
		logger.Warnf(ctx, "failed to copy offsets from mapped region: %v", err)
		return
	}

	for i := 0; i < numOffsets; i++ {
		objOffset := binary.LittleEndian.Uint64(offsetsBuf[i*8:])
		d.acquireSingleReplyHandle(ctx, replyData, objOffset)
	}
}

// acquireSingleReplyHandle checks whether the flat_binder_object at objOffset
// in replyData is a BINDER_TYPE_HANDLE and, if so, sends BC_INCREFS + BC_ACQUIRE.
func (d *Driver) acquireSingleReplyHandle(
	ctx context.Context,
	replyData []byte,
	objOffset uint64,
) {
	// Each offset points to a flat_binder_object in the reply data.
	// flat_binder_object: uint32 type + uint32 flags + uint32 handle/binder + ...
	if objOffset+12 > uint64(len(replyData)) {
		return
	}

	objType := binderObjectType(binary.LittleEndian.Uint32(replyData[objOffset:]))
	if objType != binderTypeHandle {
		return
	}

	handle := binary.LittleEndian.Uint32(replyData[objOffset+8:])
	logger.Tracef(ctx, "acquiring handle %d from reply", handle)

	// Send BC_INCREFS + BC_ACQUIRE in a single write.
	buf := make([]byte, 16)
	binary.LittleEndian.PutUint32(buf[0:4], uint32(bcIncRefs))
	binary.LittleEndian.PutUint32(buf[4:8], handle)
	binary.LittleEndian.PutUint32(buf[8:12], uint32(bcAcquire))
	binary.LittleEndian.PutUint32(buf[12:16], handle)

	if err := d.writeCommand(ctx, buf); err != nil {
		logger.Warnf(ctx, "failed to acquire handle %d: %v", handle, err)
		return
	}

	d.mu.Lock()
	d.acquiredHandles[handle] = true
	d.mu.Unlock()
}

// copyFromMapped copies data from the mmap'd binder region by computing an offset
// relative to the mapped slice base, avoiding unsafe.Pointer(uintptr) conversions.
func (d *Driver) copyFromMapped(
	addr uint64,
	size uint64,
) ([]byte, error) {
	d.fdMu.RLock()
	mapped := d.mapped
	if mapped == nil {
		d.fdMu.RUnlock()
		return nil, ErrDriverClosed
	}
	base := uintptr(unsafe.Pointer(&mapped[0]))
	relOffset := uintptr(addr) - base
	if relOffset+uintptr(size) > uintptr(len(mapped)) {
		d.fdMu.RUnlock()
		return nil, fmt.Errorf("binder: mapped buffer access out of range: offset=%d size=%d mapped=%d", relOffset, size, len(mapped))
	}
	dst := make([]byte, size)
	copy(dst, mapped[relOffset:relOffset+uintptr(size)])
	d.fdMu.RUnlock()
	return dst, nil
}

// AcquireHandle increments the strong reference count for a binder handle.
func (d *Driver) AcquireHandle(
	ctx context.Context,
	handle uint32,
) (_err error) {
	logger.Tracef(ctx, "AcquireHandle")
	defer func() { logger.Tracef(ctx, "/AcquireHandle: %v", _err) }()

	d.mu.Lock()
	err := d.checkClosed()
	d.mu.Unlock()
	if err != nil {
		return err
	}

	// Send BC_INCREFS + BC_ACQUIRE in a single write, matching the
	// protocol used by acquireSingleReplyHandle.
	buf := make([]byte, 16)
	binary.LittleEndian.PutUint32(buf[0:4], uint32(bcIncRefs))
	binary.LittleEndian.PutUint32(buf[4:8], handle)
	binary.LittleEndian.PutUint32(buf[8:12], uint32(bcAcquire))
	binary.LittleEndian.PutUint32(buf[12:16], handle)

	if err := d.writeCommand(ctx, buf); err != nil {
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

	d.mu.Lock()
	err := d.checkClosed()
	d.mu.Unlock()
	if err != nil {
		return err
	}

	// Send BC_RELEASE + BC_DECREFS in a single write to balance the
	// BC_INCREFS + BC_ACQUIRE sent by AcquireHandle.
	buf := make([]byte, 16)
	binary.LittleEndian.PutUint32(buf[0:4], uint32(bcRelease))
	binary.LittleEndian.PutUint32(buf[4:8], handle)
	binary.LittleEndian.PutUint32(buf[8:12], uint32(bcDecRefs))
	binary.LittleEndian.PutUint32(buf[12:16], handle)

	if err := d.writeCommand(ctx, buf); err != nil {
		return err
	}
	d.mu.Lock()
	delete(d.acquiredHandles, handle)
	d.mu.Unlock()
	return nil
}

// RequestDeathNotification registers a death notification for a binder handle.
// Returns an error if the handle already has a registered death recipient;
// call ClearDeathNotification first to replace it.
func (d *Driver) RequestDeathNotification(
	ctx context.Context,
	handle uint32,
	recipient binder.DeathRecipient,
) (_err error) {
	logger.Tracef(ctx, "RequestDeathNotification")
	defer func() { logger.Tracef(ctx, "/RequestDeathNotification: %v", _err) }()

	d.mu.Lock()
	err := d.checkClosed()
	d.mu.Unlock()
	if err != nil {
		return err
	}

	// Heap-allocate an entry so its address (used as the binder cookie)
	// remains valid after this function returns. The previous code took
	// &recipient — a pointer to the stack-local interface header — which
	// became a dangling pointer once the frame was reclaimed.
	entry := &deathRecipientEntry{
		recipient: recipient,
		handle:    handle,
	}
	cookie := uintptr(unsafe.Pointer(entry))

	d.deathRecipientsMu.Lock()
	if existing := d.deathRecipientsByHndl[handle]; existing != nil {
		d.deathRecipientsMu.Unlock()
		return fmt.Errorf("binder: death notification already registered for handle %d; clear it first", handle)
	}
	d.deathRecipients[cookie] = entry
	d.deathRecipientsByHndl[handle] = entry
	d.deathRecipientsMu.Unlock()

	if err := d.writeDeathCmd(ctx, bcRequestDeathNotif, handle, cookie); err != nil {
		// Roll back on failure — the kernel does not hold this cookie.
		d.deathRecipientsMu.Lock()
		delete(d.deathRecipients, cookie)
		delete(d.deathRecipientsByHndl, handle)
		d.deathRecipientsMu.Unlock()
		return err
	}
	return nil
}

// ClearDeathNotification clears a death notification for a binder handle.
func (d *Driver) ClearDeathNotification(
	ctx context.Context,
	handle uint32,
	recipient binder.DeathRecipient,
) (_err error) {
	logger.Tracef(ctx, "ClearDeathNotification")
	defer func() { logger.Tracef(ctx, "/ClearDeathNotification: %v", _err) }()

	d.mu.Lock()
	err := d.checkClosed()
	d.mu.Unlock()
	if err != nil {
		return err
	}

	// Hold the lock across lookup, delete, and send to prevent a
	// concurrent RequestDeathNotification or ClearDeathNotification
	// from racing on the same handle (TOCTOU). The writeDeathCmd call
	// is a non-blocking write-only ioctl, so holding the lock is safe.
	d.deathRecipientsMu.Lock()
	entry := d.deathRecipientsByHndl[handle]
	if entry == nil {
		d.deathRecipientsMu.Unlock()
		return fmt.Errorf("binder: no death notification registered for handle %d", handle)
	}

	cookie := uintptr(unsafe.Pointer(entry))
	delete(d.deathRecipients, cookie)
	delete(d.deathRecipientsByHndl, handle)
	d.deathRecipientsMu.Unlock()

	if err := d.writeDeathCmd(ctx, bcClearDeathNotif, handle, cookie); err != nil {
		// Re-insert on failure so the state remains consistent with
		// what the kernel believes is registered.
		d.deathRecipientsMu.Lock()
		d.deathRecipients[cookie] = entry
		d.deathRecipientsByHndl[handle] = entry
		d.deathRecipientsMu.Unlock()
		return err
	}

	return nil
}

// writeDeathCmd writes a BC death notification command (handle + cookie).
// The cookie must be a heap-stable address (from a *deathRecipientEntry)
// so that it remains valid when the kernel echoes it back.
func (d *Driver) writeDeathCmd(
	ctx context.Context,
	cmd binderCommand,
	handle uint32,
	cookie uintptr,
) (_err error) {
	// BC_REQUEST_DEATH_NOTIFICATION / BC_CLEAR_DEATH_NOTIFICATION:
	// uint32 command + uint32 handle + uintptr cookie.
	// binderHandleCookieSize accounts for the packed kernel struct
	// (__u32 handle + binder_uintptr_t cookie).
	buf := make([]byte, 4+binderHandleCookieSize)
	binary.LittleEndian.PutUint32(buf[0:4], uint32(cmd))
	binary.LittleEndian.PutUint32(buf[4:8], handle)
	putUintptr(buf[8:], cookie)

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
		d.fdMu.RLock()
		fd := d.fd
		if fd < 0 {
			d.fdMu.RUnlock()
			runtime.KeepAlive(writeBuf)
			return ErrDriverClosed
		}
		_, _, errno := unix.Syscall(
			unix.SYS_IOCTL,
			uintptr(fd),
			binderWriteReadIoctl,
			uintptr(unsafe.Pointer(&bwr)),
		)
		d.fdMu.RUnlock()

		switch errno {
		case 0:
			runtime.KeepAlive(writeBuf)
			return nil
		case unix.EINTR:
			// The kernel may have partially consumed the write buffer
			// before the signal arrived. Advance past consumed bytes to
			// avoid re-sending already-processed commands.
			if bwr.writeConsumed >= bwr.writeSize {
				runtime.KeepAlive(writeBuf)
				return nil // fully consumed
			}
			bwr.writeBuffer += bwr.writeConsumed
			bwr.writeSize -= bwr.writeConsumed
			bwr.writeConsumed = 0
			continue
		default:
			runtime.KeepAlive(writeBuf)
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
//
// fdMu.RLock is held across each syscall to prevent Close from
// invalidating the fd while an ioctl is in progress. Close takes
// fdMu.Lock (write lock), which blocks until all in-flight ioctls
// release their RLock.
func (d *Driver) doIoctl(
	bwr *binderWriteRead,
) error {
	for {
		d.fdMu.RLock()
		fd := d.fd
		if fd < 0 {
			d.fdMu.RUnlock()
			return ErrDriverClosed
		}
		_, _, errno := unix.Syscall(
			unix.SYS_IOCTL,
			uintptr(fd),
			binderWriteReadIoctl,
			uintptr(unsafe.Pointer(bwr)),
		)
		d.fdMu.RUnlock()

		switch errno {
		case 0:
			return nil
		case unix.EINTR:
			// The kernel may have partially consumed the write buffer
			// before the signal arrived. Advance past consumed bytes
			// so we retry only the remainder; keep the read buffer
			// intact so the kernel can still deliver the response.
			if bwr.writeConsumed >= bwr.writeSize {
				bwr.writeSize = 0
			} else {
				bwr.writeBuffer += bwr.writeConsumed
				bwr.writeSize -= bwr.writeConsumed
			}
			bwr.writeConsumed = 0
			continue
		default:
			return &aidlerrors.BinderError{Op: "ioctl(BINDER_WRITE_READ)", Err: errno}
		}
	}
}

// handleDeadBinder processes a BR_DEAD_BINDER event by invoking the
// registered DeathRecipient's BinderDied callback, then acknowledging
// with BC_DEAD_BINDER_DONE. The entry is removed from both maps only
// after BC_DEAD_BINDER_DONE succeeds; if the write fails, the entry
// is kept so the operation can be retried.
func (d *Driver) handleDeadBinder(
	ctx context.Context,
	cookie uintptr,
) {
	d.deathRecipientsMu.Lock()
	entry := d.deathRecipients[cookie]
	d.deathRecipientsMu.Unlock()

	if entry != nil {
		// Run the callback in a goroutine to avoid blocking the read
		// loop thread. If BinderDied performs binder operations (e.g.
		// Transact, RequestDeathNotification) it would deadlock when
		// called synchronously on the read loop, because those
		// operations need the read loop to process kernel responses.
		go entry.recipient.BinderDied()
	} else {
		logger.Warnf(ctx, "BR_DEAD_BINDER: no recipient for cookie 0x%x", cookie)
	}

	// Acknowledge with BC_DEAD_BINDER_DONE immediately after
	// dispatching the callback, not after it completes, to avoid
	// blocking the read loop while the callback runs.
	buf := make([]byte, 4+8)
	binary.LittleEndian.PutUint32(buf[0:4], uint32(bcDeadBinderDone))
	putUintptr(buf[4:], cookie)

	if err := d.writeCommand(ctx, buf); err != nil {
		logger.Warnf(ctx, "failed to send BC_DEAD_BINDER_DONE: %v", err)
		return // keep the entry so it can be retried
	}

	// Remove the entry only after BC_DEAD_BINDER_DONE succeeds.
	if entry != nil {
		d.deathRecipientsMu.Lock()
		delete(d.deathRecipients, cookie)
		delete(d.deathRecipientsByHndl, entry.handle)
		d.deathRecipientsMu.Unlock()
	}
}

// putUintptr writes a uintptr as a little-endian 8-byte value (valid
// on 64-bit Linux, the only platform this package targets).
func putUintptr(
	b []byte,
	v uintptr,
) {
	binary.LittleEndian.PutUint64(b, uint64(v))
}

// readUintptr reads a little-endian 8-byte value as a uintptr.
func readUintptr(b []byte) uintptr {
	return uintptr(binary.LittleEndian.Uint64(b))
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
