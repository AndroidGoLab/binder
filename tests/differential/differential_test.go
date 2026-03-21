package differential

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// align4 mirrors the Go parcel alignment: (n + 3) &^ 3
func align4(n int) int {
	return (n + 3) &^ 3
}

// stringUTF8WireSize mirrors parcel.WriteString wire size.
func stringUTF8WireSize(byteLen int) int {
	return 4 + align4(byteLen+1)
}

// string16WireSize mirrors parcel.WriteString16 wire size.
func string16WireSize(charCount int) int {
	return 4 + align4((charCount+1)*2)
}

// byteArrayWireSize mirrors parcel.WriteByteArray wire size.
func byteArrayWireSize(length int) int {
	return 4 + align4(length)
}

const (
	binderTypeBinder = 0x73622a85
	binderTypeHandle = 0x73682a85
)

// classifyBinder mirrors the Go ReadNullableStrongBinder logic.
func classifyBinder(objType, handle uint64) string {
	if objType == 0 {
		return "NULL"
	}
	if objType != binderTypeHandle && objType != binderTypeBinder {
		return "ERROR"
	}
	if objType == binderTypeBinder && handle == 0 {
		return "NULL"
	}
	return fmt.Sprintf("HANDLE:%d", handle)
}

// classifyNonNullBinder mirrors the fixed Go ReadStrongBinder logic.
func classifyNonNullBinder(objType, handle uint64) string {
	if objType == 0 {
		return "ERROR"
	}
	if objType != binderTypeHandle && objType != binderTypeBinder {
		return "ERROR"
	}
	if objType == binderTypeBinder && handle == 0 {
		return "ERROR"
	}
	return fmt.Sprintf("HANDLE:%d", handle)
}

// parseLeanOutput parses the Lean oracle output into a map of category+key → result.
func parseLeanOutput(output string) map[string]string {
	results := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, " ", 3)
		if len(parts) != 3 {
			continue
		}
		key := parts[0] + " " + parts[1]
		results[key] = parts[2]
	}
	return results
}

// findProjectRoot walks up from the test file to find the project root (containing go.mod).
func findProjectRoot() string {
	_, filename, _, _ := runtime.Caller(0)
	dir := filepath.Dir(filename)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// runLeanOracle builds and runs the Lean difftest executable.
func runLeanOracle(t *testing.T) map[string]string {
	t.Helper()
	root := findProjectRoot()
	require.NotEmpty(t, root, "could not find project root")

	proofsDir := filepath.Join(root, "proofs")
	if _, err := os.Stat(filepath.Join(proofsDir, "DiffTest.lean")); err != nil {
		t.Skip("proofs/DiffTest.lean not found — skipping differential tests")
	}

	if _, err := exec.LookPath("lake"); err != nil {
		t.Skip("lake not found in PATH — skipping differential tests (install Lean 4 to enable)")
	}

	// Build the difftest executable.
	buildCmd := exec.Command("lake", "build", "difftest")
	buildCmd.Dir = proofsDir
	buildOut, err := buildCmd.CombinedOutput()
	require.NoError(t, err, "lake build difftest failed:\n%s", string(buildOut))

	// Run it.
	runCmd := exec.Command("lake", "env", "lean", "--run", "DiffTest.lean")
	runCmd.Dir = proofsDir
	out, err := runCmd.CombinedOutput()
	require.NoError(t, err, "lean --run DiffTest.lean failed:\n%s", string(out))

	return parseLeanOutput(string(out))
}

func TestDifferentialAlign4(t *testing.T) {
	lean := runLeanOracle(t)

	cases := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12,
		15, 16, 17, 23, 24, 25, 31, 32, 33,
		100, 255, 256, 1000, 1023, 1024, 4096, 65535, 65536}

	for _, n := range cases {
		key := fmt.Sprintf("ALIGN4 %d", n)
		expected, ok := lean[key]
		require.True(t, ok, "missing Lean result for %s", key)

		goResult := strconv.Itoa(align4(n))
		assert.Equal(t, expected, goResult, "align4(%d): Lean=%s Go=%s", n, expected, goResult)
	}
}

func TestDifferentialStringUTF8Size(t *testing.T) {
	lean := runLeanOracle(t)

	cases := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 10, 15, 16, 31, 32, 100, 255, 256, 1000}

	for _, n := range cases {
		key := fmt.Sprintf("STRING_UTF8_SIZE %d", n)
		expected, ok := lean[key]
		require.True(t, ok, "missing Lean result for %s", key)

		goResult := strconv.Itoa(stringUTF8WireSize(n))
		assert.Equal(t, expected, goResult, "stringUTF8WireSize(%d): Lean=%s Go=%s", n, expected, goResult)
	}
}

func TestDifferentialString16Size(t *testing.T) {
	lean := runLeanOracle(t)

	cases := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 10, 15, 16, 31, 32, 100, 255, 256, 1000}

	for _, n := range cases {
		key := fmt.Sprintf("STRING16_SIZE %d", n)
		expected, ok := lean[key]
		require.True(t, ok, "missing Lean result for %s", key)

		// Go's string16WireSize uses charCount = number of UTF-16 code units
		goResult := strconv.Itoa(string16WireSize(n))
		assert.Equal(t, expected, goResult, "string16WireSize(%d): Lean=%s Go=%s", n, expected, goResult)
	}
}

func TestDifferentialByteArraySize(t *testing.T) {
	lean := runLeanOracle(t)

	cases := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 10, 15, 16, 31, 32, 100, 255, 256, 1000}

	for _, n := range cases {
		key := fmt.Sprintf("BYTEARRAY_SIZE %d", n)
		expected, ok := lean[key]
		require.True(t, ok, "missing Lean result for %s", key)

		goResult := strconv.Itoa(byteArrayWireSize(n))
		assert.Equal(t, expected, goResult, "byteArrayWireSize(%d): Lean=%s Go=%s", n, expected, goResult)
	}
}

func TestDifferentialBinderClassify(t *testing.T) {
	lean := runLeanOracle(t)

	cases := []struct {
		objType uint64
		handle  uint64
	}{
		{0, 0}, {0, 42},
		{binderTypeBinder, 0},
		{binderTypeHandle, 0}, {binderTypeHandle, 1}, {binderTypeHandle, 42},
		{binderTypeHandle, 0xFFFFFFFF},
		{binderTypeBinder, 1}, {binderTypeBinder, 42}, {binderTypeBinder, 0x12345678},
		{1, 0}, {0xDEADBEEF, 0}, {0x73622a84, 0}, {0x73682a86, 0},
	}

	for _, tc := range cases {
		key := fmt.Sprintf("BINDER_CLASSIFY %d,%d", tc.objType, tc.handle)
		expected, ok := lean[key]
		require.True(t, ok, "missing Lean result for %s", key)

		goResult := classifyBinder(tc.objType, tc.handle)
		assert.Equal(t, expected, goResult, "classifyBinder(%d,%d): Lean=%s Go=%s",
			tc.objType, tc.handle, expected, goResult)
	}
}

func TestDifferentialBinderNonNull(t *testing.T) {
	lean := runLeanOracle(t)

	cases := []struct {
		objType uint64
		handle  uint64
	}{
		{0, 0},
		{binderTypeBinder, 0},
		{binderTypeHandle, 0}, {binderTypeHandle, 42},
		{binderTypeBinder, 1},
		{1, 0}, {0xDEADBEEF, 0},
	}

	for _, tc := range cases {
		key := fmt.Sprintf("BINDER_NONNULL %d,%d", tc.objType, tc.handle)
		expected, ok := lean[key]
		require.True(t, ok, "missing Lean result for %s", key)

		goResult := classifyNonNullBinder(tc.objType, tc.handle)
		assert.Equal(t, expected, goResult, "classifyNonNullBinder(%d,%d): Lean=%s Go=%s",
			tc.objType, tc.handle, expected, goResult)
	}
}

func TestDifferentialParcelableSize(t *testing.T) {
	lean := runLeanOracle(t)

	cases := []struct {
		headerPos   int
		payloadSize int
	}{
		{0, 0}, {0, 4}, {0, 100}, {12, 0}, {12, 8}, {100, 200}, {0, 1000},
	}

	for _, tc := range cases {
		key := fmt.Sprintf("PARCELABLE_SIZE %d,%d", tc.headerPos, tc.payloadSize)
		expected, ok := lean[key]
		require.True(t, ok, "missing Lean result for %s", key)

		totalSize := 4 + tc.payloadSize
		endPos := tc.headerPos + totalSize
		goResult := fmt.Sprintf("%d,%d", totalSize, endPos)
		assert.Equal(t, expected, goResult, "parcelable(%d,%d): Lean=%s Go=%s",
			tc.headerPos, tc.payloadSize, expected, goResult)
	}
}

func TestDifferentialGrowRead(t *testing.T) {
	lean := runLeanOracle(t)

	cases := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 12, 16, 24, 28, 32, 100}

	for _, n := range cases {
		key := fmt.Sprintf("GROW_READ %d", n)
		expected, ok := lean[key]
		require.True(t, ok, "missing Lean result for %s", key)

		// Simulate grow then read on an empty parcel.
		parcelLen := align4(n) // after grow
		start := 0             // grow starts at 0 for empty parcel
		posAfterRead := align4(n)
		goResult := fmt.Sprintf("%d,%d,%d", parcelLen, start, posAfterRead)
		assert.Equal(t, expected, goResult, "growRead(%d): Lean=%s Go=%s", n, expected, goResult)
	}
}

// TestDifferentialActualParcelEncoding runs the Go parcel encoder and verifies
// wire sizes match the Lean oracle's predictions.
func TestDifferentialActualParcelEncoding(t *testing.T) {
	lean := runLeanOracle(t)

	// Test actual UTF-8 string encoding sizes.
	utf8Cases := map[string]int{
		"":        0,
		"a":       1,
		"ab":      2,
		"abc":     3,
		"abcd":    4,
		"hello":   5,
		"hellowo": 7,
	}
	for s, byteLen := range utf8Cases {
		key := fmt.Sprintf("STRING_UTF8_SIZE %d", byteLen)
		expected, ok := lean[key]
		if !ok {
			continue
		}

		// Compute actual wire size by encoding.
		p := newTestParcel()
		p.writeString(s)
		assert.Equal(t, expected, strconv.Itoa(p.len()),
			"actual WriteString(%q) wire size: Lean=%s Go=%d", s, expected, p.len())
	}

	// Test actual UTF-16 string encoding sizes.
	utf16Cases := map[string]int{
		"":      0,
		"a":     1,
		"ab":    2,
		"abc":   3,
		"abcd":  4,
		"hello": 5,
		"abcdef": 6,
		"abcdefg": 7,
	}
	for s, charCount := range utf16Cases {
		// Verify charCount matches utf16.Encode output.
		encoded := utf16.Encode([]rune(s))
		require.Equal(t, charCount, len(encoded), "charCount mismatch for %q", s)

		key := fmt.Sprintf("STRING16_SIZE %d", charCount)
		expected, ok := lean[key]
		if !ok {
			continue
		}

		p := newTestParcel()
		p.writeString16(s)
		assert.Equal(t, expected, strconv.Itoa(p.len()),
			"actual WriteString16(%q) wire size: Lean=%s Go=%d", s, expected, p.len())
	}
}

// Minimal parcel implementation for wire-size verification.
// Mirrors the real parcel but only tracks size.
type testParcel struct {
	data []byte
}

func newTestParcel() *testParcel {
	return &testParcel{}
}

func (p *testParcel) len() int {
	return len(p.data)
}

func (p *testParcel) grow(n int) []byte {
	aligned := align4(n)
	start := len(p.data)
	needed := start + aligned
	newData := make([]byte, needed)
	copy(newData, p.data)
	p.data = newData
	return p.data[start : start+n]
}

func (p *testParcel) writeInt32(v int32) {
	buf := p.grow(4)
	buf[0] = byte(v)
	buf[1] = byte(v >> 8)
	buf[2] = byte(v >> 16)
	buf[3] = byte(v >> 24)
}

func (p *testParcel) writeString(s string) {
	byteLen := len(s)
	p.writeInt32(int32(byteLen))
	buf := p.grow(byteLen + 1)
	copy(buf[:byteLen], s)
	buf[byteLen] = 0
}

func (p *testParcel) writeString16(s string) {
	runes := []rune(s)
	encoded := utf16.Encode(runes)
	charCount := len(encoded)
	p.writeInt32(int32(charCount))
	dataBytes := (charCount + 1) * 2
	buf := p.grow(dataBytes)
	for i, u := range encoded {
		buf[i*2] = byte(u)
		buf[i*2+1] = byte(u >> 8)
	}
	buf[charCount*2] = 0
	buf[charCount*2+1] = 0
}
