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

// DeviceInfoResult aggregates high-level device state from multiple services.
type DeviceInfoResult struct {
	Power   PowerInfo   `json:"power"`
	Display DisplayInfo `json:"display"`
}

// PowerInfo holds power manager state.
type PowerInfo struct {
	Interactive bool   `json:"interactive"`
	Error       string `json:"error,omitempty"`
}

// DisplayInfo holds basic display state.
type DisplayInfo struct {
	Brightness float32 `json:"brightness"`
	Error      string  `json:"error,omitempty"`
}

func (ts *ToolSet) registerGetDeviceInfo(s *server.MCPServer) {
	tool := mcp.NewTool("get_device_info",
		mcp.WithDescription(
			"Get high-level device information by querying multiple binder "+
				"services (power, display). Returns a JSON object with each "+
				"subsystem's state. Errors for individual subsystems are "+
				"reported inline rather than failing the whole call.",
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
	)

	s.AddTool(tool, ts.handleGetDeviceInfo)
}

func (ts *ToolSet) handleGetDeviceInfo(
	ctx context.Context,
	_ mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	logger.Tracef(ctx, "handleGetDeviceInfo")
	defer func() { logger.Tracef(ctx, "/handleGetDeviceInfo") }()

	result := DeviceInfoResult{
		Power:   ts.queryPowerInfo(ctx),
		Display: ts.queryDisplayInfo(ctx),
	}

	data, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("marshaling device info: %w", err)
	}

	return mcp.NewToolResultText(string(data)), nil
}

const (
	powerManagerDescriptor   = "android.os.IPowerManager"
	displayManagerDescriptor = "android.hardware.display.IDisplayManager"
)

func (ts *ToolSet) queryPowerInfo(ctx context.Context) PowerInfo {
	svc, err := ts.sm.CheckService(ctx, servicemanager.ServiceName("power"))
	if err != nil || svc == nil {
		return PowerInfo{Error: "power service unavailable"}
	}

	code, err := svc.ResolveCode(ctx, powerManagerDescriptor, "isInteractive")
	if err != nil {
		return PowerInfo{Error: fmt.Sprintf("resolving isInteractive: %v", err)}
	}

	interactive, err := transactBool(ctx, svc, powerManagerDescriptor, code)
	if err != nil {
		return PowerInfo{Error: fmt.Sprintf("isInteractive: %v", err)}
	}

	return PowerInfo{Interactive: interactive}
}

func (ts *ToolSet) queryDisplayInfo(ctx context.Context) DisplayInfo {
	svc, err := ts.sm.CheckService(ctx, servicemanager.ServiceName("display"))
	if err != nil || svc == nil {
		return DisplayInfo{Error: "display service unavailable"}
	}

	code, err := svc.ResolveCode(ctx, displayManagerDescriptor, "getBrightness")
	if err != nil {
		// getBrightness may not be available on all API levels.
		return DisplayInfo{Error: fmt.Sprintf("resolving getBrightness: %v", err)}
	}

	data := parcel.New()
	defer data.Recycle()
	data.WriteInterfaceToken(displayManagerDescriptor)
	// getBrightness(int displayId) -- use display 0 (default).
	data.WriteInt32(0)

	reply, err := svc.Transact(ctx, code, 0, data)
	if err != nil {
		return DisplayInfo{Error: fmt.Sprintf("getBrightness: %v", err)}
	}
	defer reply.Recycle()

	if err := binder.ReadStatus(reply); err != nil {
		return DisplayInfo{Error: fmt.Sprintf("getBrightness status: %v", err)}
	}

	brightness, err := reply.ReadFloat32()
	if err != nil {
		return DisplayInfo{Error: fmt.Sprintf("reading brightness: %v", err)}
	}

	return DisplayInfo{Brightness: brightness}
}

// transactBool sends a simple transaction that returns a boolean (int32 0/1)
// after the status field. It writes the interface token automatically.
func transactBool(
	ctx context.Context,
	svc binder.IBinder,
	descriptor string,
	code binder.TransactionCode,
) (bool, error) {
	data := parcel.New()
	defer data.Recycle()
	data.WriteInterfaceToken(descriptor)

	reply, err := svc.Transact(ctx, code, 0, data)
	if err != nil {
		return false, err
	}
	defer reply.Recycle()

	if err := binder.ReadStatus(reply); err != nil {
		return false, err
	}

	val, err := reply.ReadInt32()
	if err != nil {
		return false, err
	}

	return val != 0, nil
}
