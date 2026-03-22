//go:build linux

package main

import (
	"context"
	"fmt"

	"github.com/facebookincubator/go-belt/tool/logger"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerTap(s *server.MCPServer) {
	tool := mcp.NewTool("tap",
		mcp.WithDescription(
			"Tap the screen at the specified coordinates using 'input tap'.",
		),
		mcp.WithNumber("x",
			mcp.Required(),
			mcp.Description("X coordinate in pixels"),
		),
		mcp.WithNumber("y",
			mcp.Required(),
			mcp.Description("Y coordinate in pixels"),
		),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(false),
	)

	s.AddTool(tool, handleTap)
}

func handleTap(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	logger.Tracef(ctx, "handleTap")
	defer func() { logger.Tracef(ctx, "/handleTap") }()

	x, err := request.RequireInt("x")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	y, err := request.RequireInt("y")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	cmd := fmt.Sprintf("input tap %d %d", x, y)
	out, err := shellExec(cmd)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("input tap: %v", err)), nil
	}

	if out == "" {
		out = fmt.Sprintf("tapped at (%d, %d)", x, y)
	}

	return mcp.NewToolResultText(out), nil
}
