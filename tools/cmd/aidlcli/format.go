package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"text/tabwriter"

	"golang.org/x/term"
)

// Formatter handles text and JSON output with auto-detection of terminal vs pipe.
type Formatter struct {
	Mode string
	W    io.Writer
}

// NewFormatter creates a Formatter that resolves "auto" mode by checking
// whether stdout is a terminal.
func NewFormatter(
	mode string,
	w io.Writer,
) *Formatter {
	isTTY := term.IsTerminal(int(os.Stdout.Fd()))
	return &Formatter{
		Mode: resolveMode(mode, isTTY),
		W:    w,
	}
}

// resolveMode maps the mode string to a concrete output format.
// When mode is "auto", it returns "text" for terminals and "json" for pipes.
func resolveMode(mode string, isTTY bool) string {
	switch mode {
	case "text", "json":
		return mode
	default:
		if isTTY {
			return "text"
		}
		return "json"
	}
}

// Value writes a single key-value pair.
// Text: "key: val\n". JSON: {"key": val}.
func (f *Formatter) Value(key string, val any) {
	switch f.Mode {
	case "json":
		f.writeJSON(map[string]any{key: val})
	default:
		fmt.Fprintf(f.W, "%s: %v\n", key, val)
	}
}

// Result writes a map of key-value pairs.
// Text: sorted k/v lines. JSON: object.
func (f *Formatter) Result(m map[string]any) {
	switch f.Mode {
	case "json":
		f.writeJSON(m)
	default:
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(f.W, "%s: %v\n", k, m[k])
		}
	}
}

// Table writes tabular data.
// Text: aligned columns via tabwriter. JSON: array of objects keyed by headers.
func (f *Formatter) Table(headers []string, rows [][]string) {
	switch f.Mode {
	case "json":
		objects := make([]map[string]string, 0, len(rows))
		for _, row := range rows {
			obj := make(map[string]string, len(headers))
			for i, h := range headers {
				if i < len(row) {
					obj[h] = row[i]
				}
			}
			objects = append(objects, obj)
		}
		f.writeJSON(objects)
	default:
		tw := tabwriter.NewWriter(f.W, 0, 4, 2, ' ', 0)
		for i, h := range headers {
			if i > 0 {
				fmt.Fprint(tw, "\t")
			}
			fmt.Fprint(tw, h)
		}
		fmt.Fprintln(tw)
		for _, row := range rows {
			for i, cell := range row {
				if i > 0 {
					fmt.Fprint(tw, "\t")
				}
				fmt.Fprint(tw, cell)
			}
			fmt.Fprintln(tw)
		}
		tw.Flush()
	}
}

// Error writes an error message.
// Text: "error: msg\n". JSON: {"error": "msg"}.
func (f *Formatter) Error(err error) {
	switch f.Mode {
	case "json":
		f.writeJSON(map[string]string{"error": err.Error()})
	default:
		fmt.Fprintf(f.W, "error: %s\n", err.Error())
	}
}

func (f *Formatter) writeJSON(v any) {
	enc := json.NewEncoder(f.W)
	enc.SetEscapeHTML(false)
	enc.Encode(v)
}
