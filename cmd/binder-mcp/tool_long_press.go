//go:build linux

package main

import (
	"context"
	"fmt"

	"github.com/facebookincubator/go-belt/tool/logger"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const defaultLongPressDurationMS = 1000

func registerLongPress(s *server.MCPServer) {
	tool := mcp.NewTool("long_press",
		mcp.WithDescription(
			"Long-press the screen at the specified coordinates. "+
				"Implemented as 'input swipe' with identical start and end points.",
		),
		mcp.WithNumber("x",
			mcp.Required(),
			mcp.Description("X coordinate in pixels"),
		),
		mcp.WithNumber("y",
			mcp.Required(),
			mcp.Description("Y coordinate in pixels"),
		),
		mcp.WithNumber("duration_ms",
			mcp.Description("Press duration in milliseconds (default: 1000)"),
		),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(false),
	)

	s.AddTool(tool, handleLongPress)
}

func handleLongPress(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	logger.Tracef(ctx, "handleLongPress")
	defer func() { logger.Tracef(ctx, "/handleLongPress") }()

	x, err := request.RequireInt("x")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	y, err := request.RequireInt("y")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	duration := request.GetInt("duration_ms", defaultLongPressDurationMS)

	cmd := fmt.Sprintf("input swipe %d %d %d %d %d", x, y, x, y, duration)
	out, err := shellExec(cmd)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("input swipe (long press): %v", err)), nil
	}

	if out == "" {
		out = fmt.Sprintf("long-pressed at (%d, %d) for %dms", x, y, duration)
	}

	return mcp.NewToolResultText(out), nil
}
