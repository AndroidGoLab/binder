//go:build linux

package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/facebookincubator/go-belt/tool/logger"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// DeviceSpecs holds key device identification and version information.
type DeviceSpecs struct {
	Model        string `json:"model"`
	Manufacturer string `json:"manufacturer"`
	Brand        string `json:"brand"`
	Device       string `json:"device"`
	Product      string `json:"product"`
	SDKVersion   string `json:"sdk_version"`
	Release      string `json:"android_version"`
	BuildID      string `json:"build_id"`
	Fingerprint  string `json:"fingerprint"`
	Hardware     string `json:"hardware"`
	ABIs         string `json:"supported_abis"`
	SecurityPatch string `json:"security_patch"`
	Serial       string `json:"serial"`
}

func registerGetDeviceSpecs(s *server.MCPServer) {
	tool := mcp.NewTool("get_device_specs",
		mcp.WithDescription(
			"Get structured device specifications: model, manufacturer, "+
				"SDK version, Android version, build ID, hardware, ABIs, etc.",
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
	)

	s.AddTool(tool, handleGetDeviceSpecs)
}

func handleGetDeviceSpecs(
	ctx context.Context,
	_ mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	logger.Tracef(ctx, "handleGetDeviceSpecs")
	defer func() { logger.Tracef(ctx, "/handleGetDeviceSpecs") }()

	specs := DeviceSpecs{
		Model:         getprop("ro.build.model"),
		Manufacturer:  getprop("ro.product.manufacturer"),
		Brand:         getprop("ro.product.brand"),
		Device:        getprop("ro.product.device"),
		Product:       getprop("ro.product.name"),
		SDKVersion:    getprop("ro.build.version.sdk"),
		Release:       getprop("ro.build.version.release"),
		BuildID:       getprop("ro.build.display.id"),
		Fingerprint:   getprop("ro.build.fingerprint"),
		Hardware:      getprop("ro.hardware"),
		ABIs:          getprop("ro.product.cpu.abilist"),
		SecurityPatch: getprop("ro.build.version.security_patch"),
		Serial:        getprop("ro.serialno"),
	}

	data, err := json.Marshal(specs)
	if err != nil {
		return nil, fmt.Errorf("marshaling device specs: %w", err)
	}

	return mcp.NewToolResultText(string(data)), nil
}

// getprop reads a single Android system property.
func getprop(key string) string {
	out, err := shellExec("getprop " + shellQuote(key))
	if err != nil {
		return ""
	}
	return out
}
