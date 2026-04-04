//go:build linux

package conn

import (
	"context"
	"fmt"

	"github.com/facebookincubator/go-belt/tool/logger"
	"github.com/spf13/cobra"

	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/binder/versionaware"
	"github.com/AndroidGoLab/binder/kernelbinder"
	"github.com/AndroidGoLab/binder/servicemanager"
)

// Conn wraps the binder driver and service manager into a single
// connection handle for CLI subcommands.
type Conn struct {
	Driver    *kernelbinder.Driver
	Transport binder.Transport
	SM        *servicemanager.ServiceManager
}

// Open opens a binder driver connection and creates a service manager client.
func Open(
	ctx context.Context,
	cmd *cobra.Command,
) (_conn *Conn, _err error) {
	logger.Tracef(ctx, "Open")
	defer func() { logger.Tracef(ctx, "/Open: %v", _err) }()

	mapSize, err := cmd.Root().PersistentFlags().GetInt("map-size")
	if err != nil {
		return nil, fmt.Errorf("reading --map-size flag: %w", err)
	}

	targetAPI, err := cmd.Root().PersistentFlags().GetInt("target-api")
	if err != nil {
		return nil, fmt.Errorf("reading --target-api flag: %w", err)
	}

	// Detect API level BEFORE opening /dev/binder. Detection may fork
	// a child process (getprop), and forking after mmap-ing the binder
	// driver corrupts its state.
	if targetAPI <= 0 {
		targetAPI = versionaware.DetectAPILevel()
	}

	driver, err := kernelbinder.Open(ctx, binder.WithMapSize(uint32(mapSize)))
	if err != nil {
		return nil, fmt.Errorf("opening binder driver: %w", err)
	}

	// Wrap with version-aware transport so that ResolveCode returns
	// the correct transaction codes for the target device's API level.
	transport, err := versionaware.NewTransport(ctx, driver, targetAPI)
	if err != nil {
		driver.Close(ctx)
		return nil, fmt.Errorf("initializing version-aware transport: %w", err)
	}

	sm := servicemanager.New(transport)

	return &Conn{
		Driver:    driver,
		Transport: transport,
		SM:        sm,
	}, nil
}

// Close releases the binder driver resources.
func (c *Conn) Close(
	ctx context.Context,
) (_err error) {
	logger.Tracef(ctx, "Conn.Close")
	defer func() { logger.Tracef(ctx, "/Conn.Close: %v", _err) }()

	return c.Driver.Close(ctx)
}

// GetService looks up a registered binder service by name.
// Returns an error if the service is not found.
func (c *Conn) GetService(
	ctx context.Context,
	name string,
) (_binder binder.IBinder, _err error) {
	logger.Tracef(ctx, "GetService(%q)", name)
	defer func() { logger.Tracef(ctx, "/GetService(%q): %v", name, _err) }()

	svc, err := c.SM.CheckService(ctx, servicemanager.ServiceName(name))
	if err != nil {
		return nil, fmt.Errorf("checking service %q: %w", name, err)
	}

	if svc == nil {
		return nil, fmt.Errorf("service %q not found", name)
	}

	return svc, nil
}
