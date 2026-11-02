package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestProjection checks parsing, aspect fitting, rotation and invalid inputs.
func TestProjection(t *testing.T) {
	p, err := parseProjection("1080x2400@360x800/90")
	require.NoError(t, err)
	require.Equal(t, projection{1080, 2400, 360, 800, 90}, p)
	sw, sh, w, h := p.captureSize()
	require.EqualValues(t, 2400, sw)
	require.EqualValues(t, 1080, sh)
	require.EqualValues(t, 800, w)
	require.EqualValues(t, 360, h)
	for _, s := range []string{"", "0x100@10x10/0", "100x200@10x20/45", "100x200@10x20/90junk"} {
		_, err := parseProjection(s)
		require.Error(t, err)
	}
}

// TestProjectionFit clamps output dimensions and preserves source aspect.
func TestProjectionFit(t *testing.T) {
	p, err := parseProjection("1000x2000@900x900/270")
	require.NoError(t, err)
	require.EqualValues(t, 450, p.width)
	require.EqualValues(t, 900, p.height)
	sw, sh, w, h := p.captureSize()
	require.EqualValues(t, 2000, sw)
	require.EqualValues(t, 1000, sh)
	require.EqualValues(t, 900, w)
	require.EqualValues(t, 450, h)
	p, err = parseProjection("1000x2000@3000x4000/180")
	require.NoError(t, err)
	require.EqualValues(t, 1000, p.width)
	require.EqualValues(t, 2000, p.height)
}
