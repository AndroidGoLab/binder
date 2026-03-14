package parser

import (
	"fmt"
)

// Position represents a source location in an AIDL file.
type Position struct {
	Filename string
	Line     int
	Column   int
}

// String returns a human-readable representation of the position.
func (p Position) String() string {
	if p.Filename != "" {
		return fmt.Sprintf("%s:%d:%d", p.Filename, p.Line, p.Column)
	}
	return fmt.Sprintf("%d:%d", p.Line, p.Column)
}
