//go:build linux

package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/facebookincubator/go-belt/tool/logger"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/parcel"
	"github.com/AndroidGoLab/binder/servicemanager"
)

// CallMethodResult describes the call_method response.
type CallMethodResult struct {
	ReplySize int    `json:"reply_size"`
	ReplyHex  string `json:"reply_hex"`
}

func (ts *ToolSet) registerCallMethod(s *server.MCPServer) {
	tool := mcp.NewTool("call_method",
		mcp.WithDescription(
			"Call a method on a binder service using raw transaction. "+
				"Accepts a service name and either a numeric transaction code "+
				"or a method name (resolved via the AIDL descriptor). "+
				"Optional hex-encoded parcel data can be provided as input. "+
				"Returns the reply parcel as hex.",
		),
		mcp.WithString("service",
			mcp.Required(),
			mcp.Description("Service name (e.g. 'activity', 'power')"),
		),
		mcp.WithString("method",
			mcp.Required(),
			mcp.Description(
				"Method to call: a numeric transaction code (decimal or 0x hex) "+
					"or an AIDL method name (e.g. 'isInteractive')",
			),
		),
		mcp.WithString("data",
			mcp.Description("Hex-encoded parcel data to send (empty if omitted)"),
		),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
	)

	s.AddTool(tool, ts.handleCallMethod)
}

func (ts *ToolSet) handleCallMethod(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	logger.Tracef(ctx, "handleCallMethod")
	defer func() { logger.Tracef(ctx, "/handleCallMethod") }()

	serviceName, err := request.RequireString("service")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	methodStr, err := request.RequireString("method")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	hexData := request.GetString("data", "")

	svc, err := ts.sm.CheckService(ctx, servicemanager.ServiceName(serviceName))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("checking service %q: %v", serviceName, err)), nil
	}
	if svc == nil {
		return mcp.NewToolResultError(fmt.Sprintf("service %q not found", serviceName)), nil
	}

	code, err := resolveTransactionCode(ctx, svc, methodStr)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("resolving method %q: %v", methodStr, err)), nil
	}

	var data *parcel.Parcel
	switch hexData {
	case "":
		data = parcel.New()
	default:
		raw, decErr := hex.DecodeString(hexData)
		if decErr != nil {
			return mcp.NewToolResultError(fmt.Sprintf("decoding hex data: %v", decErr)), nil
		}
		data = parcel.FromBytes(raw)
	}
	defer data.Recycle()

	reply, err := svc.Transact(ctx, code, 0, data)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("transact failed: %v", err)), nil
	}
	defer reply.Recycle()

	replyData := reply.Data()
	result := CallMethodResult{
		ReplySize: len(replyData),
		ReplyHex:  hex.EncodeToString(replyData),
	}

	out, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("marshaling call result: %w", err)
	}

	return mcp.NewToolResultText(string(out)), nil
}

// resolveTransactionCode converts a method string to a TransactionCode.
// It first tries to parse as a numeric code, then falls back to
// AIDL descriptor-based resolution via ResolveCode.
func resolveTransactionCode(
	ctx context.Context,
	svc binder.IBinder,
	method string,
) (binder.TransactionCode, error) {
	// Try numeric parse first (supports decimal and 0x prefix).
	n, err := strconv.ParseUint(method, 0, 32)
	if err == nil {
		return binder.TransactionCode(n), nil
	}

	// Fall back to descriptor-based resolution.
	descriptor := queryDescriptor(ctx, svc)
	if descriptor == "" || descriptor == "(unknown)" {
		return 0, fmt.Errorf(
			"cannot resolve method name %q: service descriptor unknown; use a numeric code instead",
			method,
		)
	}

	return svc.ResolveCode(ctx, descriptor, method)
}
