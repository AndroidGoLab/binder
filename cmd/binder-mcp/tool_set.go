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

// RegisterShellTools adds all shell-based MCP tools to the given server.
// These tools use direct shell command execution and do not require a binder
// connection. They are available in both device and remote modes.
func RegisterShellTools(s *server.MCPServer) {
	registerGetDeviceProperties(s)
	registerGetDeviceSpecs(s)
	registerTakeScreenshot(s)
	registerTap(s)
	registerLongPress(s)
	registerSwipe(s)
	registerInputText(s)
	registerPressKey(s)
	registerDumpUIHierarchy(s)
	registerFindUIElement(s)
	registerClickUIElement(s)
	registerGetFocusedWindow(s)
	registerInstallApp(s)
	registerUninstallApp(s)
	registerGetSetting(s)
	registerSetSetting(s)
	registerGetLogcat(s)
	registerShellExec(s)
	registerDumpService(s)
	registerOpenURL(s)
	registerStartActivity(s)
}
