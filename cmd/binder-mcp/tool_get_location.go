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

const locationManagerDescriptor = "android.location.ILocationManager"

// LocationResult holds the get_location response.
type LocationResult struct {
	Provider  string  `json:"provider"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Altitude  float64 `json:"altitude"`
	Speed     float32 `json:"speed"`
	Error     string  `json:"error,omitempty"`
}

func (ts *ToolSet) registerGetLocation(s *server.MCPServer) {
	tool := mcp.NewTool("get_location",
		mcp.WithDescription(
			"Get the last known location from the Android LocationManager. "+
				"Returns lat/lon/alt/speed. May return null when called from "+
				"a shell UID without location permissions.",
		),
		mcp.WithString("provider",
			mcp.Description("Location provider (default: 'fused'). Options: gps, network, fused, passive"),
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
	)

	s.AddTool(tool, ts.handleGetLocation)
}

func (ts *ToolSet) handleGetLocation(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	logger.Tracef(ctx, "handleGetLocation")
	defer func() { logger.Tracef(ctx, "/handleGetLocation") }()

	provider := request.GetString("provider", "fused")

	svc, err := ts.sm.CheckService(ctx, servicemanager.ServiceName("location"))
	if err != nil || svc == nil {
		return mcp.NewToolResultError("location service unavailable"), nil
	}

	code, err := svc.ResolveCode(ctx, locationManagerDescriptor, "getLastLocation")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("resolving getLastLocation: %v", err)), nil
	}

	data := parcel.New()
	defer data.Recycle()
	data.WriteInterfaceToken(locationManagerDescriptor)

	// getLastLocation(LocationRequest request, String packageName, String attributionTag)
	// Write a minimal LocationRequest parcel inline:
	//   provider string, quality=0, interval=0
	data.WriteString16(provider)
	// packageName (required by the API).
	data.WriteString16("com.android.shell")
	// attributionTag (nullable).
	data.WriteInt32(-1)

	reply, err := svc.Transact(ctx, code, 0, data)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("getLastLocation: %v", err)), nil
	}
	defer reply.Recycle()

	if err := binder.ReadStatus(reply); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("getLastLocation status: %v", err)), nil
	}

	// The reply is a nullable Location parcel. Read the null marker.
	isNull, err := reply.ReadInt32()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("reading location null marker: %v", err)), nil
	}

	if isNull == 0 {
		result := LocationResult{
			Provider: provider,
			Error:    "no location available (null returned)",
		}
		out, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(out)), nil
	}

	// Location parcel layout: provider(String16), time(int64), elapsedRealtimeNanos(int64),
	// latitude(double), longitude(double), hasAltitude(byte), altitude(double),
	// hasSpeed(byte), speed(float), ...
	// Skip provider string.
	_, _ = reply.ReadString16()
	// Skip time.
	_, _ = reply.ReadInt64()
	// Skip elapsedRealtimeNanos.
	_, _ = reply.ReadInt64()

	lat, _ := reply.ReadFloat64()
	lon, _ := reply.ReadFloat64()

	// In Android's Location Parcelable, boolean flags are written as int32.
	hasAlt, _ := reply.ReadInt32()
	var alt float64
	if hasAlt != 0 {
		alt, _ = reply.ReadFloat64()
	}

	hasSpeed, _ := reply.ReadInt32()
	var speed float32
	if hasSpeed != 0 {
		speed, _ = reply.ReadFloat32()
	}

	result := LocationResult{
		Provider:  provider,
		Latitude:  lat,
		Longitude: lon,
		Altitude:  alt,
		Speed:     speed,
	}

	out, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("marshaling location result: %w", err)
	}

	return mcp.NewToolResultText(string(out)), nil
}
