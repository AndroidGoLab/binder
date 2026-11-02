package main

import (
	"bytes"
	"context"
	"flag"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestOptions checks minicap-compatible flags and validates incompatible modes.
func TestOptions(t *testing.T) {
	o, err := parseOptions([]string{"-P", "1080x2400@360x800/90", "-Q", "0", "-n", "stream", "-S"})
	require.NoError(t, err)
	require.Equal(t, 0, o.quality)
	require.Equal(t, "stream", o.socketName)
	require.True(t, o.skip)
	require.Equal(t, 90, o.projection.rotation)
	_, err = parseOptions([]string{"-P", "1080x2400@360x800/0", "-Q", "101"})
	require.Error(t, err)
	_, err = parseOptions([]string{"-s", "-t"})
	require.Error(t, err)
	_, err = parseOptions([]string{"-P", "invalid"})
	require.Error(t, err)
}

// TestOptionsRejectsInvalidFrameRate checks that explicit nonpositive rates fail.
func TestOptionsRejectsInvalidFrameRate(t *testing.T) {
	for _, v := range []string{"0", "-1", "NaN", "Inf"} {
		_, err := parseOptions([]string{"-P", "100x200@50x100/0", "-r", v})
		require.Error(t, err)
	}
}

// TestOptionsHelp recognizes the conventional help flag.
func TestOptionsHelp(t *testing.T) {
	_, err := parseOptions([]string{"-h"})
	require.ErrorIs(t, err, flag.ErrHelp)
}

// TestHelpExitsSuccessfully checks the CLI's user-facing help behavior.
func TestHelpExitsSuccessfully(t *testing.T) {
	var out bytes.Buffer
	err := run(context.Background(), []string{"-h"}, &out)
	require.NoError(t, err)
	require.Contains(t, out.String(), "-P")
}

// TestFrameDelay starts rate limiting at acquisition, after any delayed accept.
func TestFrameDelay(t *testing.T) {
	start := time.Unix(100, 0)
	require.Equal(t, 50*time.Millisecond, frameDelay(start, start.Add(50*time.Millisecond), 10))
	require.Equal(t, time.Duration(0), frameDelay(start, start.Add(200*time.Millisecond), 10))
}

// TestOptionsAcceptsAttachedShortValues checks common getopt command forms.
func TestOptionsAcceptsAttachedShortValues(t *testing.T) {
	o, err := parseOptions([]string{"-P1080x2400@360x800/0", "-Q80", "-nstream", "-S"})
	require.NoError(t, err)
	require.Equal(t, 80, o.quality)
	require.Equal(t, "stream", o.socketName)
	require.True(t, o.skip)
	o, err = parseOptions([]string{"-P1080x2400@360x800/0", "-sS"})
	require.NoError(t, err)
	require.True(t, o.screenshot)
	require.True(t, o.skip)
}
