//go:build linux

package main

import (
	"context"
	"fmt"

	"github.com/facebookincubator/go-belt/tool/logger"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerShellExec(s *server.MCPServer) {
	tool := mcp.NewTool("shell_exec",
		mcp.WithDescription(
			"Execute an arbitrary shell command on the device and return "+
				"combined stdout+stderr. This is a powerful escape hatch for "+
				"commands not covered by other tools. The command runs as the "+
				"shell UID (same as adb shell). Use with caution.",
		),
		mcp.WithString("command",
			mcp.Required(),
			mcp.Description("Shell command to execute (passed to sh -c)"),
		),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
	)

	s.AddTool(tool, handleShellExec)
}

func handleShellExec(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	logger.Tracef(ctx, "handleShellExec")
	defer func() { logger.Tracef(ctx, "/handleShellExec") }()

	command, err := request.RequireString("command")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	logger.Debugf(ctx, "shell_exec: %s", command)

	out, execErr := shellExec(command)
	if execErr != nil {
		return mcp.NewToolResultError(fmt.Sprintf("command failed: %v\noutput: %s", execErr, out)), nil
	}

	return mcp.NewToolResultText(out), nil
}
