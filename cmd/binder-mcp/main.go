//go:build linux

// binder-mcp is an MCP server that exposes Android binder services as
// tools for AI agents. In device mode it runs on-device (agents connect
// via adb shell); in remote mode it runs on the host and proxies binder
// transactions to a device through adb port-forwarding.
package main

import (
	"fmt"
	"os"

	"github.com/facebookincubator/go-belt"
	"github.com/facebookincubator/go-belt/tool/logger"
	"github.com/facebookincubator/go-belt/tool/logger/implementation/logrus"
	"github.com/spf13/cobra"
)

// Mode selects the operating mode of the MCP server.
type Mode string

const (
	// ModeDevice runs the MCP server on-device, opening /dev/binder directly.
	ModeDevice Mode = "device"

	// ModeRemote runs the MCP server on the host and proxies binder
	// transactions to an Android device via gadb.
	ModeRemote Mode = "remote"
)

func newRootCmd() *cobra.Command {
	logLevel := logger.LevelWarning
	mode := ModeDevice

	cmd := &cobra.Command{
		Use:   "binder-mcp",
		Short: "MCP server exposing Android binder services as AI-agent tools",
		Long: `binder-mcp serves Model Context Protocol (MCP) tools over stdio.

In device mode it opens /dev/binder directly (agents connect via
adb shell /data/local/tmp/binder-mcp). In remote mode it proxies
binder transactions to a device through adb port-forwarding.`,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			l := logrus.Default().WithLevel(logLevel)
			ctx := belt.CtxWithBelt(cmd.Context(), belt.New())
			ctx = logger.CtxWithLogger(ctx, l)
			cmd.SetContext(ctx)
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			switch mode {
			case ModeDevice:
				return runDevice(cmd, args)
			case ModeRemote:
				return runRemoteMode(cmd, args)
			default:
				return fmt.Errorf("unknown mode %q; supported: device, remote", mode)
			}
		},
	}

	cmd.PersistentFlags().Var(
		&logLevel,
		"log-level",
		"log level: trace, debug, info, warning, error, fatal, panic",
	)
	cmd.Flags().StringVar(
		(*string)(&mode),
		"mode",
		string(ModeDevice),
		"operating mode: device (on-device binder) or remote (adb bridge via gadb)",
	)
	cmd.Flags().String(
		"serial",
		"",
		"device serial for remote mode (empty = auto-discover first device)",
	)
	cmd.Flags().String(
		"binder-device",
		defaultBinderDevice,
		"path to the binder device (device mode)",
	)
	cmd.Flags().Int(
		"map-size",
		defaultMapSize,
		"binder mmap size in bytes (device mode)",
	)
	cmd.Flags().Int(
		"target-api",
		0,
		"Android API level (0 = auto-detect, device mode)",
	)

	return cmd
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
