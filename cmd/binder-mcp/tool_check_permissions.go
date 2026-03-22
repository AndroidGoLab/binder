//go:build linux

package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/facebookincubator/go-belt/tool/logger"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/parcel"
	"github.com/AndroidGoLab/binder/servicemanager"
)

// PermissionCheckResult holds the check_permissions response.
type PermissionCheckResult struct {
	Service    string `json:"service"`
	Alive      bool   `json:"alive"`
	Descriptor string `json:"descriptor"`
	PingOK     bool   `json:"ping_ok"`
	PingError  string `json:"ping_error,omitempty"`
}

func (ts *ToolSet) registerCheckPermissions(s *server.MCPServer) {
	tool := mcp.NewTool("check_permissions",
		mcp.WithDescription(
			"Check whether the current process can access a binder service. "+
				"Attempts a ping transaction and reports the result, including "+
				"any SELinux denial details from enriched errors.",
		),
		mcp.WithString("service",
			mcp.Required(),
			mcp.Description("Service name to check (e.g. 'camera', 'location')"),
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
	)

	s.AddTool(tool, ts.handleCheckPermissions)
}

func (ts *ToolSet) handleCheckPermissions(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	logger.Tracef(ctx, "handleCheckPermissions")
	defer func() { logger.Tracef(ctx, "/handleCheckPermissions") }()

	name, err := request.RequireString("service")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	svc, err := ts.sm.CheckService(ctx, servicemanager.ServiceName(name))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("checking service %q: %v", name, err)), nil
	}
	if svc == nil {
		return mcp.NewToolResultError(fmt.Sprintf("service %q not found", name)), nil
	}

	descriptor := queryDescriptor(ctx, svc)

	result := PermissionCheckResult{
		Service:    name,
		Alive:      svc.IsAlive(ctx),
		Descriptor: descriptor,
	}

	// Attempt a ping transaction to check accessibility.
	// The enriched error from the binder layer includes SELinux context
	// when available.
	pingErr := pingService(ctx, svc)
	switch pingErr {
	case nil:
		result.PingOK = true
	default:
		result.PingError = pingErr.Error()
	}

	data, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("marshaling permission check: %w", err)
	}

	return mcp.NewToolResultText(string(data)), nil
}

// pingService sends a PING transaction to the binder service.
func pingService(
	ctx context.Context,
	svc binder.IBinder,
) error {
	data := parcel.New()
	defer data.Recycle()

	reply, err := svc.Transact(ctx, binder.PingTransaction, 0, data)
	if err != nil {
		return err
	}
	reply.Recycle()
	return nil
}
