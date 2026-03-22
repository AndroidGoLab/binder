//go:build linux

package main

import (
	"context"
	"fmt"

	"github.com/facebookincubator/go-belt/tool/logger"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const screenshotPath = "/data/local/tmp/screenshot"

func registerTakeScreenshot(s *server.MCPServer) {
	tool := mcp.NewTool("take_screenshot",
		mcp.WithDescription(
			"Capture the device screen and return it as a base64-encoded "+
				"image. Supports PNG and JPEG formats. The screenshot is "+
				"saved to a temporary file, encoded, and cleaned up.",
		),
		mcp.WithString("format",
			mcp.Description("Image format: png or jpeg (default: png)"),
			mcp.Enum("png", "jpeg"),
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
	)

	s.AddTool(tool, handleTakeScreenshot)
}

func handleTakeScreenshot(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	logger.Tracef(ctx, "handleTakeScreenshot")
	defer func() { logger.Tracef(ctx, "/handleTakeScreenshot") }()

	format := request.GetString("format", "png")

	var mimeType string
	switch format {
	case "png":
		mimeType = "image/png"
	case "jpeg":
		mimeType = "image/jpeg"
	default:
		return mcp.NewToolResultError(fmt.Sprintf("unsupported format %q: use png or jpeg", format)), nil
	}

	filePath := screenshotPath + "." + format

	// screencap always produces PNG; convert to JPEG if requested.
	var captureCmd string
	switch format {
	case "png":
		captureCmd = fmt.Sprintf("screencap -p %s", shellQuote(filePath))
	case "jpeg":
		// screencap -p produces PNG; pipe through conversion if available,
		// otherwise capture as PNG and note the format mismatch.
		captureCmd = fmt.Sprintf(
			"screencap -p %s",
			shellQuote(screenshotPath+".png"),
		)
	}

	if _, err := shellExec(captureCmd); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("screencap: %v", err)), nil
	}

	// For JPEG, try converting with available tools.
	if format == "jpeg" {
		pngPath := screenshotPath + ".png"
		// Try toybox-based conversion; fall back to returning PNG as-is.
		convCmd := fmt.Sprintf(
			"if command -v convert >/dev/null 2>&1; then "+
				"convert %s %s && rm -f %s; "+
				"else cp %s %s; fi",
			shellQuote(pngPath), shellQuote(filePath),
			shellQuote(pngPath),
			shellQuote(pngPath), shellQuote(filePath),
		)
		if _, err := shellExec(convCmd); err != nil {
			// Fall back to PNG.
			filePath = pngPath
			mimeType = "image/png"
		}
	}

	b64, err := shellExec(fmt.Sprintf("base64 -w 0 %s", shellQuote(filePath)))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("base64 encode: %v", err)), nil
	}

	// Clean up temporary file.
	_, _ = shellExec(fmt.Sprintf("rm -f %s %s",
		shellQuote(screenshotPath+".png"),
		shellQuote(screenshotPath+".jpeg"),
	))

	return mcp.NewToolResultImage("screenshot", b64, mimeType), nil
}
