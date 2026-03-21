//go:build linux

package kernelbinder

import (
	"context"
	"encoding/binary"
	"fmt"
	"runtime"
	"unsafe"

	"github.com/xaionaro-go/binder/binder"
	"github.com/xaionaro-go/binder/logger"
	aidlerrors "github.com/xaionaro-go/binder/errors"
	"github.com/xaionaro-go/binder/parcel"
	"golang.org/x/sys/unix"
)

// RegisterReceiver associates a TransactionReceiver with a unique cookie
// and starts the read loop if it has not been started yet.
// Returns the cookie that identifies this receiver in incoming BR_TRANSACTION events.
//
// The cookie is the heap address of a receiverEntry allocated for this
// registration. The kernel binder driver uses the flat_binder_object.binder
// and .cookie fields to create/lookup binder nodes and to dispatch incoming
// transactions. Both fields must be non-zero process-space addresses --
// synthetic counter values (1, 2, 3 ...) are rejected by the kernel.
// Storing the *receiverEntry in the receivers map keeps the object reachable,
// so its address remains valid until UnregisterReceiver is called.
func (d *Driver) RegisterReceiver(
	ctx context.Context,
	receiver binder.TransactionReceiver,
) uintptr {
	logger.Tracef(ctx, "RegisterReceiver")

	entry := &receiverEntry{receiver: receiver}
	cookie := uintptr(unsafe.Pointer(entry))

	d.receiversMu.Lock()
	d.receivers[cookie] = entry
	d.receiversMu.Unlock()

	// readLoopOnce is not reset on Close. A closed Driver must not be
	// reused — Open always returns a fresh instance. If RegisterReceiver
	// is called after Close, the read loop will not start, which is the
	// intended behavior (the fd is already closed).
	d.readLoopOnce.Do(func() {
		d.readLoopStarted.Store(true)
		// Use a plain goroutine because observability.Go is not available
		// in this module. TODO: switch to observability.Go when available.
		go d.runReadLoop(ctx)
	})

	logger.Tracef(ctx, "/RegisterReceiver: cookie=0x%x", cookie)
	return cookie
}

// UnregisterReceiver removes a TransactionReceiver by its cookie.
func (d *Driver) UnregisterReceiver(
	ctx context.Context,
	cookie uintptr,
) {
	logger.Tracef(ctx, "UnregisterReceiver")
	defer func() { logger.Tracef(ctx, "/UnregisterReceiver") }()

	d.receiversMu.Lock()
	delete(d.receivers, cookie)
	d.receiversMu.Unlock()
}

// lookupReceiver returns the TransactionReceiver for the given cookie,
// or nil if no receiver is registered.
func (d *Driver) lookupReceiver(
	cookie uintptr,
) binder.TransactionReceiver {
	d.receiversMu.RLock()
	defer d.receiversMu.RUnlock()

	entry := d.receivers[cookie]
	if entry == nil {
		return nil
	}
	return entry.receiver
}

// runReadLoop is the background goroutine that reads BR_* events
// from the binder driver and dispatches incoming transactions.
func (d *Driver) runReadLoop(
	ctx context.Context,
) {
	logger.Tracef(ctx, "runReadLoop")
	defer func() {
		logger.Tracef(ctx, "/runReadLoop")
		close(d.readLoopDone)
	}()

	// Pin to OS thread: binder routes replies by thread ID.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// Tell the kernel this thread accepts incoming transactions.
	enterBuf := make([]byte, 4)
	binary.LittleEndian.PutUint32(enterBuf[0:4], uint32(bcEnterLooper))

	if err := d.writeCommand(ctx, enterBuf); err != nil {
		logger.Errorf(ctx, "failed to send BC_ENTER_LOOPER: %v", err)
		return
	}

	readBuf := make([]byte, readBufferSize)
	for {
		select {
		case <-ctx.Done():
			d.sendExitLooper(ctx)
			return
		default:
		}

		bwr := binderWriteRead{
			readSize:   uint64(len(readBuf)),
			readBuffer: uint64(uintptr(unsafe.Pointer(&readBuf[0]))),
		}

		d.fdMu.RLock()
		fd := d.fd
		if fd < 0 {
			d.fdMu.RUnlock()
			logger.Tracef(ctx, "read loop: fd invalidated, exiting")
			return
		}
		var errno unix.Errno
		for {
			_, _, errno = unix.Syscall(
				unix.SYS_IOCTL,
				uintptr(fd),
				binderWriteReadIoctl,
				uintptr(unsafe.Pointer(&bwr)),
			)
			if errno == unix.EINTR {
				continue
			}
			break
		}
		d.fdMu.RUnlock()

		if errno != 0 {
			// If the fd was closed (e.g. during shutdown), stop gracefully.
			if errno == unix.EBADF {
				logger.Tracef(ctx, "read loop: fd closed, exiting")
				return
			}
			logger.Errorf(ctx, "read loop ioctl error: %v", errno)
			return
		}

		if bwr.readConsumed == 0 {
			continue
		}

		d.dispatchReadBuffer(ctx, readBuf[:bwr.readConsumed])
	}
}

// sendExitLooper notifies the kernel that this thread is leaving the looper.
func (d *Driver) sendExitLooper(
	ctx context.Context,
) {
	exitBuf := make([]byte, 4)
	binary.LittleEndian.PutUint32(exitBuf[0:4], uint32(bcExitLooper))

	if err := d.writeCommand(ctx, exitBuf); err != nil {
		logger.Warnf(ctx, "failed to send BC_EXIT_LOOPER: %v", err)
	}
}

// dispatchReadBuffer processes BR_* codes from the read buffer,
// handling incoming transactions and reference management commands.
func (d *Driver) dispatchReadBuffer(
	ctx context.Context,
	buf []byte,
) {
	offset := 0
	txnSize := int(unsafe.Sizeof(binderTransactionData{}))
	txnSecctxSize := int(binderTransactionDataSecctxSize)
	ptrCookieSize := int(binderPtrCookieSize)

	for offset < len(buf) {
		if offset+4 > len(buf) {
			logger.Warnf(ctx, "read loop: truncated BR code at offset %d", offset)
			return
		}

		cmd := binderReturn(binary.LittleEndian.Uint32(buf[offset:]))
		offset += 4

		switch cmd {
		case brNoop:
			logger.Tracef(ctx, "read loop: BR_NOOP")

		case brTransactionComplete:
			logger.Tracef(ctx, "read loop: BR_TRANSACTION_COMPLETE")

		case brTransaction:
			logger.Tracef(ctx, "read loop: BR_TRANSACTION")
			if offset+txnSize > len(buf) {
				logger.Warnf(ctx, "read loop: truncated BR_TRANSACTION at offset %d", offset)
				return
			}

			var txn binderTransactionData
			copyBytesToStruct(unsafe.Pointer(&txn), buf[offset:], unsafe.Sizeof(txn))
			offset += txnSize

			d.handleIncomingTransaction(ctx, &txn)

		case brTransactionSecCtx:
			// BR_TRANSACTION_SEC_CTX carries binder_transaction_data +
			// a binder_uintptr_t security context pointer. We read both
			// but ignore the secctx, treating it as a normal transaction.
			logger.Tracef(ctx, "read loop: BR_TRANSACTION_SEC_CTX")
			if offset+txnSecctxSize > len(buf) {
				logger.Warnf(ctx, "read loop: truncated BR_TRANSACTION_SEC_CTX at offset %d", offset)
				return
			}

			var txn binderTransactionData
			copyBytesToStruct(unsafe.Pointer(&txn), buf[offset:], unsafe.Sizeof(txn))
			offset += txnSecctxSize // consume both txn data and secctx pointer

			d.handleIncomingTransaction(ctx, &txn)

		case brReply:
			// The read loop should not normally receive BR_REPLY;
			// those are handled by the synchronous Transact path.
			logger.Warnf(ctx, "read loop: unexpected BR_REPLY")
			if offset+txnSize > len(buf) {
				logger.Warnf(ctx, "read loop: truncated BR_REPLY at offset %d", offset)
				return
			}

			// Parse the transaction data to extract the kernel buffer address.
			// The kernel allocates this buffer for every BR_REPLY; failing to
			// free it leaks kernel mmap'd memory until the fd is closed.
			var txn binderTransactionData
			copyBytesToStruct(unsafe.Pointer(&txn), buf[offset:], unsafe.Sizeof(txn))
			if err := d.freeBuffer(ctx, txn.dataBuffer); err != nil {
				logger.Warnf(ctx, "failed to free unexpected BR_REPLY buffer: %v", err)
			}
			offset += txnSize

		case brIncRefs:
			logger.Tracef(ctx, "read loop: BR_INCREFS")
			if offset+ptrCookieSize > len(buf) {
				logger.Warnf(ctx, "read loop: truncated BR_INCREFS at offset %d", offset)
				return
			}
			d.handleRefCommand(ctx, bcIncRefsDone, buf[offset:offset+ptrCookieSize])
			offset += ptrCookieSize

		case brAcquire:
			logger.Tracef(ctx, "read loop: BR_ACQUIRE")
			if offset+ptrCookieSize > len(buf) {
				logger.Warnf(ctx, "read loop: truncated BR_ACQUIRE at offset %d", offset)
				return
			}
			d.handleRefCommand(ctx, bcAcquireDone, buf[offset:offset+ptrCookieSize])
			offset += ptrCookieSize

		case brRelease:
			logger.Tracef(ctx, "read loop: BR_RELEASE")
			if offset+ptrCookieSize > len(buf) {
				logger.Warnf(ctx, "read loop: truncated BR_RELEASE at offset %d", offset)
				return
			}
			// No acknowledgment needed; reference counting is a no-op for now.
			offset += ptrCookieSize

		case brDecrefs:
			logger.Tracef(ctx, "read loop: BR_DECREFS")
			if offset+ptrCookieSize > len(buf) {
				logger.Warnf(ctx, "read loop: truncated BR_DECREFS at offset %d", offset)
				return
			}
			// No acknowledgment needed; reference counting is a no-op for now.
			offset += ptrCookieSize

		case brDeadBinder:
			logger.Tracef(ctx, "read loop: BR_DEAD_BINDER")
			cookieSize := int(unsafe.Sizeof(uintptr(0)))
			if offset+cookieSize > len(buf) {
				logger.Warnf(ctx, "read loop: truncated BR_DEAD_BINDER at offset %d", offset)
				return
			}
			cookie := readUintptr(buf[offset:])
			offset += cookieSize
			d.handleDeadBinder(ctx, cookie)

		case brClearDeathNotificationDone:
			logger.Tracef(ctx, "read loop: BR_CLEAR_DEATH_NOTIFICATION_DONE")
			cookieSize := int(unsafe.Sizeof(uintptr(0)))
			if offset+cookieSize > len(buf) {
				logger.Warnf(ctx, "read loop: truncated BR_CLEAR_DEATH_NOTIFICATION_DONE at offset %d", offset)
				return
			}
			// Read and discard the cookie; this is just an acknowledgment.
			offset += cookieSize

		case brDeadReply:
			logger.Tracef(ctx, "read loop: BR_DEAD_REPLY")

		case brFailedReply:
			logger.Tracef(ctx, "read loop: BR_FAILED_REPLY")

		case brError:
			if offset+4 > len(buf) {
				logger.Warnf(ctx, "read loop: truncated BR_ERROR at offset %d", offset)
				return
			}
			errCode := int32(binary.LittleEndian.Uint32(buf[offset:]))
			offset += 4
			logger.Errorf(ctx, "read loop: BR_ERROR %d", errCode)

		case brSpawnLooper:
			logger.Tracef(ctx, "read loop: BR_SPAWN_LOOPER (ignored)")

		default:
			logger.Warnf(ctx, "read loop: unknown BR code 0x%08x at offset %d", cmd, offset-4)
			return
		}
	}
}

// refAckBufSize is the size of the acknowledgment buffer for BR_INCREFS/BR_ACQUIRE:
// 4 bytes (command) + binderPtrCookieSize (16 on 64-bit) = 20.
const refAckBufSize = 4 + binderPtrCookieSize

// handleRefCommand acknowledges a BR_INCREFS or BR_ACQUIRE with the
// corresponding BC_INCREFS_DONE or BC_ACQUIRE_DONE response.
func (d *Driver) handleRefCommand(
	ctx context.Context,
	doneCmd binderCommand,
	ptrCookieBuf []byte,
) {
	// The payload is binder_ptr_cookie: two pointer-sized values (ptr + cookie).
	// We echo them back in BC_INCREFS_DONE / BC_ACQUIRE_DONE.
	// Use a fixed-size array so the compiler can stack-allocate it.
	var ackArr [refAckBufSize]byte
	ackBuf := ackArr[:]
	binary.LittleEndian.PutUint32(ackBuf[0:4], uint32(doneCmd))
	copy(ackBuf[4:], ptrCookieBuf)

	if err := d.writeCommand(ctx, ackBuf); err != nil {
		logger.Warnf(ctx, "failed to send ref acknowledgment 0x%08x: %v", doneCmd, err)
	}
}

// handleSystemTransaction handles system transaction codes (those above
// LastCallTransaction) such as INTERFACE_TRANSACTION and PING_TRANSACTION.
func (d *Driver) handleSystemTransaction(
	ctx context.Context,
	code binder.TransactionCode,
	receiver binder.TransactionReceiver,
	isOneway bool,
) {
	switch code {
	case binder.InterfaceTransaction:
		desc := receiver.Descriptor()
		logger.Tracef(ctx, "INTERFACE_TRANSACTION: returning descriptor %q", desc)
		reply := parcel.New()
		reply.WriteString16(desc)
		if !isOneway {
			d.sendReply(ctx, reply)
		}

	case binder.PingTransaction:
		if !isOneway {
			d.sendReply(ctx, parcel.New())
		}

	default:
		logger.Tracef(ctx, "ignoring system transaction code 0x%x", uint32(code))
		if !isOneway {
			d.sendReply(ctx, parcel.New())
		}
	}
}

// handleIncomingTransaction processes a BR_TRANSACTION by looking up the
// registered receiver, calling OnTransaction, sending BC_REPLY, and
// freeing the transaction buffer.
func (d *Driver) handleIncomingTransaction(
	ctx context.Context,
	txn *binderTransactionData,
) {
	cookie := uintptr(txn.cookie)
	code := binder.TransactionCode(txn.code)
	isOneway := binder.TransactionFlags(txn.flags)&binder.FlagOneway != 0

	// Copy transaction data from the mmap'd region.
	var dataBytes []byte
	if txn.dataSize > 0 {
		var copyErr error
		dataBytes, copyErr = d.copyFromMapped(txn.dataBuffer, txn.dataSize)
		if copyErr != nil {
			logger.Warnf(ctx, "failed to copy transaction data: %v", copyErr)
		}
	}

	// Free the mmap'd buffer as soon as we have copied the data.
	// The kernel requires BC_FREE_BUFFER for ALL BR_TRANSACTION buffers
	// regardless of dataSize.
	if err := d.freeBuffer(ctx, txn.dataBuffer); err != nil {
		logger.Warnf(ctx, "failed to free transaction buffer: %v", err)
	}

	receiver := d.lookupReceiver(cookie)
	if receiver == nil {
		logger.Warnf(ctx, "no receiver registered for cookie %d", cookie)
		if !isOneway {
			d.sendReply(ctx, parcel.New())
		}
		return
	}

	// System transaction codes (above LastCallTransaction) are handled
	// separately from user call codes to keep the switch value-based.
	if code > binder.LastCallTransaction {
		d.handleSystemTransaction(ctx, code, receiver, isOneway)
		return
	}

	dataParcel := parcel.FromBytes(dataBytes)
	replyParcel, err := receiver.OnTransaction(ctx, code, dataParcel)
	if err != nil {
		// Trace level: fires on every unhandled transaction code on the
		// per-frame hot path. Warn-level consumed 20ms in profiling.
		logger.Tracef(ctx, "OnTransaction(code=%d) failed: %v", code, err)
		if !isOneway {
			// Write the error as an AIDL status into the reply so the
			// caller receives a proper error instead of an empty parcel.
			errReply := parcel.New()
			binder.WriteStatus(errReply, err)
			d.sendReply(ctx, errReply)
		}
		return
	}

	// For one-way transactions the kernel does not expect a reply.
	if isOneway {
		return
	}

	if replyParcel == nil {
		replyParcel = parcel.New()
	}

	d.sendReply(ctx, replyParcel)
}

// sendReply sends a BC_REPLY with the given parcel data.
func (d *Driver) sendReply(
	ctx context.Context,
	reply *parcel.Parcel,
) {
	if err := d.Reply(ctx, reply); err != nil {
		logger.Warnf(ctx, "failed to send BC_REPLY: %v", err)
	}
}

// Reply sends BC_REPLY with the given parcel data back to the
// transaction originator.
func (d *Driver) Reply(
	ctx context.Context,
	reply *parcel.Parcel,
) (_err error) {
	replyData := reply.Data()
	objects := reply.Objects()

	var dataPtr, offsetsPtr uint64
	if len(replyData) > 0 {
		dataPtr = uint64(uintptr(unsafe.Pointer(&replyData[0])))
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
		code:          0,
		flags:         0,
		dataSize:      uint64(len(replyData)),
		offsetsSize:   uint64(len(objects) * 8),
		dataBuffer:    dataPtr,
		offsetsBuffer: offsetsPtr,
	}

	txnSize := unsafe.Sizeof(txn)
	// Use a fixed-size array so the compiler can stack-allocate it,
	// avoiding a per-call heap allocation for the write buffer.
	var writeBufArr [replyWriteBufSize]byte
	writeBuf := writeBufArr[:4+txnSize]
	binary.LittleEndian.PutUint32(writeBuf[0:4], uint32(bcReply))
	copyStructToBytes(writeBuf[4:], unsafe.Pointer(&txn), txnSize)

	// Use readSize=0 so the ioctl only sends the BC_REPLY without
	// consuming any incoming events from the read buffer. Events will
	// be picked up by the Transact loop or the read loop instead.
	// Previously readSize was set to readBufferSize, which caused
	// incoming BR_TRANSACTION events to be silently dropped, leading
	// to deadlocks when the remote end expected a reply.
	bwr := binderWriteRead{
		writeSize:   uint64(len(writeBuf)),
		writeBuffer: uint64(uintptr(unsafe.Pointer(&writeBuf[0]))),
		readSize:    0,
		readBuffer:  0,
	}

	for {
		d.fdMu.RLock()
		fd := d.fd
		if fd < 0 {
			d.fdMu.RUnlock()
			runtime.KeepAlive(replyData)
			runtime.KeepAlive(offsetsBuf)
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
		case unix.EINTR:
			// The kernel may have partially consumed the write buffer
			// before the signal arrived. Advance past consumed bytes to
			// avoid re-sending commands or sending an empty ioctl that
			// silently succeeds without delivering BC_REPLY.
			if bwr.writeConsumed >= bwr.writeSize {
				break // fully consumed, nothing left to send
			}
			bwr.writeBuffer += bwr.writeConsumed
			bwr.writeSize -= bwr.writeConsumed
			bwr.writeConsumed = 0
			continue
		default:
			return fmt.Errorf("Reply: %w", &aidlerrors.BinderError{
				Op:  "ioctl(BINDER_WRITE_READ)",
				Err: errno,
			})
		}
		break
	}
	runtime.KeepAlive(replyData)
	runtime.KeepAlive(offsetsBuf)

	return nil
}
