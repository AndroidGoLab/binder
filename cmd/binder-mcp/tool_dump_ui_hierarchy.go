//go:build linux

package main

import (
	"context"
	"fmt"

	"github.com/facebookincubator/go-belt/tool/logger"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const uiDumpPath = "/data/local/tmp/ui.xml"

func registerDumpUIHierarchy(s *server.MCPServer) {
	tool := mcp.NewTool("dump_ui_hierarchy",
		mcp.WithDescription(
			"Dump the current UI hierarchy as XML using 'uiautomator dump'. "+
				"Returns the full XML tree of all visible UI elements with their "+
				"properties (text, resource-id, class, bounds, etc.). "+
				"This operation can take 2-3 seconds.",
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
	)

	s.AddTool(tool, handleDumpUIHierarchy)
}

func handleDumpUIHierarchy(
	ctx context.Context,
	_ mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	logger.Tracef(ctx, "handleDumpUIHierarchy")
	defer func() { logger.Tracef(ctx, "/handleDumpUIHierarchy") }()

	cmd := fmt.Sprintf("uiautomator dump %s && cat %s",
		shellQuote(uiDumpPath), shellQuote(uiDumpPath))

	out, err := shellExec(cmd)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("uiautomator dump: %v", err)), nil
	}

	// uiautomator dump prints a status line before the XML in some versions.
	// The XML always starts with "<?xml" — strip any prefix.
	xml := extractXML(out)

	return mcp.NewToolResultText(xml), nil
}

// extractXML strips any non-XML prefix from uiautomator output.
// The dump command sometimes prints "UI hierchary dumped to: /path"
// before the XML when using 'dump && cat'.
func extractXML(output string) string {
	for i := 0; i < len(output); i++ {
		if output[i] == '<' {
			return output[i:]
		}
	}
	return output
}
