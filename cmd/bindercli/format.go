package main

import (
	"github.com/AndroidGoLab/binder/cmd/bindercli/output"
)

// Formatter is an alias for output.Formatter for use within package main.
type Formatter = output.Formatter

// NewFormatter is a convenience alias for output.NewFormatter.
var NewFormatter = output.NewFormatter

// resolveMode wraps output.ResolveMode for backward compatibility
// within package main (used by tests).
func resolveMode(
	mode string,
	isTTY bool,
) string {
	return output.ResolveMode(mode, isTTY)
}
