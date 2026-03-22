//go:build linux

package main

import (
	"context"
	"fmt"

	"github.com/facebookincubator/go-belt/tool/logger"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const defaultLogcatLines = 100

func registerGetLogcat(s *server.MCPServer) {
	tool := mcp.NewTool("get_logcat",
		mcp.WithDescription(
			"Read recent logcat entries using 'logcat -d'. "+
				"Returns the most recent N lines, optionally filtered by tag.",
		),
		mcp.WithNumber("lines",
			mcp.Description("Number of recent lines to return (default: 100)"),
		),
		mcp.WithString("tag",
			mcp.Description("Filter by log tag (e.g. 'ActivityManager', 'System.err')"),
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
	)

	s.AddTool(tool, handleGetLogcat)
}

func handleGetLogcat(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	logger.Tracef(ctx, "handleGetLogcat")
	defer func() { logger.Tracef(ctx, "/handleGetLogcat") }()

	lines := request.GetInt("lines", defaultLogcatLines)
	tag := request.GetString("tag", "")

	var cmd string
	switch tag {
	case "":
		cmd = fmt.Sprintf("logcat -d -t %d", lines)
	default:
		// Filter: show only the specified tag at verbose level, silence everything else.
		cmd = fmt.Sprintf("logcat -d -t %d -s %s:V", lines, shellQuote(tag))
	}

	out, err := shellExec(cmd)
	if err != nil {
		// logcat may return exit code 1 when buffer is empty but still produce output.
		if out == "" {
			return mcp.NewToolResultError(fmt.Sprintf("logcat: %v", err)), nil
		}
	}

	return mcp.NewToolResultText(out), nil
}
