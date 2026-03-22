//go:build linux

package main

import (
	"fmt"
	"log"
	"os"

	"github.com/facebookincubator/go-belt/tool/logger"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"

	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/binder/versionaware"
	"github.com/AndroidGoLab/binder/kernelbinder"
	"github.com/AndroidGoLab/binder/servicemanager"
)

const (
	defaultBinderDevice = "/dev/binder"
	defaultMapSize      = 128 * 1024
)

// runDevice implements the on-device MCP server mode: opens /dev/binder,
// creates a service manager connection, and serves MCP tools over stdio.
func runDevice(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()

	mapSize, err := cmd.Flags().GetInt("map-size")
	if err != nil {
		return fmt.Errorf("reading --map-size: %w", err)
	}

	targetAPI, err := cmd.Flags().GetInt("target-api")
	if err != nil {
		return fmt.Errorf("reading --target-api: %w", err)
	}

	// Detect API level before opening /dev/binder. Detection may fork
	// a child process (getprop), and forking after mmap-ing the binder
	// driver corrupts its state.
	if targetAPI <= 0 {
		targetAPI = versionaware.DetectAPILevel()
	}

	logger.Debugf(ctx, "opening binder driver (mapSize=%d, targetAPI=%d)", mapSize, targetAPI)

	driver, err := kernelbinder.Open(ctx, binder.WithMapSize(uint32(mapSize)))
	if err != nil {
		return fmt.Errorf("opening binder driver: %w", err)
	}
	defer driver.Close(ctx)

	transport, err := versionaware.NewTransport(ctx, driver, targetAPI)
	if err != nil {
		return fmt.Errorf("initializing version-aware transport: %w", err)
	}

	sm := servicemanager.New(transport)

	tools := &ToolSet{
		sm: sm,
	}

	mcpServer := server.NewMCPServer(
		"binder-mcp",
		"1.0.0",
		server.WithToolCapabilities(false),
		server.WithRecovery(),
	)

	tools.Register(mcpServer)
	RegisterShellTools(mcpServer)

	logger.Debugf(ctx, "serving MCP over stdio")

	// Redirect stderr for the MCP error logger so it does not
	// contaminate the JSON-RPC stream on stdout.
	errLogger := log.New(os.Stderr, "binder-mcp: ", log.LstdFlags)

	return server.ServeStdio(mcpServer, server.WithErrorLogger(errLogger))
}
