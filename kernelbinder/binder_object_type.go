//go:build linux

package kernelbinder

// binderObjectType represents a binder object type code (e.g. BINDER_TYPE_HANDLE).
type binderObjectType uint32

// binderTypeHandle is BINDER_TYPE_HANDLE: B_PACK_CHARS('s','h','*',0x85) = 0x73682a85.
// Used to identify flat_binder_object entries containing remote binder handles.
const binderTypeHandle = binderObjectType(0x73682a85)
