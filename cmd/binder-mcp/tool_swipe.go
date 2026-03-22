//go:build linux

package main

import (
	"context"
	"fmt"

	"github.com/facebookincubator/go-belt/tool/logger"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const defaultSwipeDurationMS = 300

func registerSwipe(s *server.MCPServer) {
	tool := mcp.NewTool("swipe",
		mcp.WithDescription(
			"Swipe on the screen from (x1, y1) to (x2, y2) "+
				"over the specified duration using 'input swipe'.",
		),
		mcp.WithNumber("x1",
			mcp.Required(),
			mcp.Description("Start X coordinate in pixels"),
		),
		mcp.WithNumber("y1",
			mcp.Required(),
			mcp.Description("Start Y coordinate in pixels"),
		),
		mcp.WithNumber("x2",
			mcp.Required(),
			mcp.Description("End X coordinate in pixels"),
		),
		mcp.WithNumber("y2",
			mcp.Required(),
			mcp.Description("End Y coordinate in pixels"),
		),
		mcp.WithNumber("duration_ms",
			mcp.Description("Swipe duration in milliseconds (default: 300)"),
		),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(false),
	)

	s.AddTool(tool, handleSwipe)
}

func handleSwipe(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	logger.Tracef(ctx, "handleSwipe")
	defer func() { logger.Tracef(ctx, "/handleSwipe") }()

	x1, err := request.RequireInt("x1")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	y1, err := request.RequireInt("y1")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	x2, err := request.RequireInt("x2")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	y2, err := request.RequireInt("y2")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	duration := request.GetInt("duration_ms", defaultSwipeDurationMS)

	cmd := fmt.Sprintf("input swipe %d %d %d %d %d", x1, y1, x2, y2, duration)
	out, err := shellExec(cmd)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("input swipe: %v", err)), nil
	}

	if out == "" {
		out = fmt.Sprintf("swiped from (%d, %d) to (%d, %d) in %dms", x1, y1, x2, y2, duration)
	}

	return mcp.NewToolResultText(out), nil
}
