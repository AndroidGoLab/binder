//go:build linux

package main

import (
	"context"
	"fmt"

	"github.com/facebookincubator/go-belt/tool/logger"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerGetSetting(s *server.MCPServer) {
	tool := mcp.NewTool("get_setting",
		mcp.WithDescription(
			"Read an Android system setting using 'settings get'. "+
				"Namespaces: system, secure, global.",
		),
		mcp.WithString("namespace",
			mcp.Required(),
			mcp.Description("Settings namespace"),
			mcp.Enum("system", "secure", "global"),
		),
		mcp.WithString("key",
			mcp.Required(),
			mcp.Description("Setting key name (e.g. screen_brightness, font_scale)"),
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
	)

	s.AddTool(tool, handleGetSetting)
}

func handleGetSetting(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	logger.Tracef(ctx, "handleGetSetting")
	defer func() { logger.Tracef(ctx, "/handleGetSetting") }()

	namespace, err := request.RequireString("namespace")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	key, err := request.RequireString("key")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	cmd := fmt.Sprintf("settings get %s %s", shellQuote(namespace), shellQuote(key))
	out, err := shellExec(cmd)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("settings get: %v", err)), nil
	}

	return mcp.NewToolResultText(out), nil
}
