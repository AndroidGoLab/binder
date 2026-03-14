package versionaware

import (
	"encoding/json"
	"os"
	"strconv"
)

// detectAPILevel returns the Android API level of the running device.
// Reads /etc/build_flags.json (world-readable, no root needed, no fork).
// Returns 0 if detection fails (e.g. when running outside Android).
func detectAPILevel() int {
	return detectViaBuildFlags()
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

// buildFlags is the top-level structure of /etc/build_flags.json.
type buildFlags struct {
	Flags []buildFlag `json:"flags"`
}

type buildFlag struct {
	Declaration buildFlagDeclaration `json:"flag_declaration"`
	Value       buildFlagValue       `json:"value"`
}

type buildFlagDeclaration struct {
	Name string `json:"name"`
}

type buildFlagValue struct {
	Val map[string]json.RawMessage `json:"Val"`
}

func parseBuildFlags(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}

	var flags buildFlags
	if err := json.Unmarshal(data, &flags); err != nil {
		return 0
	}

	for _, f := range flags.Flags {
		if f.Declaration.Name != "RELEASE_PLATFORM_SDK_VERSION" {
			continue
		}
		raw, ok := f.Value.Val["StringValue"]
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
