//go:build linux

package main

import (
	"context"
	"fmt"

	"github.com/facebookincubator/go-belt/tool/logger"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerInstallApp(s *server.MCPServer) {
	tool := mcp.NewTool("install_app",
		mcp.WithDescription(
			"Install an APK from the device filesystem using 'pm install -r'. "+
				"The -r flag allows reinstallation over existing installs.",
		),
		mcp.WithString("path",
			mcp.Required(),
			mcp.Description("Absolute path to the APK file on the device (e.g. /data/local/tmp/app.apk)"),
		),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(true),
	)

	s.AddTool(tool, handleInstallApp)
}

func handleInstallApp(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	logger.Tracef(ctx, "handleInstallApp")
	defer func() { logger.Tracef(ctx, "/handleInstallApp") }()

	path, err := request.RequireString("path")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	cmd := fmt.Sprintf("pm install -r %s", shellQuote(path))
	out, err := shellExec(cmd)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("pm install: %v", err)), nil
	}

	return mcp.NewToolResultText(out), nil
}
