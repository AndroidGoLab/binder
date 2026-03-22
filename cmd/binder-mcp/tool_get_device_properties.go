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

func registerGetDeviceProperties(s *server.MCPServer) {
	tool := mcp.NewTool("get_device_properties",
		mcp.WithDescription(
			"Get all Android system properties (getprop) as a JSON object "+
				"of key-value pairs. Returns properties like ro.build.model, "+
				"ro.build.version.sdk, ro.product.manufacturer, etc.",
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
	)

	s.AddTool(tool, handleGetDeviceProperties)
}

func handleGetDeviceProperties(
	ctx context.Context,
	_ mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	logger.Tracef(ctx, "handleGetDeviceProperties")
	defer func() { logger.Tracef(ctx, "/handleGetDeviceProperties") }()

	out, err := shellExec("getprop")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("getprop: %v", err)), nil
	}

	props := parseGetpropOutput(out)

	data, err := json.Marshal(props)
	if err != nil {
		return nil, fmt.Errorf("marshaling properties: %w", err)
	}

	return mcp.NewToolResultText(string(data)), nil
}

// parseGetpropOutput parses `getprop` output lines of the form
// "[key]: [value]" into a map.
func parseGetpropOutput(output string) map[string]string {
	props := make(map[string]string)
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Format: [key]: [value]
		colonIdx := strings.Index(line, "]: [")
		if colonIdx < 0 {
			continue
		}

		if !strings.HasPrefix(line, "[") || !strings.HasSuffix(line, "]") {
			continue
		}

		key := line[1:colonIdx]
		value := line[colonIdx+4 : len(line)-1]
		props[key] = value
	}
	return props
}
