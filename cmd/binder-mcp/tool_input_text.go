//go:build linux

package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/facebookincubator/go-belt/tool/logger"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerInputText(s *server.MCPServer) {
	tool := mcp.NewTool("input_text",
		mcp.WithDescription(
			"Type text on the device using 'input text'. "+
				"Spaces and special characters are escaped automatically. "+
				"The device must have an active text field focused.",
		),
		mcp.WithString("text",
			mcp.Required(),
			mcp.Description("Text to type"),
		),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(false),
	)

	s.AddTool(tool, handleInputText)
}

func handleInputText(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	logger.Tracef(ctx, "handleInputText")
	defer func() { logger.Tracef(ctx, "/handleInputText") }()

	text, err := request.RequireString("text")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	// Android's 'input text' requires spaces to be escaped as %s
	// and other special shell characters to be handled.
	escaped := escapeInputText(text)

	cmd := fmt.Sprintf("input text %s", shellQuote(escaped))
	out, err := shellExec(cmd)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("input text: %v", err)), nil
	}

	if out == "" {
		out = fmt.Sprintf("typed %d characters", len(text))
	}

	return mcp.NewToolResultText(out), nil
}

// escapeInputText escapes text for Android's 'input text' command.
// Spaces must be replaced with %s, and certain characters need escaping.
func escapeInputText(text string) string {
	// Android input text uses %s for space.
	text = strings.ReplaceAll(text, " ", "%s")
	return text
}
