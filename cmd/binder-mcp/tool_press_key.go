//go:build linux

package main

import (
	"context"
	"fmt"

	"github.com/facebookincubator/go-belt/tool/logger"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerPressKey(s *server.MCPServer) {
	tool := mcp.NewTool("press_key",
		mcp.WithDescription(
			"Press a key on the device using 'input keyevent'. "+
				"Accepts Android keycode names (e.g. HOME, BACK, ENTER, "+
				"VOLUME_UP, VOLUME_DOWN, POWER, MENU, DPAD_UP, DPAD_DOWN, "+
				"DPAD_LEFT, DPAD_RIGHT, DPAD_CENTER, TAB, DEL) or numeric "+
				"keycode values. Prefix KEYCODE_ is added automatically if missing.",
		),
		mcp.WithString("keycode",
			mcp.Required(),
			mcp.Description("Android keycode name (e.g. HOME, BACK, ENTER) or numeric value"),
		),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(false),
	)

	s.AddTool(tool, handlePressKey)
}

func handlePressKey(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	logger.Tracef(ctx, "handlePressKey")
	defer func() { logger.Tracef(ctx, "/handlePressKey") }()

	keycode, err := request.RequireString("keycode")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	cmd := fmt.Sprintf("input keyevent %s", shellQuote(keycode))
	out, err := shellExec(cmd)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("input keyevent: %v", err)), nil
	}

	if out == "" {
		out = fmt.Sprintf("pressed key %s", keycode)
	}

	return mcp.NewToolResultText(out), nil
}
