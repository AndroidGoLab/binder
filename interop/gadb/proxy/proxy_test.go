package proxy_test

import (
	"context"
	"testing"

	"github.com/electricbubble/gadb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/interop/gadb/proxy"
	"github.com/AndroidGoLab/binder/parcel"
)

const testDeviceSerial = "41041JEKB08092"

// requireADB skips the test if the ADB server is not reachable.
func requireADB(t *testing.T) {
	t.Helper()
	if _, err := gadb.NewClient(); err != nil {
		t.Skipf("ADB server not available: %v", err)
	}
}

func TestSession(t *testing.T) {
	requireADB(t)

	ctx := context.Background()

	session, err := proxy.NewSession(ctx, testDeviceSerial)
	require.NoError(t, err, "NewSession must succeed")
	defer func() {
		require.NoError(t, session.Close(ctx))
	}()

	transport := session.Transport()
	require.NotNil(t, transport)

	const smDescriptor = "android.os.IServiceManager"

	// Verify basic connectivity with INTERFACE_TRANSACTION (fixed code).
	t.Run("InterfaceTransaction", func(t *testing.T) {
		data := parcel.New()
		defer data.Recycle()

		reply, err := transport.Transact(
			ctx, smDescriptor,
			uint32(binder.InterfaceTransaction), 0, data,
		)
		require.NoError(t, err, "INTERFACE_TRANSACTION must succeed")
		defer reply.Recycle()

		desc, err := reply.ReadString16()
		require.NoError(t, err)
		assert.Equal(t, smDescriptor, desc,
			"ServiceManager must report its own descriptor")
	})

	// List services by probing candidate codes for listServices.
	// The code is FirstCallTransaction + N where N varies by device
	// revision. We try all candidates and use the first that succeeds.
	t.Run("ListServices", func(t *testing.T) {
		const dumpFlagPriorityAll = int32(1 | 2 | 4 | 8)

		var services []string
		found := false

		for offset := uint32(0); offset <= 13 && !found; offset++ {
			code := uint32(binder.FirstCallTransaction) + offset

			data := parcel.New()
			data.WriteInterfaceToken(smDescriptor)
			data.WriteInt32(dumpFlagPriorityAll)

			reply, err := transport.Transact(ctx, smDescriptor, code, 0, data)
			data.Recycle()
			if err != nil {
				continue
			}

			status, err := reply.ReadInt32()
			if err != nil || status != 0 {
				reply.Recycle()
				continue
			}

			count, err := reply.ReadInt32()
			if err != nil || count < 100 {
				reply.Recycle()
				continue
			}

			t.Logf("listServices resolved to code %d (offset %d), count=%d", code, offset, count)

			for i := int32(0); i < count; i++ {
				name, err := reply.ReadString16()
				if err != nil {
					break
				}
				services = append(services, name)
			}
			reply.Recycle()
			found = true
		}

		require.True(t, found, "must find a working listServices code")
		t.Logf("found %d services", len(services))
		assert.Greater(t, len(services), 100,
			"should have >100 services on a real device")

		// Log first few for inspection.
		for i, name := range services {
			if i >= 5 {
				break
			}
			t.Logf("  service[%d]: %s", i, name)
		}
	})
}
