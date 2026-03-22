//go:build linux

package main

import (
	"context"
	"fmt"

	"github.com/facebookincubator/go-belt/tool/logger"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerStartActivity(s *server.MCPServer) {
	tool := mcp.NewTool("start_activity",
		mcp.WithDescription(
			"Start an Android activity using 'am start -n'. "+
				"The component should be in the format package/activity "+
				"(e.g. com.example.app/.MainActivity). Optional extras "+
				"are passed as raw 'am start' flags.",
		),
		mcp.WithString("component",
			mcp.Required(),
			mcp.Description("Component name: package/.Activity (e.g. com.android.settings/.Settings)"),
		),
		mcp.WithString("extras",
			mcp.Description("Additional am start flags (e.g. '--es key value --ei count 5')"),
		),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(true),
	)

	s.AddTool(tool, handleStartActivity)
}

func handleStartActivity(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	logger.Tracef(ctx, "handleStartActivity")
	defer func() { logger.Tracef(ctx, "/handleStartActivity") }()

	component, err := request.RequireString("component")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	extras := request.GetString("extras", "")

	cmd := fmt.Sprintf("am start -n %s", shellQuote(component))
	if extras != "" {
		cmd += " " + extras
	}

	out, err := shellExec(cmd)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("am start: %v", err)), nil
	}

	return mcp.NewToolResultText(out), nil
}
