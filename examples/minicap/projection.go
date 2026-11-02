package main

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// parseProjection parses and fits a minicap projection to its real display.
//
// Example: 1080x2400@360x800/0 scales both dimensions by three.
func parseProjection(s string) (projection, error) {
	var p projection
	pieces := strings.FieldsFunc(s, func(r rune) bool { return r == 'x' || r == '@' || r == '/' })
	if len(pieces) != 5 {
		return p, fmt.Errorf("invalid projection %q", s)
	}
	expected := pieces[0] + "x" + pieces[1] + "@" + pieces[2] + "x" + pieces[3] + "/" + pieces[4]
	if s != expected {
		return p, fmt.Errorf("invalid projection %q", s)
	}
	values := [5]int64{}
	for i, part := range pieces {
		for _, r := range part {
			if r < '0' || r > '9' {
				return p, fmt.Errorf("invalid projection %q", s)
			}
		}
		n, err := strconv.ParseInt(part, 10, 32)
		if err != nil {
			return p, fmt.Errorf("invalid projection %q: %w", s, err)
		}
		values[i] = n
	}
	for _, v := range values[:4] {
		if v < 1 {
			return p, fmt.Errorf("projection dimensions must be positive")
		}
	}
	if values[4] != 0 && values[4] != 90 && values[4] != 180 && values[4] != 270 {
		return p, fmt.Errorf("invalid rotation %d", values[4])
	}
	p = projection{int32(values[0]), int32(values[1]), int32(values[2]), int32(values[3]), int(values[4])}
	p.width = min(p.width, p.realWidth)
	p.height = min(p.height, p.realHeight)
	aspect := float64(p.realWidth) / float64(p.realHeight)
	if float64(p.height) > float64(p.width)/aspect {
		p.height = int32(math.Round(float64(p.width) / aspect))
	} else {
		p.width = int32(math.Round(float64(p.height) * aspect))
	}
	if p.width < 1 || p.height < 1 {
		return projection{}, fmt.Errorf("projection is too small")
	}
	return p, nil
}

// captureSize returns SurfaceFlinger dimensions in the requested orientation.
//
// Example: quarter turns exchange both source and output axes.
func (p projection) captureSize() (int32, int32, int32, int32) {
	if p.rotation == 90 || p.rotation == 270 {
		return p.realHeight, p.realWidth, p.height, p.width
	}
	return p.realWidth, p.realHeight, p.width, p.height
}
