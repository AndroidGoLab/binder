package versionaware

import (
	"debug/elf"
	"encoding/binary"
	"encoding/json"
	"os"
	"strconv"
	"strings"
)

// detectAPILevel returns the Android API level of the running device.
// Tries multiple detection methods in order:
// 1. Parse .note.android.ident from /system/lib64/libbinder_ndk.so (ELF note, always readable)
// 2. Read RELEASE_PLATFORM_SDK_VERSION from /etc/build_flags.json
// Returns 0 if detection fails (e.g. when running outside Android).
func detectAPILevel() int {
	if n := detectViaBinderNDKNote(); n > 0 {
		return n
	}
	return detectViaBuildFlags()
}

// binderNDKPaths lists candidate locations for libbinder_ndk.so.
var binderNDKPaths = []string{
	"/system/lib64/libbinder_ndk.so",
	"/system/lib/libbinder_ndk.so",
}

// detectViaBinderNDKNote reads the .note.android.ident ELF note from
// libbinder_ndk.so to extract the Android API level. This note is
// present in all Android system libraries and is always readable.
func detectViaBinderNDKNote() int {
	for _, path := range binderNDKPaths {
		n := parseELFAndroidAPILevel(path)
		if n > 0 {
			return n
		}
	}
	return 0
}

// parseELFAndroidAPILevel reads the .note.android.ident section from
// an ELF binary and returns the API level stored in it.
func parseELFAndroidAPILevel(path string) int {
	f, err := elf.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()

	section := f.Section(".note.android.ident")
	if section == nil {
		return 0
	}

	data, err := section.Data()
	if err != nil {
		return 0
	}

	// Parse ELF note format: namesz(4) + descsz(4) + type(4) + name(aligned) + desc(aligned)
	for len(data) >= 12 {
		namesz := binary.LittleEndian.Uint32(data[0:4])
		descsz := binary.LittleEndian.Uint32(data[4:8])
		// noteType := binary.LittleEndian.Uint32(data[8:12])

		nameOff := uint32(12)
		// Guard against overflow: if namesz is large enough that
		// nameOff+namesz wraps around uint32, stop parsing.
		if namesz > uint32(len(data))-nameOff {
			break
		}
		nameEnd := nameOff + namesz
		// Align to 4 bytes.
		descOff := (nameEnd + 3) &^ 3
		if descOff < nameEnd {
			break // overflow from alignment
		}
		if descsz > uint32(len(data))-descOff {
			break // descEnd would exceed data
		}
		descEnd := descOff + descsz
		nextOff := (descEnd + 3) &^ 3
		if nextOff < descEnd {
			break // overflow from alignment
		}

		name := strings.TrimRight(string(data[nameOff:nameEnd]), "\x00")
		if name == "Android" && descsz >= 4 {
			apiLevel := binary.LittleEndian.Uint32(data[descOff : descOff+4])
			return int(apiLevel)
		}

		if nextOff == 0 || int(nextOff) > len(data) {
			break
		}
		data = data[nextOff:]
	}

	return 0
}

// buildFlagsPaths lists candidate locations for the build flags file.
var buildFlagsPaths = []string{
	"/etc/build_flags.json",
	"/system/etc/build_flags.json",
}

func detectViaBuildFlags() int {
	for _, path := range buildFlagsPaths {
		n := parseBuildFlags(path)
		if n > 0 {
			return n
		}
	}
	return 0
}

// buildFlagsFile handles two JSON schemas found on Android devices:
//   - API 36+ (Pixel 8a): {"flags": [{"flag_declaration": {...}, "value": {...}}]}
//   - API 35  (emulator): {"flag_artifacts": [{"flag_declaration": {...}, "val": {...}}]}
type buildFlagsFile struct {
	Flags         []buildFlagEntry `json:"flags"`
	FlagArtifacts []buildFlagEntry `json:"flag_artifacts"`
}

type buildFlagEntry struct {
	Declaration buildFlagDeclaration `json:"flag_declaration"`
	// "value" (API 36 schema) or "val" (API 35 schema) — try both.
	Value buildFlagVal `json:"value"`
	Val   buildFlagVal `json:"val"`
}

type buildFlagDeclaration struct {
	Name string `json:"name"`
}

type buildFlagVal struct {
	Val map[string]json.RawMessage `json:"Val"`
}

func parseBuildFlags(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}

	var file buildFlagsFile
	if err := json.Unmarshal(data, &file); err != nil {
		return 0
	}

	// Merge both arrays — one will be empty depending on the schema.
	// Use a fresh slice to avoid mutating file.Flags's backing array,
	// which could corrupt data if Flags has spare capacity.
	entries := make([]buildFlagEntry, 0, len(file.Flags)+len(file.FlagArtifacts))
	entries = append(entries, file.Flags...)
	entries = append(entries, file.FlagArtifacts...)

	for _, f := range entries {
		if f.Declaration.Name != "RELEASE_PLATFORM_SDK_VERSION" {
			continue
		}

		// Try "value" field first (API 36 schema), then "val" (API 35 schema).
		raw, ok := f.Value.Val["StringValue"]
		if !ok {
			raw, ok = f.Val.Val["StringValue"]
		}
		if !ok {
			return 0
		}

		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return 0
		}
		n, err := strconv.Atoi(s)
		if err != nil {
			return 0
		}
		return n
	}
	return 0
}
