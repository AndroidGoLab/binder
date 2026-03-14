package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	beginMarker = "<!-- BEGIN GENERATED PACKAGES -->"
	endMarker   = "<!-- END GENERATED PACKAGES -->"
	moduleBase  = "github.com/xaionaro-go/aidl"
)

// generatedDirs lists the top-level directories containing generated code.
var generatedDirs = []string{
	"android",
	"com",
	"fuzztest",
	"libgui_test_server",
	"parcelables",
	"src",
}

func main() {
	readmePath := "README.md"

	if len(os.Args) > 1 {
		readmePath = os.Args[1]
	}

	if err := run(readmePath); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(
	readmePath string,
) error {
	var packages []packageInfo
	for _, dir := range generatedDirs {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			continue
		}
		pkgs, err := discoverPackages(dir)
		if err != nil {
			return fmt.Errorf("discovering packages in %s: %w", dir, err)
		}
		packages = append(packages, pkgs...)
	}

	sort.Slice(packages, func(i, j int) bool {
		return packages[i].importPath < packages[j].importPath
	})

	table := renderTable(packages)

	readme, err := os.ReadFile(readmePath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", readmePath, err)
	}

	result, err := replaceBetweenMarkers(string(readme), table)
	if err != nil {
		return err
	}

	return os.WriteFile(readmePath, []byte(result), 0o644)
}

type packageInfo struct {
	dir        string
	importPath string
	fileCount  int
}

func discoverPackages(dir string) ([]packageInfo, error) {
	var packages []packageInfo

	err := filepath.Walk(dir, func(
		path string,
		info os.FileInfo,
		err error,
	) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			return nil
		}

		goFiles, globErr := filepath.Glob(filepath.Join(path, "*.go"))
		if globErr != nil {
			return nil
		}

		// Filter out test files.
		var count int
		for _, f := range goFiles {
			if !strings.HasSuffix(f, "_test.go") {
				count++
			}
		}

		if count == 0 {
			return nil
		}

		packages = append(packages, packageInfo{
			dir:        path,
			importPath: moduleBase + "/" + filepath.ToSlash(path),
			fileCount:  count,
		})
		return nil
	})

	return packages, err
}

func renderTable(
	packages []packageInfo,
) string {
	var b strings.Builder

	groups := groupPackages(packages)

	totalFiles := 0
	for _, pkg := range packages {
		totalFiles += pkg.fileCount
	}

	fmt.Fprintf(&b, "%d packages, %d generated Go files.\n\n", len(packages), totalFiles)

	for _, g := range groups {
		fmt.Fprintf(&b, "<details>\n")
		fmt.Fprintf(&b, "<summary><strong>%s</strong> (%d packages)</summary>\n\n", g.name, len(g.packages))
		fmt.Fprintf(&b, "| Package | Files | Import Path |\n")
		fmt.Fprintf(&b, "|---|---|---|\n")

		for _, pkg := range g.packages {
			displayName := filepath.ToSlash(pkg.dir)

			fmt.Fprintf(&b, "| [`%s`](https://pkg.go.dev/%s) | %d | `%s` |\n",
				displayName, pkg.importPath, pkg.fileCount, pkg.importPath)
		}

		fmt.Fprintf(&b, "\n</details>\n\n")
	}

	return b.String()
}

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
