//go:build linux

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/facebookincubator/go-belt/tool/logger"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// FocusedWindowResult holds the parsed current focus information.
type FocusedWindowResult struct {
	Package  string `json:"package"`
	Activity string `json:"activity"`
	Raw      string `json:"raw"`
}

func registerGetFocusedWindow(s *server.MCPServer) {
	tool := mcp.NewTool("get_focused_window",
		mcp.WithDescription(
			"Get the currently focused window's package and activity name "+
				"by parsing 'dumpsys window' output.",
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
	)

	s.AddTool(tool, handleGetFocusedWindow)
}

func handleGetFocusedWindow(
	ctx context.Context,
	_ mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	logger.Tracef(ctx, "handleGetFocusedWindow")
	defer func() { logger.Tracef(ctx, "/handleGetFocusedWindow") }()

	out, err := shellExec("dumpsys window | grep -E 'mCurrentFocus|mFocusedApp'")
	if err != nil {
		// grep returns exit code 1 if no match; still report the output.
		if out == "" {
			return mcp.NewToolResultError(fmt.Sprintf("dumpsys window: %v", err)), nil
		}
	}

	result := parseFocusedWindow(out)

	data, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("marshaling focused window: %w", err)
	}

	return mcp.NewToolResultText(string(data)), nil
}

// parseFocusedWindow extracts package/activity from dumpsys window lines
// like: "mCurrentFocus=Window{...u0 com.example/.MainActivity}"
func parseFocusedWindow(output string) FocusedWindowResult {
	result := FocusedWindowResult{Raw: output}

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, "mCurrentFocus") {
			continue
		}

		// Find the component after "u0 " (or similar user prefix).
		idx := strings.LastIndex(line, " ")
		if idx < 0 {
			continue
		}

		component := strings.TrimSuffix(line[idx+1:], "}")

		parts := strings.SplitN(component, "/", 2)
		if len(parts) == 2 {
			result.Package = parts[0]
			result.Activity = parts[1]
		} else {
			result.Package = component
		}
		break
	}

	return result
}
