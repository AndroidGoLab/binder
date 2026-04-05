//go:build linux

package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/binder/versionaware"
	"github.com/AndroidGoLab/binder/cmd/bindercli/discovery"
	"github.com/AndroidGoLab/binder/parcel"
	"github.com/AndroidGoLab/binder/servicemanager"
)

func newServiceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "service",
		Short: "List, inspect, and transact with binder services",
	}

	cmd.AddCommand(newServiceListCmd())
	cmd.AddCommand(newServiceInspectCmd())
	cmd.AddCommand(newServiceTransactCmd())
	cmd.AddCommand(newServiceMethodsCmd())
	cmd.AddCommand(newServiceResolveCmd())

	return cmd
}

func newServiceListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all registered binder services",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			conn, err := OpenConn(ctx, cmd)
			if err != nil {
				return err
			}
			defer conn.Close(ctx)

			services, err := conn.SM.ListServices(ctx)
			if err != nil {
				return fmt.Errorf("listing services: %w", err)
			}

			headers := []string{"NAME", "STATUS"}
			rows := make([][]string, 0, len(services))
			for _, name := range services {
				status := serviceStatus(ctx, conn, string(name))
				rows = append(rows, []string{string(name), status})
			}

			mode, err := cmd.Root().PersistentFlags().GetString("format")
			if err != nil {
				return fmt.Errorf("reading --format flag: %w", err)
			}

			NewFormatter(mode, os.Stdout).Table(headers, rows)
			return nil
		},
	}
}

func serviceStatus(
	ctx context.Context,
	conn *Conn,
	name string,
) string {
	svc, err := conn.SM.CheckService(ctx, servicemanager.ServiceName(name))
	if err != nil || svc == nil {
		return "not found"
	}

	if svc.IsAlive(ctx) {
		return "alive"
	}
	return "dead"
}

func newServiceInspectCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "inspect <name>",
		Short: "Inspect a binder service by name",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			name := args[0]

			conn, err := OpenConn(ctx, cmd)
			if err != nil {
				return err
			}
			defer conn.Close(ctx)

			svc, err := conn.GetService(ctx, name)
			if err != nil {
				return err
			}

			descriptor := discovery.QueryDescriptor(ctx, svc)

			mode, err := cmd.Root().PersistentFlags().GetString("format")
			if err != nil {
				return fmt.Errorf("reading --format flag: %w", err)
			}

			NewFormatter(mode, os.Stdout).Result(map[string]any{
				"name":       name,
				"handle":     svc.Handle(),
				"alive":      svc.IsAlive(ctx),
				"descriptor": descriptor,
			})
			return nil
		},
	}
}

func newServiceTransactCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "transact <name> <code-or-method> [hex-data]",
		Short: "Send a raw transaction to a binder service",
		Args:  cobra.RangeArgs(2, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			name := args[0]
			codeArg := args[1]

			var txCode binder.TransactionCode
			isNumeric := false

			// Try parsing as numeric transaction code first.
			if parsed, parseErr := strconv.ParseUint(codeArg, 0, 32); parseErr == nil {
				txCode = binder.TransactionCode(parsed)
				isNumeric = true
			}

			var data *parcel.Parcel
			if len(args) >= 3 {
				raw, err := hex.DecodeString(args[2])
				if err != nil {
					return fmt.Errorf("decoding hex data: %w", err)
				}
				data = parcel.FromBytes(raw)
			} else {
				data = parcel.New()
			}

			conn, err := OpenConn(ctx, cmd)
			if err != nil {
				return err
			}
			defer conn.Close(ctx)

			svc, err := conn.GetService(ctx, name)
			if err != nil {
				return err
			}

			// If not numeric, resolve method name to code.
			if !isNumeric {
				descriptor, descErr := descriptorForBinder(ctx, svc, name)
				if descErr != nil {
					return fmt.Errorf("resolving method name %q: %w", codeArg, descErr)
				}

				table, tableErr := getActiveTable(conn)
				if tableErr != nil {
					return fmt.Errorf("resolving method name %q: %w", codeArg, tableErr)
				}

				methodName := kebabToMethod(codeArg, descriptor)

				resolved, ok := resolveMethodToCode(table, descriptor, methodName)
				if !ok {
					return fmt.Errorf("method %q not found on interface %s", codeArg, descriptor)
				}
				txCode = resolved
			}

			reply, err := svc.Transact(ctx, txCode, 0, data)
			if err != nil {
				return fmt.Errorf("transact failed: %w", err)
			}

			replyData := reply.Data()

			mode, err := cmd.Root().PersistentFlags().GetString("format")
			if err != nil {
				return fmt.Errorf("reading --format flag: %w", err)
			}

			NewFormatter(mode, os.Stdout).Result(map[string]any{
				"reply_size": len(replyData),
				"reply_hex":  hex.EncodeToString(replyData),
			})
			return nil
		},
	}
}

func newServiceMethodsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "methods <name>",
		Short: "List methods of a binder service interface",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			name := args[0]

			conn, err := OpenConn(ctx, cmd)
			if err != nil {
				return err
			}
			defer conn.Close(ctx)

			svc, err := conn.GetService(ctx, name)
			if err != nil {
				return err
			}

			descriptor := discovery.QueryDescriptor(ctx, svc)

			// InterfaceTransaction may return empty on some services.
			// Fall back to the static knownServiceNames map (reverse lookup).
			if descriptor == "" || descriptor == "(unknown)" {
				for desc, svcName := range discovery.KnownServiceNames {
					if svcName == name {
						descriptor = desc
						break
					}
				}
			}

			mode, err := cmd.Root().PersistentFlags().GetString("format")
			if err != nil {
				return fmt.Errorf("reading --format flag: %w", err)
			}

			if generatedRegistry == nil {
				return fmt.Errorf("no method registry available")
			}

			info := generatedRegistry.ByDescriptor(descriptor)
			if info == nil {
				// Also try looking up by alias (the service name itself).
				info = generatedRegistry.ByAlias(name)
			}
			if info == nil {
				return fmt.Errorf("unknown interface %q for service %q — not in registry", descriptor, name)
			}

			table, tableErr := getActiveTable(conn)

			f := NewFormatter(mode, os.Stdout)
			switch f.Mode {
			case "json":
				f.WriteJSON(map[string]any{
					"descriptor": descriptor,
					"methods":    methodsToJSONWithCodes(info.Methods, table, descriptor),
				})
			default:
				fmt.Fprintf(f.W, "Interface: %s (%d methods)\n\n", descriptor, len(info.Methods))
				for _, m := range info.Methods {
					codeStr := "  ????  "
					if tableErr == nil {
						if code, ok := resolveMethodToCode(table, descriptor, m.Name); ok {
							codeStr = fmt.Sprintf("  0x%04x", code)
						}
					}
					fmt.Fprintf(f.W, "%s  %s\n", codeStr, formatMethodSignature(m))
				}
			}

			return nil
		},
	}
}

// formatMethodSignature renders a MethodInfo as "kebab-name(params...) -> returnType".
func formatMethodSignature(m MethodInfo) string {
	var b strings.Builder
	b.WriteString(camelToKebab(m.Name))
	b.WriteByte('(')
	for i, p := range m.Params {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(p.Name)
		b.WriteByte(' ')
		b.WriteString(p.Type)
	}
	b.WriteByte(')')

	if m.ReturnType != "" {
		b.WriteString(" -> ")
		b.WriteString(m.ReturnType)
	}

	return b.String()
}

// methodsToJSON converts a slice of MethodInfo into a JSON-friendly representation.
func methodsToJSON(methods []MethodInfo) []map[string]any {
	result := make([]map[string]any, 0, len(methods))
	for _, m := range methods {
		entry := map[string]any{
			"name": camelToKebab(m.Name),
		}

		if len(m.Params) > 0 {
			params := make([]map[string]string, 0, len(m.Params))
			for _, p := range m.Params {
				params = append(params, map[string]string{
					"name": p.Name,
					"type": p.Type,
				})
			}
			entry["params"] = params
		}

		if m.ReturnType != "" {
			entry["return_type"] = m.ReturnType
		}

		result = append(result, entry)
	}
	return result
}

func methodsToJSONWithCodes(
	methods []MethodInfo,
	table versionaware.VersionTable,
	descriptor string,
) []map[string]any {
	result := make([]map[string]any, 0, len(methods))
	for _, m := range methods {
		entry := map[string]any{
			"name": camelToKebab(m.Name),
		}

		if table != nil {
			if code, ok := resolveMethodToCode(table, descriptor, m.Name); ok {
				entry["code"] = fmt.Sprintf("0x%04x", code)
			}
		}

		if len(m.Params) > 0 {
			params := make([]map[string]string, 0, len(m.Params))
			for _, p := range m.Params {
				params = append(params, map[string]string{
					"name": p.Name,
					"type": p.Type,
				})
			}
			entry["params"] = params
		}

		if m.ReturnType != "" {
			entry["return_type"] = m.ReturnType
		}

		result = append(result, entry)
	}
	return result
}

func newServiceResolveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resolve <name> <method-or-code>",
		Short: "Resolve a transaction code or method name for a binder service",
		Long: `Resolve translates between method names and transaction codes.

If the second argument is a number, it is treated as a transaction code and
resolved to a method name. Otherwise it is treated as a method name (in
camelCase or kebab-case) and resolved to a transaction code.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			name := args[0]
			query := args[1]

			conn, err := OpenConn(ctx, cmd)
			if err != nil {
				return err
			}
			defer conn.Close(ctx)

			descriptor, err := resolveDescriptor(ctx, conn, name)
			if err != nil {
				return err
			}

			table, err := getActiveTable(conn)
			if err != nil {
				return err
			}

			mode, err := cmd.Root().PersistentFlags().GetString("format")
			if err != nil {
				return fmt.Errorf("reading --format flag: %w", err)
			}

			f := NewFormatter(mode, os.Stdout)

			// Try parsing as numeric transaction code first.
			if code, parseErr := strconv.ParseUint(query, 0, 32); parseErr == nil {
				methodName, ok := resolveCodeToMethod(table, descriptor, binder.TransactionCode(code))
				if !ok {
					return fmt.Errorf("no method found for code %#x on %s", code, descriptor)
				}
				f.Result(map[string]any{
					"descriptor": descriptor,
					"method":     camelToKebab(methodName),
					"code":       fmt.Sprintf("0x%04x", code),
				})
				return nil
			}

			// Treat as method name -- support kebab-case via registry lookup.
			methodName := kebabToMethod(query, descriptor)

			code, ok := resolveMethodToCode(table, descriptor, methodName)
			if !ok {
				return fmt.Errorf("method %q not found on %s", query, descriptor)
			}

			f.Result(map[string]any{
				"descriptor": descriptor,
				"method":     camelToKebab(methodName),
				"code":       fmt.Sprintf("0x%04x", code),
			})
			return nil
		},
	}
}
