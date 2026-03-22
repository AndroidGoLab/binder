//go:build linux

package main

import (
	"context"

	"github.com/mark3labs/mcp-go/server"

	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/servicemanager"
)

// ServiceLookup is the subset of servicemanager.ServiceManager used by the
// MCP tool handlers. Both the real ServiceManager (device mode) and the
// remote proxy implementation satisfy this interface.
type ServiceLookup interface {
	ListServices(ctx context.Context) ([]servicemanager.ServiceName, error)
	CheckService(ctx context.Context, name servicemanager.ServiceName) (binder.IBinder, error)
}

// ToolSet holds the binder connection state shared by all MCP tool handlers.
type ToolSet struct {
	sm ServiceLookup
}

// Register adds all binder MCP tools to the given server.
func (ts *ToolSet) Register(s *server.MCPServer) {
	ts.registerListServices(s)
	ts.registerGetServiceInfo(s)
	ts.registerCallMethod(s)
	ts.registerGetDeviceInfo(s)
	ts.registerGetLocation(s)
	ts.registerCheckPermissions(s)
}
