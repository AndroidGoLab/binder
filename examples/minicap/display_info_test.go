package main

import (
	"github.com/stretchr/testify/require"
	"testing"
)

// TestChooseDisplay selects the default or an exact physical display ID.
func TestChooseDisplay(t *testing.T) {
	id, err := chooseDisplay([]uint64{42, 99}, 0)
	require.NoError(t, err)
	require.EqualValues(t, 42, id)
	id, err = chooseDisplay([]uint64{42, 99}, 99)
	require.NoError(t, err)
	require.EqualValues(t, 99, id)
	_, err = chooseDisplay([]uint64{42, 99}, 7)
	require.Error(t, err)
	_, err = chooseDisplay(nil, 0)
	require.Error(t, err)
}
