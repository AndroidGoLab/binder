//go:build linux

package main

import (
	"context"
	"fmt"

	"github.com/facebookincubator/go-belt/tool/logger"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerUninstallApp(s *server.MCPServer) {
	tool := mcp.NewTool("uninstall_app",
		mcp.WithDescription(
			"Uninstall an application by package name using 'pm uninstall'.",
		),
		mcp.WithString("package",
			mcp.Required(),
			mcp.Description("Package name to uninstall (e.g. com.example.app)"),
		),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(true),
	)

	s.AddTool(tool, handleUninstallApp)
}

func handleUninstallApp(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	logger.Tracef(ctx, "handleUninstallApp")
	defer func() { logger.Tracef(ctx, "/handleUninstallApp") }()

	pkg, err := request.RequireString("package")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	cmd := fmt.Sprintf("pm uninstall %s", shellQuote(pkg))
	out, err := shellExec(cmd)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("pm uninstall: %v", err)), nil
	}

	return mcp.NewToolResultText(out), nil
}
