// Command spec2readme reads YAML spec files (produced by aidl2spec) and
// updates a README.md with generated sections: package listing, quick-start
// example, usage examples, generated-code documentation, and examples table.
// It replaces content between matched marker pairs.
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
	"regexp"
	"sort"
	"strings"

	"github.com/xaionaro-go/binder/tools/pkg/spec"
)

const (
	moduleBase = "github.com/xaionaro-go/binder"
)

func main() {
	specsDir := flag.String("specs", "specs/", "Directory containing spec YAML files")
	outputPath := flag.String("output", "README.md", "Output README path to update")
	examplesDir := flag.String("examples", "examples/", "Directory containing example programs")
	flag.Parse()

	if err := run(*specsDir, *outputPath, *examplesDir); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(
	specsDir string,
	outputPath string,
	examplesDir string,
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

	examples, err := discoverExamples(examplesDir)
	if err != nil {
		return fmt.Errorf("discovering examples: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Found %d examples\n", len(examples))

	readme, err := os.ReadFile(outputPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", outputPath, err)
	}

	content := string(readme)

	sections := []struct {
		name    string
		content string
	}{
		{"PACKAGES", renderPackageTable(packages)},
		{"QUICK_START", renderQuickStart()},
		{"USAGE_EXAMPLES", renderUsageExamples()},
		{"GENERATED_CODE", renderGeneratedCode()},
		{"EXAMPLES_TABLE", renderExamplesTable(examples)},
		{"BINDERCLI_POWER", renderBindercliPower()},
	}

	for _, s := range sections {
		beginMarker := fmt.Sprintf("<!-- BEGIN GENERATED %s -->", s.name)
		endMarker := fmt.Sprintf("<!-- END GENERATED %s -->", s.name)
		content, err = replaceBetweenMarkers(content, beginMarker, endMarker, s.content)
		if err != nil {
			return fmt.Errorf("section %s: %w", s.name, err)
		}
	}

	// Update inline example count in directory tree.
	content = updateExampleCount(content, len(examples))

	return os.WriteFile(outputPath, []byte(content), 0o644)
}

// exampleInfo describes a runnable example in the examples/ directory.
type exampleInfo struct {
	name string
	desc string
}

// discoverExamples scans the examples directory for subdirectories
// containing main.go files and extracts the first line of the doc comment.
func discoverExamples(dir string) ([]exampleInfo, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	descriptions := map[string]string{
		"list_services":         "Enumerate all binder services, ping each",
		"activity_manager":      "Process limits, monkey test flag, permission checks",
		"battery_health":        "Capacity, charge status, current draw",
		"device_info":           "Device properties, build info",
		"display_info":          "Display IDs, brightness, night mode",
		"audio_status":          "Audio device info, volume state",
		"power_status":          "Power supply state, charging info",
		"storage_info":          "Storage device stats, mount points",
		"package_query":         "Package list, installation info",
		"softap_manage":         "WiFi hotspot enable/disable, config",
		"softap_wifi_hal":       "WiFi chip info, AP interface state",
		"softap_tether_offload": "Tethering offload config, stats",
		"camera_connect":        "Camera device connection with callback stub",
	}

	var examples []exampleInfo
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		mainPath := filepath.Join(dir, e.Name(), "main.go")
		if _, err := os.Stat(mainPath); err != nil {
			continue
		}
		desc := descriptions[e.Name()]
		if desc == "" {
			desc = extractDocComment(mainPath)
		}
		examples = append(examples, exampleInfo{
			name: e.Name(),
			desc: desc,
		})
	}

	sort.Slice(examples, func(i, j int) bool {
		return examples[i].name < examples[j].name
	})

	return examples, nil
}

// extractDocComment reads the first line of a Go file's package doc comment.
func extractDocComment(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "//") {
			return strings.TrimSpace(strings.TrimPrefix(line, "//"))
		}
		if line != "" {
			break
		}
	}
	return ""
}

func renderQuickStart() string {
	return `**Go library** — ` + "`go get github.com/xaionaro-go/binder`" + ` and call any service:

` + "```go" + `
driver, _ := kernelbinder.Open(ctx, binder.WithMapSize(128*1024))
defer driver.Close(ctx)
transport, _ := versionaware.NewTransport(ctx, driver, 0)
sm := servicemanager.New(transport)

power, _ := os.GetPowerManager(ctx, sm)
interactive, _ := power.IsInteractive(ctx)
fmt.Printf("Screen on: %v\n", interactive)
` + "```" + `
`
}

func renderUsageExamples() string {
	return `### Check Power State

` + "```go" + `
import (
    "context"
    "fmt"
    "log"

    genOs "github.com/xaionaro-go/binder/android/os"
    "github.com/xaionaro-go/binder/binder"
    "github.com/xaionaro-go/binder/binder/versionaware"
    "github.com/xaionaro-go/binder/kernelbinder"
    "github.com/xaionaro-go/binder/servicemanager"
)

    ctx := context.Background()

    driver, err := kernelbinder.Open(ctx, binder.WithMapSize(128*1024))
    if err != nil {
        log.Fatal(err)
    }
    defer driver.Close(ctx)

    transport, err := versionaware.NewTransport(ctx, driver, 0)
    if err != nil {
        log.Fatal(err)
    }
    sm := servicemanager.New(transport)

    power, err := genOs.GetPowerManager(ctx, sm)
    if err != nil {
        log.Fatal(err)
    }

    interactive, _ := power.IsInteractive(ctx)
    fmt.Printf("Screen on: %v\n", interactive)

    powerSave, _ := power.IsPowerSaveMode(ctx)
    fmt.Printf("Power save: %v\n", powerSave)
` + "```" + `

### List Binder Services

` + "```go" + `
    sm := servicemanager.New(transport)

    services, err := sm.ListServices(ctx)
    if err != nil {
        log.Fatal(err)
    }

    for _, name := range services {
        svc, err := sm.CheckService(ctx, name)
        if err == nil && svc != nil && svc.IsAlive(ctx) {
            fmt.Printf("%-60s alive\n", name)
        }
    }
` + "```" + `

### Call a System Service (ActivityManager)

` + "```go" + `
import (
    "github.com/xaionaro-go/binder/android/app"
    "github.com/xaionaro-go/binder/servicemanager"
)

    svc, err := sm.GetService(ctx, servicemanager.ActivityService)
    if err != nil {
        log.Fatal(err)
    }
    am := app.NewActivityManagerProxy(svc)

    limit, _ := am.GetProcessLimit(ctx)
    fmt.Printf("Process limit: %d\n", limit)

    monkey, _ := am.IsUserAMonkey(ctx)
    fmt.Printf("Is monkey: %v\n", monkey)
` + "```" + `

More examples: [` + "`examples/`" + `](examples/)
`
}

func renderGeneratedCode() string {
	return `For an AIDL interface like:

` + "```java" + `
// android/app/IActivityManager.aidl
package android.app;

interface IActivityManager {
    int getProcessLimit();
    int checkPermission(in String permission, int pid, int uid);
    boolean isUserAMonkey();
    // ... 200+ more methods
}
` + "```" + `

The compiler generates:

` + "```go" + `
package app

const DescriptorIActivityManager = "android.app.IActivityManager"

const (
    TransactionIActivityManagerGetProcessLimit  = binder.FirstCallTransaction + 51
    TransactionIActivityManagerCheckPermission  = binder.FirstCallTransaction + 8
    // ...
)

const (
    MethodIActivityManagerGetProcessLimit  = "getProcessLimit"
    MethodIActivityManagerCheckPermission  = "checkPermission"
    // ...
)

type IActivityManager interface {
    GetProcessLimit(ctx context.Context) (int32, error)
    CheckPermission(ctx context.Context, permission string, pid int32, uid int32) (int32, error)
    IsUserAMonkey(ctx context.Context) (bool, error)
    // ...
}

type ActivityManagerProxy struct {
    Remote binder.IBinder
}

func NewActivityManagerProxy(remote binder.IBinder) *ActivityManagerProxy {
    return &ActivityManagerProxy{Remote: remote}
}

func (p *ActivityManagerProxy) GetProcessLimit(ctx context.Context) (int32, error) {
    var _result int32
    _data := parcel.New()
    defer _data.Recycle()
    _data.WriteInterfaceToken(DescriptorIActivityManager)

    _code, _err := p.Remote.ResolveCode(ctx, DescriptorIActivityManager, MethodIActivityManagerGetProcessLimit)
    if _err != nil {
        return _result, fmt.Errorf("resolving %s.%s: %w", DescriptorIActivityManager, MethodIActivityManagerGetProcessLimit, _err)
    }

    _reply, _err := p.Remote.Transact(ctx, _code, 0, _data)
    if _err != nil {
        return _result, _err
    }
    defer _reply.Recycle()

    if _err = binder.ReadStatus(_reply); _err != nil {
        return _result, _err
    }

    _result, _err = _reply.ReadInt32()
    if _err != nil {
        return _result, _err
    }
    return _result, nil
}
` + "```" + `
`
}

func renderExamplesTable(examples []exampleInfo) string {
	var b strings.Builder
	b.WriteString("| Example                                                    | Queries                                             |\n")
	b.WriteString("| ---------------------------------------------------------- | --------------------------------------------------- |\n")
	for _, ex := range examples {
		fmt.Fprintf(&b, "| [`%s`](examples/%s/) | %s |\n", ex.name, ex.name, ex.desc)
	}
	return b.String()
}

func renderBindercliPower() string {
	return `<summary>Query power and battery state</summary>

` + "```bash" + `
# Check if screen is on
bindercli android.os.IPowerManager is-interactive
# Example output: {"result":true}

# Check power save mode
bindercli android.os.IPowerManager is-power-save-mode
# Example output: {"result":false}

# Check if device is in Doze mode
bindercli android.os.IPowerManager is-device-idle-mode
# Example output: {"result":false}

# Get battery health info
bindercli android.hardware.health.IHealth get-health-info
` + "```" + `
`
}

// updateExampleCount replaces "N runnable examples" in the directory tree.
func updateExampleCount(content string, count int) string {
	re := regexp.MustCompile(`\d+ runnable examples`)
	return re.ReplaceAllString(content, fmt.Sprintf("%d runnable examples", count))
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

func renderPackageTable(
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
	beginMarker string,
	endMarker string,
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
	b.WriteString("\n")
	b.WriteString(endMarker)
	b.WriteString(content[endIdx+len(endMarker):])

	return b.String(), nil
}
