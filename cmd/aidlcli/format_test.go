package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatValue_Text(t *testing.T) {
	var buf bytes.Buffer
	f := &Formatter{Mode: "text", W: &buf}

	f.Value("monkey", true)

	assert.Equal(t, "monkey: true\n", buf.String())
}

func TestFormatValue_JSON(t *testing.T) {
	var buf bytes.Buffer
	f := &Formatter{Mode: "json", W: &buf}

	f.Value("monkey", true)

	var got map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &got))
	assert.Equal(t, true, got["monkey"])
}

func TestFormatTable_Text(t *testing.T) {
	var buf bytes.Buffer
	f := &Formatter{Mode: "text", W: &buf}

	headers := []string{"Name", "Age"}
	rows := [][]string{
		{"Alice", "30"},
		{"Bob", "25"},
	}
	f.Table(headers, rows)

	out := buf.String()
	assert.Contains(t, out, "Name")
	assert.Contains(t, out, "Age")
	assert.Contains(t, out, "Alice")
	assert.Contains(t, out, "30")
	assert.Contains(t, out, "Bob")
	assert.Contains(t, out, "25")
}

func TestFormatTable_JSON(t *testing.T) {
	var buf bytes.Buffer
	f := &Formatter{Mode: "json", W: &buf}

	headers := []string{"Name", "Age"}
	rows := [][]string{
		{"Alice", "30"},
		{"Bob", "25"},
	}
	f.Table(headers, rows)

	var got []map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &got))
	require.Len(t, got, 2)
	assert.Equal(t, "Alice", got[0]["Name"])
	assert.Equal(t, "30", got[0]["Age"])
	assert.Equal(t, "Bob", got[1]["Name"])
	assert.Equal(t, "25", got[1]["Age"])
}

func TestResolveMode(t *testing.T) {
	assert.Equal(t, "json", resolveMode("json", false))
	assert.Equal(t, "json", resolveMode("json", true))
	assert.Equal(t, "text", resolveMode("text", false))
	assert.Equal(t, "text", resolveMode("text", true))
	assert.Equal(t, "text", resolveMode("auto", true))
	assert.Equal(t, "json", resolveMode("auto", false))
}

func TestFormatResult_Text(t *testing.T) {
	var buf bytes.Buffer
	f := &Formatter{Mode: "text", W: &buf}

	f.Result(map[string]any{
		"status": "ok",
	})

	out := buf.String()
	assert.Contains(t, out, "status")
	assert.Contains(t, out, "ok")
}

func TestFormatResult_JSON(t *testing.T) {
	var buf bytes.Buffer
	f := &Formatter{Mode: "json", W: &buf}

	f.Result(map[string]any{
		"status": "ok",
		"count":  42,
	})

	var got map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &got))
	assert.Equal(t, "ok", got["status"])
	assert.Equal(t, float64(42), got["count"])
}

func TestFormatError_Text(t *testing.T) {
	var buf bytes.Buffer
	f := &Formatter{Mode: "text", W: &buf}

	f.Error(assert.AnError)

	assert.Equal(t, "error: "+assert.AnError.Error()+"\n", buf.String())
}

func TestFormatError_JSON(t *testing.T) {
	var buf bytes.Buffer
	f := &Formatter{Mode: "json", W: &buf}

	f.Error(assert.AnError)

	var got map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &got))
	assert.Equal(t, assert.AnError.Error(), got["error"])
}
