//go:build linux

package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/facebookincubator/go-belt/tool/logger"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/AndroidGoLab/binder/servicemanager"
)

// ServiceEntry describes one entry in the list_services result.
type ServiceEntry struct {
	Name  string `json:"name"`
	Alive bool   `json:"alive"`
}

func (ts *ToolSet) registerListServices(s *server.MCPServer) {
	tool := mcp.NewTool("list_services",
		mcp.WithDescription(
			"List all registered Android binder services and their liveness status. "+
				"Returns a JSON array of {name, alive} objects.",
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
	)

	s.AddTool(tool, ts.handleListServices)
}

func (ts *ToolSet) handleListServices(
	ctx context.Context,
	_ mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	logger.Tracef(ctx, "handleListServices")
	defer func() { logger.Tracef(ctx, "/handleListServices") }()

	names, err := ts.sm.ListServices(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("listing services: %v", err)), nil
	}

	entries := make([]ServiceEntry, 0, len(names))
	for _, name := range names {
		alive := ts.isServiceAlive(ctx, name)
		entries = append(entries, ServiceEntry{
			Name:  string(name),
			Alive: alive,
		})
	}

	data, err := json.Marshal(entries)
	if err != nil {
		return nil, fmt.Errorf("marshaling service list: %w", err)
	}

	return mcp.NewToolResultText(string(data)), nil
}

func (ts *ToolSet) isServiceAlive(
	ctx context.Context,
	name servicemanager.ServiceName,
) bool {
	svc, err := ts.sm.CheckService(ctx, name)
	if err != nil || svc == nil {
		return false
	}
	return svc.IsAlive(ctx)
}
