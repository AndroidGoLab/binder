//go:build !unix

package parcel

import "fmt"

// duplicateFileDescriptor reports that descriptor duplication is unavailable.
//
// Example: reject owned decoding on platforms without Unix descriptors.
func duplicateFileDescriptor(fd int32) (int32, error) {
	return 0, fmt.Errorf("descriptor duplication is unsupported on this platform")
}
