// Package igbp defines shared constants for IGraphicBufferProducer
// interactions used by multiple camera-related commands.
package igbp

import "github.com/xaionaro-go/binder/binder"

// Descriptor is the interface descriptor for IGraphicBufferProducer.
const Descriptor = "android.gui.IGraphicBufferProducer"

// IGBPTransaction identifies an IGraphicBufferProducer transaction code.
type IGBPTransaction = binder.TransactionCode

// Transaction codes from IGraphicBufferProducer.cpp.
// IBinder::FIRST_CALL_TRANSACTION == 1.
const (
	RequestBuffer          IGBPTransaction = 1
	DequeueBuffer          IGBPTransaction = 2
	DetachBuffer           IGBPTransaction = 3
	DetachNextBuffer       IGBPTransaction = 4
	AttachBuffer           IGBPTransaction = 5
	QueueBuffer            IGBPTransaction = 6
	CancelBuffer           IGBPTransaction = 7
	Query                  IGBPTransaction = 8
	Connect                IGBPTransaction = 9
	Disconnect             IGBPTransaction = 10
	SetSidebandStream      IGBPTransaction = 11
	AllocateBuffers        IGBPTransaction = 12
	AllowAllocation        IGBPTransaction = 13
	SetGenerationNumber    IGBPTransaction = 14
	GetConsumerName        IGBPTransaction = 15
	SetMaxDequeuedBufCount IGBPTransaction = 16
	SetAsyncMode           IGBPTransaction = 17
	SetSharedBufferMode    IGBPTransaction = 18
	SetAutoRefresh         IGBPTransaction = 19
	SetDequeueTimeout      IGBPTransaction = 20
	GetLastQueuedBuffer    IGBPTransaction = 21
	GetFrameTimestamps     IGBPTransaction = 22
	GetUniqueId            IGBPTransaction = 23
	GetConsumerUsage       IGBPTransaction = 24
	SetLegacyBufferDrop    IGBPTransaction = 25
	SetAutoPrerotation     IGBPTransaction = 26
)

// NativeWindowQuery identifies a NATIVE_WINDOW query constant
// from <system/window.h>.
type NativeWindowQuery int32

// NATIVE_WINDOW query constants from <system/window.h>.
const (
	NativeWindowWidth             NativeWindowQuery = 0
	NativeWindowHeight            NativeWindowQuery = 1
	NativeWindowFormat            NativeWindowQuery = 2
	NativeWindowMinUndequeued     NativeWindowQuery = 3
	NativeWindowQueuesToComposer  NativeWindowQuery = 4
	NativeWindowConcreteType      NativeWindowQuery = 5
	NativeWindowDefaultWidth      NativeWindowQuery = 6
	NativeWindowDefaultHeight     NativeWindowQuery = 7
	NativeWindowTransformHint     NativeWindowQuery = 8
	NativeWindowConsumerRunning   NativeWindowQuery = 9
	NativeWindowConsumerUsageBits NativeWindowQuery = 10
	NativeWindowStickyTransform   NativeWindowQuery = 11
	NativeWindowDefaultDataspace  NativeWindowQuery = 12
	NativeWindowBufferAge         NativeWindowQuery = 13
	NativeWindowMaxBufferCount    NativeWindowQuery = 21

	NativeWindowSurface NativeWindowQuery = 1 // NATIVE_WINDOW_SURFACE for CONCRETE_TYPE
)

// PixelFormat identifies an Android HAL pixel format.
type PixelFormat int32

// Pixel formats.
const (
	PixelFormatImplementationDefined PixelFormat = 0x22 // HAL_PIXEL_FORMAT_IMPLEMENTATION_DEFINED
	PixelFormatYCbCr420_888          PixelFormat = 0x23 // HAL_PIXEL_FORMAT_YCbCr_420_888
)

// AndroidStatus represents an Android status/error code.
type AndroidStatus int32

// Android status codes.
const (
	StatusOK     AndroidStatus = 0
	StatusNoInit AndroidStatus = -19
)

// BufferNeedsRealloc is BUFFER_NEEDS_REALLOCATION = 0x1.
const BufferNeedsRealloc = 0x1

// GraphicBufferMagicGB01 is the 'GB01' magic: 'G'<<24 | 'B'<<16 | '0'<<8 | '1'.
const GraphicBufferMagicGB01 = int32(0x47423031)

// MaxBufferSlots matches BufferQueueDefs::NUM_BUFFER_SLOTS.
const MaxBufferSlots = 64
