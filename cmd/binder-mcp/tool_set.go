//go:build linux

package main

import (
	"github.com/mark3labs/mcp-go/server"

	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/servicemanager"
)

// ToolSet holds the binder connection state shared by all MCP tool handlers.
type ToolSet struct {
	sm        *servicemanager.ServiceManager
	transport binder.VersionAwareTransport
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
