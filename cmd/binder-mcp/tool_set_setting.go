//go:build linux

package main

import (
	"context"
	"fmt"

	"github.com/facebookincubator/go-belt/tool/logger"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerSetSetting(s *server.MCPServer) {
	tool := mcp.NewTool("set_setting",
		mcp.WithDescription(
			"Write an Android system setting using 'settings put'. "+
				"Namespaces: system, secure, global. "+
				"Requires appropriate permissions (shell UID has access to most settings).",
		),
		mcp.WithString("namespace",
			mcp.Required(),
			mcp.Description("Settings namespace"),
			mcp.Enum("system", "secure", "global"),
		),
		mcp.WithString("key",
			mcp.Required(),
			mcp.Description("Setting key name"),
		),
		mcp.WithString("value",
			mcp.Required(),
			mcp.Description("Value to set"),
		),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(true),
	)

	s.AddTool(tool, handleSetSetting)
}

func handleSetSetting(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	logger.Tracef(ctx, "handleSetSetting")
	defer func() { logger.Tracef(ctx, "/handleSetSetting") }()

	namespace, err := request.RequireString("namespace")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	key, err := request.RequireString("key")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	value, err := request.RequireString("value")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	cmd := fmt.Sprintf("settings put %s %s %s",
		shellQuote(namespace), shellQuote(key), shellQuote(value))
	out, err := shellExec(cmd)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("settings put: %v", err)), nil
	}

	if out == "" {
		out = fmt.Sprintf("set %s/%s = %s", namespace, key, value)
	}

	return mcp.NewToolResultText(out), nil
}
