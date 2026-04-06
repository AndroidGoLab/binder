package main

import (
	"fmt"
	"io"
	"os"
	"runtime/pprof"
	"strings"

	"github.com/facebookincubator/go-belt"
	"github.com/facebookincubator/go-belt/tool/logger"
	"github.com/facebookincubator/go-belt/tool/logger/implementation/logrus"
	"github.com/spf13/cobra"
)

const (
	defaultBinderDevice = "/dev/binder"
	defaultMapSize      = 128 * 1024
)

var cpuProfileFile *os.File

func newRootCmd() *cobra.Command {
	logLevel := logger.LevelWarning

	cmd := &cobra.Command{
		Use:   "bindercli",
		Short: "CLI tool for interacting with Android Binder services",
		Long: `bindercli is a command-line interface for listing, inspecting,
and invoking Android Binder services using AIDL-generated Go bindings.`,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			l := logrus.Default().WithLevel(logLevel)
			ctx := belt.CtxWithBelt(cmd.Context(), belt.New())
			ctx = logger.CtxWithLogger(ctx, l)
			cmd.SetContext(ctx)

			cpuFile, _ := cmd.Flags().GetString("cpuprofile")
			if cpuFile != "" {
				f, err := os.Create(cpuFile)
				if err != nil {
					return fmt.Errorf("creating CPU profile: %w", err)
				}
				cpuProfileFile = f
				if err := pprof.StartCPUProfile(f); err != nil {
					return fmt.Errorf("starting CPU profile: %w", err)
				}
			}
			return nil
		},
		PersistentPostRunE: func(cmd *cobra.Command, _ []string) error {
			cpuFile, _ := cmd.Flags().GetString("cpuprofile")
			if cpuFile != "" {
				pprof.StopCPUProfile()
				if cpuProfileFile != nil {
					cpuProfileFile.Close()
					cpuProfileFile = nil
				}
			}

			memFile, _ := cmd.Flags().GetString("memprofile")
			if memFile != "" {
				f, err := os.Create(memFile)
				if err != nil {
					return fmt.Errorf("creating memory profile: %w", err)
				}
				defer f.Close()
				if err := pprof.WriteHeapProfile(f); err != nil {
					return fmt.Errorf("writing memory profile: %w", err)
				}
			}
			return nil
		},
	}

	cmd.PersistentFlags().Var(
		&logLevel,
		"log-level",
		"log level: trace, debug, info, warning, error, fatal, panic",
	)
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
	cmd.PersistentFlags().Int(
		"target-api",
		0,
		"Android API level to target (0 = auto-detect from device)",
	)
	cmd.PersistentFlags().String(
		"cpuprofile",
		"",
		"write CPU profile to file",
	)
	cmd.PersistentFlags().String(
		"memprofile",
		"",
		"write memory profile to file",
	)

	cmd.AddCommand(newServiceCmd())
	cmd.AddCommand(newAIDLCmd())
	cmd.AddCommand(newCameraCmd())

	return cmd
}

// isUnknownCommandError reports whether err is cobra's "unknown command" error,
// which means the user invoked a subcommand that doesn't exist yet (likely a
// generated proxy command that hasn't been registered).
func isUnknownCommandError(err error) bool {
	return err != nil && strings.HasPrefix(err.Error(), "unknown command ")
}

func main() {
	// Phase 1: try executing with only the lightweight built-in commands
	// (service, aidl, camera). Suppress cobra's error output so that an
	// "unknown command" failure for a generated proxy command is invisible.
	// Stdout is left alone so --help and normal command output still work.
	cmd := newRootCmd()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetErr(io.Discard)

	err := cmd.Execute()
	switch {
	case err == nil:
		return
	case isUnknownCommandError(err):
		// Phase 2: the user invoked a generated command. Register all
		// generated commands and retry with normal output.
		addGeneratedCommands(cmd)
		cmd.SilenceErrors = false
		cmd.SilenceUsage = false
		cmd.SetErr(os.Stderr)
		if err := cmd.Execute(); err != nil {
			os.Exit(1)
		}
	default:
		// A built-in command failed. Print the error ourselves since
		// cobra's error printing was silenced for phase 1.
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
