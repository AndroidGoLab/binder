//go:build linux

package main

import (
	"context"
	"fmt"

	"github.com/facebookincubator/go-belt/tool/logger"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerOpenURL(s *server.MCPServer) {
	tool := mcp.NewTool("open_url",
		mcp.WithDescription(
			"Open a URL in the default browser or handling app using "+
				"'am start' with ACTION_VIEW.",
		),
		mcp.WithString("url",
			mcp.Required(),
			mcp.Description("URL to open (e.g. https://example.com)"),
		),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(true),
	)

	s.AddTool(tool, handleOpenURL)
}

func handleOpenURL(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	logger.Tracef(ctx, "handleOpenURL")
	defer func() { logger.Tracef(ctx, "/handleOpenURL") }()

	url, err := request.RequireString("url")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	cmd := fmt.Sprintf("am start -a android.intent.action.VIEW -d %s", shellQuote(url))
	out, err := shellExec(cmd)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("am start: %v", err)), nil
	}

	return mcp.NewToolResultText(out), nil
}
