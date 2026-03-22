//go:build linux

package main

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"

	"github.com/facebookincubator/go-belt/tool/logger"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// UIElement represents a matched UI element from the hierarchy.
type UIElement struct {
	Text        string `json:"text,omitempty"`
	ResourceID  string `json:"resource_id,omitempty"`
	Class       string `json:"class,omitempty"`
	ContentDesc string `json:"content_desc,omitempty"`
	Bounds      string `json:"bounds"`
	CenterX     int    `json:"center_x"`
	CenterY     int    `json:"center_y"`
	Clickable   bool   `json:"clickable"`
	Enabled     bool   `json:"enabled"`
}

func registerFindUIElement(s *server.MCPServer) {
	tool := mcp.NewTool("find_ui_element",
		mcp.WithDescription(
			"Search the UI hierarchy for elements matching the given criteria. "+
				"Dumps the UI via uiautomator and parses the XML to find "+
				"elements by text, resource-id, content-desc, or class. "+
				"Returns matching elements with their bounds and center coordinates.",
		),
		mcp.WithString("text",
			mcp.Description("Match elements whose text contains this substring (case-insensitive)"),
		),
		mcp.WithString("resource_id",
			mcp.Description("Match elements whose resource-id contains this substring"),
		),
		mcp.WithString("content_desc",
			mcp.Description("Match elements whose content-desc contains this substring (case-insensitive)"),
		),
		mcp.WithString("class",
			mcp.Description("Match elements whose class contains this substring"),
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
	)

	s.AddTool(tool, handleFindUIElement)
}

func handleFindUIElement(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	logger.Tracef(ctx, "handleFindUIElement")
	defer func() { logger.Tracef(ctx, "/handleFindUIElement") }()

	textFilter := request.GetString("text", "")
	resourceIDFilter := request.GetString("resource_id", "")
	contentDescFilter := request.GetString("content_desc", "")
	classFilter := request.GetString("class", "")

	if textFilter == "" && resourceIDFilter == "" && contentDescFilter == "" && classFilter == "" {
		return mcp.NewToolResultError("at least one search filter is required (text, resource_id, content_desc, or class)"), nil
	}

	xmlData, err := dumpUIXML()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("dumping UI: %v", err)), nil
	}

	elements := parseAndFilterUI(xmlData, textFilter, resourceIDFilter, contentDescFilter, classFilter)

	data, err := json.Marshal(elements)
	if err != nil {
		return nil, fmt.Errorf("marshaling UI elements: %w", err)
	}

	return mcp.NewToolResultText(string(data)), nil
}

// dumpUIXML runs uiautomator dump and returns the XML content.
func dumpUIXML() (string, error) {
	cmd := fmt.Sprintf("uiautomator dump %s >/dev/null 2>&1 && cat %s",
		shellQuote(uiDumpPath), shellQuote(uiDumpPath))
	return shellExec(cmd)
}

// xmlNode represents a node element in the uiautomator XML.
type xmlNode struct {
	XMLName     xml.Name  `xml:"node"`
	Text        string    `xml:"text,attr"`
	ResourceID  string    `xml:"resource-id,attr"`
	Class       string    `xml:"class,attr"`
	ContentDesc string    `xml:"content-desc,attr"`
	Bounds      string    `xml:"bounds,attr"`
	Clickable   string    `xml:"clickable,attr"`
	Enabled     string    `xml:"enabled,attr"`
	Children    []xmlNode `xml:"node"`
}

// xmlHierarchy is the root of the uiautomator XML.
type xmlHierarchy struct {
	XMLName  xml.Name  `xml:"hierarchy"`
	Children []xmlNode `xml:"node"`
}

// parseAndFilterUI parses the UI XML and filters nodes by the given criteria.
func parseAndFilterUI(
	xmlData string,
	textFilter string,
	resourceIDFilter string,
	contentDescFilter string,
	classFilter string,
) []UIElement {
	var hierarchy xmlHierarchy
	if err := xml.Unmarshal([]byte(xmlData), &hierarchy); err != nil {
		return nil
	}

	var results []UIElement
	for i := range hierarchy.Children {
		collectMatchingNodes(
			&hierarchy.Children[i],
			textFilter, resourceIDFilter, contentDescFilter, classFilter,
			&results,
		)
	}
	return results
}

func collectMatchingNodes(
	node *xmlNode,
	textFilter string,
	resourceIDFilter string,
	contentDescFilter string,
	classFilter string,
	results *[]UIElement,
) {
	if matchesFilters(node, textFilter, resourceIDFilter, contentDescFilter, classFilter) {
		cx, cy := parseBoundsCenter(node.Bounds)
		*results = append(*results, UIElement{
			Text:        node.Text,
			ResourceID:  node.ResourceID,
			Class:       node.Class,
			ContentDesc: node.ContentDesc,
			Bounds:      node.Bounds,
			CenterX:     cx,
			CenterY:     cy,
			Clickable:   node.Clickable == "true",
			Enabled:     node.Enabled == "true",
		})
	}

	for i := range node.Children {
		collectMatchingNodes(
			&node.Children[i],
			textFilter, resourceIDFilter, contentDescFilter, classFilter,
			results,
		)
	}
}

func matchesFilters(
	node *xmlNode,
	textFilter string,
	resourceIDFilter string,
	contentDescFilter string,
	classFilter string,
) bool {
	if textFilter != "" && !containsIgnoreCase(node.Text, textFilter) {
		return false
	}
	if resourceIDFilter != "" && !strings.Contains(node.ResourceID, resourceIDFilter) {
		return false
	}
	if contentDescFilter != "" && !containsIgnoreCase(node.ContentDesc, contentDescFilter) {
		return false
	}
	if classFilter != "" && !strings.Contains(node.Class, classFilter) {
		return false
	}
	return true
}

func containsIgnoreCase(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

// parseBoundsCenter parses Android bounds format "[x1,y1][x2,y2]" and
// returns the center coordinates.
func parseBoundsCenter(bounds string) (int, int) {
	// Format: [x1,y1][x2,y2]
	parts := strings.FieldsFunc(bounds, func(r rune) bool {
		return r == '[' || r == ']' || r == ','
	})
	if len(parts) < 4 {
		return 0, 0
	}

	x1, _ := strconv.Atoi(parts[0])
	y1, _ := strconv.Atoi(parts[1])
	x2, _ := strconv.Atoi(parts[2])
	y2, _ := strconv.Atoi(parts[3])

	return (x1 + x2) / 2, (y1 + y2) / 2
}
