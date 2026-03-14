//go:build linux

package main

import (
	"context"
	"fmt"

	"github.com/facebookincubator/go-belt/tool/logger"
	"github.com/spf13/cobra"
	"github.com/xaionaro-go/aidl/binder"
	"github.com/xaionaro-go/aidl/kernelbinder"
	"github.com/xaionaro-go/aidl/servicemanager"
)

// Conn wraps the binder driver and service manager into a single
// connection handle for CLI subcommands.
type Conn struct {
	Driver *kernelbinder.Driver
	SM     *servicemanager.ServiceManager
}

// OpenConn opens a binder driver connection and creates a service manager client.
func OpenConn(
	ctx context.Context,
	cmd *cobra.Command,
) (_conn *Conn, _err error) {
	logger.Tracef(ctx, "OpenConn")
	defer func() { logger.Tracef(ctx, "/OpenConn: %v", _err) }()

	mapSize, err := cmd.Root().PersistentFlags().GetInt("map-size")
	if err != nil {
		return nil, fmt.Errorf("reading --map-size flag: %w", err)
	}

	driver, err := kernelbinder.Open(ctx, binder.WithMapSize(uint32(mapSize)))
	if err != nil {
		return nil, fmt.Errorf("opening binder driver: %w", err)
	}

	sm := servicemanager.New(driver)

	return &Conn{
		Driver: driver,
		SM:     sm,
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

	svc, err := c.SM.CheckService(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("checking service %q: %w", name, err)
	}

	if svc == nil {
		return nil, fmt.Errorf("service %q not found", name)
	}

	return svc, nil
}
