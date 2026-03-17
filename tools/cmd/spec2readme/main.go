// Command spec2readme reads YAML spec files (produced by aidl2spec) and
// updates a README.md with a generated package listing section.
// It replaces genreadme, which scanned directory structure instead of specs.
//
// Usage:
//
//	spec2readme -specs specs/ -output README.md
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/xaionaro-go/binder/tools/pkg/spec"
)

const (
	beginMarker = "<!-- BEGIN GENERATED PACKAGES -->"
	endMarker   = "<!-- END GENERATED PACKAGES -->"
	moduleBase  = "github.com/xaionaro-go/binder"
)

func main() {
	specsDir := flag.String("specs", "specs/", "Directory containing spec YAML files")
	outputPath := flag.String("output", "README.md", "Output README path to update")
	flag.Parse()

	if err := run(*specsDir, *outputPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(
	specsDir string,
	outputPath string,
) error {
	fmt.Fprintf(os.Stderr, "Reading specs from %s...\n", specsDir)
	specs, err := spec.ReadAllSpecs(specsDir)
	if err != nil {
		return fmt.Errorf("reading specs: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Read %d package specs\n", len(specs))

	packages := collectPackageInfo(specs)
	sort.Slice(packages, func(i, j int) bool {
		return packages[i].importPath < packages[j].importPath
	})

	table := renderTable(packages)

	readme, err := os.ReadFile(outputPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", outputPath, err)
	}

	result, err := replaceBetweenMarkers(string(readme), table)
	if err != nil {
		return err
	}

	return os.WriteFile(outputPath, []byte(result), 0o644)
}

// packageInfo describes a single package for README generation.
type packageInfo struct {
	dir             string
	importPath      string
	interfaceCount  int
	parcelableCount int
	enumCount       int
	unionCount      int
	serviceCount    int
}

// collectPackageInfo converts specs to packageInfo entries.
func collectPackageInfo(
	specs map[string]*spec.PackageSpec,
) []packageInfo {
	var packages []packageInfo

	for _, ps := range specs {
		totalDefs := len(ps.Interfaces) + len(ps.Parcelables) + len(ps.Enums) + len(ps.Unions)
		if totalDefs == 0 {
			continue
		}

		packages = append(packages, packageInfo{
			dir:             ps.GoPackage,
			importPath:      moduleBase + "/" + ps.GoPackage,
			interfaceCount:  len(ps.Interfaces),
			parcelableCount: len(ps.Parcelables),
			enumCount:       len(ps.Enums),
			unionCount:      len(ps.Unions),
			serviceCount:    len(ps.Services),
		})
	}

	return packages
}

// packageGroup groups packages under a common prefix for collapsible display.
type packageGroup struct {
	name     string
	packages []packageInfo
}

func groupPackages(
	packages []packageInfo,
) []packageGroup {
	groupMap := make(map[string][]packageInfo)
	var groupOrder []string

	for _, pkg := range packages {
		rel := filepath.ToSlash(pkg.dir)
		parts := strings.SplitN(rel, "/", 3)

		var groupName string
		switch {
		case len(parts) >= 2:
			groupName = parts[0] + "/" + parts[1]
		default:
			groupName = parts[0]
		}

		if _, exists := groupMap[groupName]; !exists {
			groupOrder = append(groupOrder, groupName)
		}
		groupMap[groupName] = append(groupMap[groupName], pkg)
	}

	sort.Strings(groupOrder)

	groups := make([]packageGroup, 0, len(groupOrder))
	for _, name := range groupOrder {
		groups = append(groups, packageGroup{
			name:     name,
			packages: groupMap[name],
		})
	}
	return groups
}

func renderTable(
	packages []packageInfo,
) string {
	var b strings.Builder

	groups := groupPackages(packages)

	totalInterfaces := 0
	totalParcelables := 0
	totalEnums := 0
	totalUnions := 0
	for _, pkg := range packages {
		totalInterfaces += pkg.interfaceCount
		totalParcelables += pkg.parcelableCount
		totalEnums += pkg.enumCount
		totalUnions += pkg.unionCount
	}

	fmt.Fprintf(&b, "%d packages: %d interfaces, %d parcelables, %d enums, %d unions.\n\n",
		len(packages), totalInterfaces, totalParcelables, totalEnums, totalUnions)

	for _, g := range groups {
		fmt.Fprintf(&b, "<details>\n")
		fmt.Fprintf(&b, "<summary><strong>%s</strong> (%d packages)</summary>\n\n", g.name, len(g.packages))
		fmt.Fprintf(&b, "| Package | Interfaces | Parcelables | Enums | Unions | Import Path |\n")
		fmt.Fprintf(&b, "|---|---|---|---|---|---|\n")

		for _, pkg := range g.packages {
			displayName := filepath.ToSlash(pkg.dir)
			fmt.Fprintf(&b, "| [`%s`](https://pkg.go.dev/%s) | %d | %d | %d | %d | `%s` |\n",
				displayName, pkg.importPath,
				pkg.interfaceCount, pkg.parcelableCount, pkg.enumCount, pkg.unionCount,
				pkg.importPath)
		}

		fmt.Fprintf(&b, "\n</details>\n\n")
	}

	return b.String()
}

func replaceBetweenMarkers(
	content string,
	replacement string,
) (string, error) {
	beginIdx := strings.Index(content, beginMarker)
	if beginIdx == -1 {
		return "", fmt.Errorf("marker %q not found in README", beginMarker)
	}

	endIdx := strings.Index(content, endMarker)
	if endIdx == -1 {
		return "", fmt.Errorf("marker %q not found in README", endMarker)
	}

	if endIdx <= beginIdx {
		return "", fmt.Errorf("end marker appears before begin marker")
	}

	var b strings.Builder
	b.WriteString(content[:beginIdx])
	b.WriteString(beginMarker)
	b.WriteString("\n\n")
	b.WriteString(replacement)
	b.WriteString(endMarker)
	b.WriteString(content[endIdx+len(endMarker):])

	return b.String(), nil
}
