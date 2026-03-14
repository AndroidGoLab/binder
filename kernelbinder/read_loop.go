//go:build linux

package kernelbinder

// readLoop is a background goroutine that reads async BR_* events
// (death notifications, etc.) and dispatches them.
// This is a placeholder for future use -- initial implementation
// only handles synchronous Transact.
