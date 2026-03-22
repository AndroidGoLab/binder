package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/AndroidGoLab/binder/tools/pkg/codegen"
	"github.com/AndroidGoLab/binder/tools/pkg/parser"
	"github.com/AndroidGoLab/binder/tools/pkg/resolver"
)

func newAIDLCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "aidl",
		Short: "AIDL compiler toolchain",
	}

	cmd.AddCommand(newAIDLCompileCmd())
	cmd.AddCommand(newAIDLParseCmd())
	cmd.AddCommand(newAIDLCheckCmd())

	return cmd
}

func newAIDLCompileCmd() *cobra.Command {
	var searchPaths []string
	var outputDir string

	cmd := &cobra.Command{
		Use:   "compile <files...>",
		Short: "Compile AIDL files to Go code",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r := resolver.New(searchPaths)

			for _, f := range args {
				if err := r.ResolveFile(f); err != nil {
					return fmt.Errorf("resolving %s: %w", f, err)
				}
			}

			gen := codegen.NewGenerator(r, outputDir)
			if err := gen.GenerateAll(); err != nil {
				return fmt.Errorf("generating code: %w", err)
			}

			return nil
		},
	}

	cmd.Flags().StringSliceVarP(&searchPaths, "I", "I", nil, "search paths for AIDL imports (can be repeated)")
	cmd.Flags().StringVar(&outputDir, "output", "gen", "output directory for generated Go files")

	return cmd
}

func newAIDLParseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "parse <file>",
		Short: "Parse an AIDL file and print the AST as JSON",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			doc, err := parser.ParseFile(args[0])
			if err != nil {
				return fmt.Errorf("parsing %s: %w", args[0], err)
			}

			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			if err := enc.Encode(doc); err != nil {
				return fmt.Errorf("encoding JSON: %w", err)
			}

			return nil
		},
	}
}

func newAIDLCheckCmd() *cobra.Command {
	var searchPaths []string

	cmd := &cobra.Command{
		Use:   "check <files...>",
		Short: "Check AIDL files for errors without generating code",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r := resolver.New(searchPaths)

			successCount := 0
			failureCount := 0

			for _, f := range args {
				if err := r.ResolveFile(f); err != nil {
					fmt.Fprintf(os.Stderr, "FAIL: %s: %v\n", f, err)
					failureCount++
					continue
				}
				fmt.Fprintf(os.Stdout, "OK:   %s\n", f)
				successCount++
			}

			fmt.Fprintf(os.Stdout, "\n%d succeeded, %d failed\n", successCount, failureCount)

			if failureCount > 0 {
				return fmt.Errorf("%d file(s) failed validation", failureCount)
			}

			return nil
		},
	}

	cmd.Flags().StringSliceVarP(&searchPaths, "I", "I", nil, "search paths for AIDL imports (can be repeated)")

	return cmd
}
