package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

const (
	defaultBinderDevice = "/dev/binder"
	defaultMapSize      = 128 * 1024
)

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "aidlcli",
		Short: "CLI tool for interacting with Android Binder services",
		Long: `aidlcli is a command-line interface for listing, inspecting,
and invoking Android Binder services using AIDL-generated Go bindings.`,
	}

	cmd.PersistentFlags().String(
		"format",
		"auto",
		"output format: json, text, or auto (detect terminal vs pipe)",
	)
	cmd.PersistentFlags().String(
		"binder-device",
		defaultBinderDevice,
		"path to the binder device",
	)
	cmd.PersistentFlags().Int(
		"map-size",
		defaultMapSize,
		"binder mmap size in bytes",
	)

	cmd.AddCommand(newServiceCmd())
	cmd.AddCommand(newAIDLCmd())
	addGeneratedCommands(cmd)

	return cmd
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
