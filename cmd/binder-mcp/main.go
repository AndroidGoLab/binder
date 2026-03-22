//go:build linux

// binder-mcp is an MCP server that exposes Android binder services as
// tools for AI agents. It runs on-device and communicates via stdio,
// so an agent can connect through `adb shell /data/local/tmp/binder-mcp`.
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

	// ModeRemote is a placeholder for future adb-bridge mode.
	ModeRemote Mode = "remote"
)

func newRootCmd() *cobra.Command {
	logLevel := logger.LevelWarning
	mode := ModeDevice

	cmd := &cobra.Command{
		Use:   "binder-mcp",
		Short: "MCP server exposing Android binder services as AI-agent tools",
		Long: `binder-mcp opens /dev/binder and serves Model Context Protocol (MCP)
tools over stdio. AI agents connect via adb shell /data/local/tmp/binder-mcp.`,
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
				return fmt.Errorf("remote mode is not yet implemented")
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
		"operating mode: device (on-device binder) or remote (adb bridge, not yet implemented)",
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
