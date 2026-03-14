package aidlerrors

// BinderError represents a driver-level Binder error (e.g., ioctl failure).
type BinderError struct {
	Op  string // operation that failed (e.g., "ioctl", "mmap")
	Err error  // underlying error (typically syscall.Errno)
}

func (e *BinderError) Error() string {
	return "binder: " + e.Op + ": " + e.Err.Error()
}

func (e *BinderError) Unwrap() error {
	return e.Err
}
