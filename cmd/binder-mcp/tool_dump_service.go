//go:build linux

package main

import (
	"context"
	"fmt"

	"github.com/facebookincubator/go-belt/tool/logger"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerDumpService(s *server.MCPServer) {
	tool := mcp.NewTool("dump_service",
		mcp.WithDescription(
			"Dump the state of an Android system service using 'dumpsys'. "+
				"Returns the full text dump. Common services: activity, window, "+
				"power, battery, wifi, connectivity, meminfo, cpuinfo, package, "+
				"notification, alarm, audio, display, input.",
		),
		mcp.WithString("service",
			mcp.Required(),
			mcp.Description("Service name (e.g. 'activity', 'battery', 'meminfo')"),
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
	)

	s.AddTool(tool, handleDumpService)
}

func handleDumpService(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	logger.Tracef(ctx, "handleDumpService")
	defer func() { logger.Tracef(ctx, "/handleDumpService") }()

	service, err := request.RequireString("service")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	cmd := fmt.Sprintf("dumpsys %s", shellQuote(service))
	out, err := shellExec(cmd)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("dumpsys: %v", err)), nil
	}

	return mcp.NewToolResultText(out), nil
}
