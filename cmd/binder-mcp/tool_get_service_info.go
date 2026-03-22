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

// ServiceInfoResult describes the get_service_info response.
type ServiceInfoResult struct {
	Name       string       `json:"name"`
	Handle     uint32       `json:"handle"`
	Alive      bool         `json:"alive"`
	Descriptor string       `json:"descriptor"`
	Methods    []MethodDesc `json:"methods,omitempty"`
}

// MethodDesc describes a single method on a binder interface.
type MethodDesc struct {
	Name       string      `json:"name"`
	Params     []ParamDesc `json:"params,omitempty"`
	ReturnType string      `json:"return_type,omitempty"`
}

// ParamDesc describes one parameter of a binder method.
type ParamDesc struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

func (ts *ToolSet) registerGetServiceInfo(s *server.MCPServer) {
	tool := mcp.NewTool("get_service_info",
		mcp.WithDescription(
			"Get detailed information about a binder service: "+
				"AIDL descriptor, handle, liveness, and available methods.",
		),
		mcp.WithString("service",
			mcp.Required(),
			mcp.Description("Service name as registered with ServiceManager (e.g. 'activity', 'power')"),
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
	)

	s.AddTool(tool, ts.handleGetServiceInfo)
}

func (ts *ToolSet) handleGetServiceInfo(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	logger.Tracef(ctx, "handleGetServiceInfo")
	defer func() { logger.Tracef(ctx, "/handleGetServiceInfo") }()

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

	result := ServiceInfoResult{
		Name:       name,
		Handle:     svc.Handle(),
		Alive:      svc.IsAlive(ctx),
		Descriptor: descriptor,
	}

	data, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("marshaling service info: %w", err)
	}

	return mcp.NewToolResultText(string(data)), nil
}

// queryDescriptor sends an InterfaceTransaction to retrieve the AIDL
// interface descriptor string from a binder service.
func queryDescriptor(
	ctx context.Context,
	svc binder.IBinder,
) string {
	p := parcel.New()
	defer p.Recycle()

	reply, err := svc.Transact(ctx, binder.InterfaceTransaction, 0, p)
	if err != nil {
		return "(unknown)"
	}
	defer reply.Recycle()

	desc, err := reply.ReadString16()
	if err != nil {
		return "(unknown)"
	}

	return desc
}
