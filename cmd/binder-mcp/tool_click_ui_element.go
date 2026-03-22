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

// ClickUIResult describes the click_ui_element response.
type ClickUIResult struct {
	MatchedText        string `json:"matched_text,omitempty"`
	MatchedResourceID  string `json:"matched_resource_id,omitempty"`
	MatchedContentDesc string `json:"matched_content_desc,omitempty"`
	TappedX            int    `json:"tapped_x"`
	TappedY            int    `json:"tapped_y"`
}

func registerClickUIElement(s *server.MCPServer) {
	tool := mcp.NewTool("click_ui_element",
		mcp.WithDescription(
			"Find a UI element by text, resource-id, or content-desc, "+
				"compute the center of its bounds, and tap it. "+
				"This is a convenience tool that combines find_ui_element + tap. "+
				"Returns an error if no matching element is found or if multiple "+
				"elements match (use more specific criteria).",
		),
		mcp.WithString("text",
			mcp.Description("Match element whose text contains this substring (case-insensitive)"),
		),
		mcp.WithString("resource_id",
			mcp.Description("Match element whose resource-id contains this substring"),
		),
		mcp.WithString("content_desc",
			mcp.Description("Match element whose content-desc contains this substring (case-insensitive)"),
		),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(false),
	)

	s.AddTool(tool, handleClickUIElement)
}

func handleClickUIElement(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	logger.Tracef(ctx, "handleClickUIElement")
	defer func() { logger.Tracef(ctx, "/handleClickUIElement") }()

	textFilter := request.GetString("text", "")
	resourceIDFilter := request.GetString("resource_id", "")
	contentDescFilter := request.GetString("content_desc", "")

	if textFilter == "" && resourceIDFilter == "" && contentDescFilter == "" {
		return mcp.NewToolResultError("at least one search filter is required (text, resource_id, or content_desc)"), nil
	}

	xmlData, err := dumpUIXML()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("dumping UI: %v", err)), nil
	}

	elements := parseAndFilterUI(xmlData, textFilter, resourceIDFilter, contentDescFilter, "")

	switch len(elements) {
	case 0:
		return mcp.NewToolResultError("no matching UI element found"), nil
	case 1:
		// Exactly one match — tap it.
	default:
		data, _ := json.Marshal(elements)
		return mcp.NewToolResultError(fmt.Sprintf(
			"found %d matching elements (use more specific criteria): %s",
			len(elements), string(data),
		)), nil
	}

	elem := elements[0]
	cmd := fmt.Sprintf("input tap %d %d", elem.CenterX, elem.CenterY)
	if _, err := shellExec(cmd); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("input tap: %v", err)), nil
	}

	result := ClickUIResult{
		MatchedText:        elem.Text,
		MatchedResourceID:  elem.ResourceID,
		MatchedContentDesc: elem.ContentDesc,
		TappedX:            elem.CenterX,
		TappedY:            elem.CenterY,
	}

	data, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("marshaling click result: %w", err)
	}

	return mcp.NewToolResultText(string(data)), nil
}
