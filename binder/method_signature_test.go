package binder

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSignatureMatches(t *testing.T) {
	t.Run("identical", func(t *testing.T) {
		compiled := []string{"I", "Z", "Landroid/content/AttributionSource;"}
		device := []string{"I", "Z", "Landroid/content/AttributionSource;"}
		assert.True(t, SignatureMatches(compiled, device))
	})

	t.Run("different_length", func(t *testing.T) {
		compiled := []string{"I", "Z", "Landroid/content/AttributionSource;"}
		device := []string{"I", "Z"}
		assert.False(t, SignatureMatches(compiled, device))
	})

	t.Run("different_type", func(t *testing.T) {
		compiled := []string{"I", "Z"}
		device := []string{"I", "J"}
		assert.False(t, SignatureMatches(compiled, device))
	})

	t.Run("both_empty", func(t *testing.T) {
		assert.True(t, SignatureMatches(nil, nil))
	})
}

func TestMatchParamsToSignature(t *testing.T) {
	t.Run("api36_vs_api35_registerClient", func(t *testing.T) {
		// API 36 (compiled): ParcelUuid, IBluetoothGattCallback, bool, int, AttributionSource
		compiled := []string{
			"Landroid/os/ParcelUuid;",
			"Landroid/bluetooth/IBluetoothGattCallback;",
			"Z",
			"I",
			"Landroid/content/AttributionSource;",
		}
		// API 35 (device): ParcelUuid, IBluetoothGattCallback, bool, AttributionSource
		device := []string{
			"Landroid/os/ParcelUuid;",
			"Landroid/bluetooth/IBluetoothGattCallback;",
			"Z",
			"Landroid/content/AttributionSource;",
		}

		result := MatchParamsToSignature(compiled, device)
		// Device param 0 → compiled param 0 (ParcelUuid)
		// Device param 1 → compiled param 1 (callback)
		// Device param 2 → compiled param 2 (bool)
		// Device param 3 → compiled param 4 (AttributionSource)
		// Compiled param 3 (int) is skipped.
		assert.Equal(t, []int{0, 1, 2, 4}, result)
	})

	t.Run("device_has_extra_param", func(t *testing.T) {
		compiled := []string{"I", "Z"}
		device := []string{"I", "J", "Z"}

		result := MatchParamsToSignature(compiled, device)
		// Device param 0 → compiled param 0 (I)
		// Device param 1 → -1 (J not in compiled)
		// Device param 2 → compiled param 1 (Z)
		assert.Equal(t, []int{0, -1, 1}, result)
	})

	t.Run("identical_signatures", func(t *testing.T) {
		compiled := []string{"I", "Z", "Ljava/lang/String;"}
		device := []string{"I", "Z", "Ljava/lang/String;"}

		result := MatchParamsToSignature(compiled, device)
		assert.Equal(t, []int{0, 1, 2}, result)
	})

	t.Run("empty_device_sig", func(t *testing.T) {
		compiled := []string{"I", "Z"}
		device := []string{}

		result := MatchParamsToSignature(compiled, device)
		assert.Empty(t, result)
	})

	t.Run("duplicate_types", func(t *testing.T) {
		// Two ints in compiled, one in device.
		compiled := []string{"I", "I", "Z"}
		device := []string{"I", "Z"}

		result := MatchParamsToSignature(compiled, device)
		// First I matches compiled[0], Z matches compiled[2].
		assert.Equal(t, []int{0, 2}, result)
	})
}
