package hidlcodec2

import "fmt"

// Status is the Codec2 status code.
//
// The HIDL types.hal defines Status with negative values, but the actual
// HIDL implementation (Configurable.cpp) casts raw c2_status_t values
// (positive POSIX errno) directly into the wire format:
//
//	_hidl_cb((Status)c2res, failures, outParams);
//
// So on the wire we see positive errno values (EINVAL=22, ENXIO=6, etc.),
// not the negative HIDL-spec values. We define both sets for correct
// matching.
type Status int32

// Positive-errno values as seen on the HIDL wire (c2_status_t raw values).
const (
	StatusOK        Status = 0
	StatusBadValue  Status = 22  // EINVAL
	StatusBadIndex  Status = 6   // ENXIO
	StatusCannotDo  Status = 95  // ENOTSUP (EOPNOTSUPP on some systems)
	StatusDuplicate Status = 17  // EEXIST
	StatusNotFound  Status = 2   // ENOENT
	StatusBadState  Status = 1   // EPERM
	StatusBlocking  Status = 11  // EAGAIN/EWOULDBLOCK
	StatusCanceled  Status = 4   // EINTR
	StatusNoMemory  Status = 12  // ENOMEM
	StatusRefused   Status = 13  // EACCES
	StatusTimedOut  Status = 110 // ETIMEDOUT
	StatusOmitted   Status = 38  // ENOSYS
	StatusCorrupted Status = 14  // EFAULT
	StatusNoInit    Status = 19  // ENODEV
)

// Err returns nil if status is OK, otherwise an error.
func (s Status) Err() error {
	if s == StatusOK {
		return nil
	}
	return fmt.Errorf("codec2 status %d (%s)", int32(s), s.String())
}

// String returns a human-readable name for the status code.
func (s Status) String() string {
	switch s {
	case StatusOK:
		return "OK"
	case StatusBadValue:
		return "BAD_VALUE"
	case StatusBadIndex:
		return "BAD_INDEX"
	case StatusCannotDo:
		return "CANNOT_DO"
	case StatusDuplicate:
		return "DUPLICATE"
	case StatusNotFound:
		return "NOT_FOUND"
	case StatusBadState:
		return "BAD_STATE"
	case StatusBlocking:
		return "BLOCKING"
	case StatusCanceled:
		return "CANCELED"
	case StatusNoMemory:
		return "NO_MEMORY"
	case StatusRefused:
		return "REFUSED"
	case StatusTimedOut:
		return "TIMED_OUT"
	case StatusOmitted:
		return "OMITTED"
	case StatusCorrupted:
		return "CORRUPTED"
	case StatusNoInit:
		return "NO_INIT"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", int32(s))
	}
}
