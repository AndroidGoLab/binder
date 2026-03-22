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
	"bufio"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/AndroidGoLab/binder/tools/pkg/spec"
)

const (
	moduleBase = "github.com/AndroidGoLab/binder"
)

// generatedDirs lists directories containing generated Go code.
var generatedDirs = []string{"android", "com"}

// codebaseStats holds dynamically computed statistics about the generated codebase.
type codebaseStats struct {
	goFiles    int
	packages   int
	methods    int
	interfaces int
}

// computeCodebaseStats counts Go files, packages, and proxy methods in the
// generated code directories.
func computeCodebaseStats() (codebaseStats, error) {
	var stats codebaseStats
	packageDirs := make(map[string]bool)
	proxyMethodRe := regexp.MustCompile(`^func \(p \*\w+`)
	interfaceRe := regexp.MustCompile(`^type I\w+ interface \{`)

	for _, dir := range generatedDirs {
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil // skip inaccessible paths
			}
			if info.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			stats.goFiles++
			packageDirs[filepath.Dir(path)] = true

			f, err := os.Open(path)
			if err != nil {
				return nil
			}
			defer f.Close()

			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				line := scanner.Text()
				if proxyMethodRe.MatchString(line) {
					stats.methods++
				}
				if interfaceRe.MatchString(line) {
					stats.interfaces++
				}
			}
			return nil
		})
		if err != nil {
			return stats, fmt.Errorf("walking %s: %w", dir, err)
		}
	}

	stats.packages = len(packageDirs)
	return stats, nil
}

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
		{"BINDER_MCP", renderBinderMCP()},
		{"INTEROPERABILITY", renderInteroperability()},
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

	// Compute and update inline codebase stats.
	stats, err := computeCodebaseStats()
	if err != nil {
		return fmt.Errorf("computing codebase stats: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Codebase stats: %d Go files, %d packages, %d methods, %d interfaces\n",
		stats.goFiles, stats.packages, stats.methods, stats.interfaces)

	content = updateInlineStats(content, stats, packages)

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
		"gps_location":          "Live GPS fix via ILocationListener callback",
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
	return `**Go library** — ` + "`go get github.com/AndroidGoLab/binder`" + ` — live GPS location via binder IPC:

` + "```go" + `
package main

import (
    "context"
    "fmt"
    "math"
    "os"
    "time"

    "github.com/AndroidGoLab/binder/android/location"
    androidos "github.com/AndroidGoLab/binder/android/os"
    "github.com/AndroidGoLab/binder/binder"
    "github.com/AndroidGoLab/binder/binder/versionaware"
    "github.com/AndroidGoLab/binder/kernelbinder"
    "github.com/AndroidGoLab/binder/servicemanager"
)

// gpsListener receives location callbacks from the LocationManager.
type gpsListener struct{ fixCh chan location.Location }

func (l *gpsListener) OnLocationChanged(_ context.Context, locs []location.Location, _ androidos.IRemoteCallback) error {
    for _, loc := range locs { select { case l.fixCh <- loc: default: } }
    return nil
}
func (l *gpsListener) OnProviderEnabledChanged(_ context.Context, _ string, _ bool) error { return nil }
func (l *gpsListener) OnFlushComplete(_ context.Context, _ int32) error                   { return nil }

func main() {
    ctx := context.Background()
    drv, _ := kernelbinder.Open(ctx, binder.WithMapSize(128*1024))
    defer drv.Close(ctx)
    transport, _ := versionaware.NewTransport(ctx, drv, 0)
    sm := servicemanager.New(transport)

    lm, _ := location.GetLocationManager(ctx, sm)

    impl := &gpsListener{fixCh: make(chan location.Location, 1)}
    listener := location.NewLocationListenerStub(impl)

    request := location.LocationRequest{
        Provider: location.GpsProvider, IntervalMillis: 1000,
        ExpireAtRealtimeMillis: math.MaxInt64, DurationMillis: math.MaxInt64,
    }
    pkg := binder.DefaultCallerIdentity().PackageName
    _ = lm.RegisterLocationListener(ctx, location.GpsProvider, request, listener, pkg, "gps")
    defer lm.UnregisterLocationListener(ctx, listener)

    select {
    case loc := <-impl.fixCh:
        fmt.Printf("Lat: %.6f  Lon: %.6f  Alt: %.1f m  Accuracy: %.1f m\n",
            loc.LatitudeDegrees, loc.LongitudeDegrees, loc.AltitudeMeters, loc.HorizontalAccuracyMeters)
    case <-time.After(30 * time.Second):
        fmt.Fprintln(os.Stderr, "timed out")
    }
}
` + "```" + `

Full runnable example: [` + "`examples/gps_location/`" + `](examples/gps_location/)

Or query power state:

` + "```go" + `
power, _ := os.GetPowerManager(ctx, sm)
interactive, _ := power.IsInteractive(ctx)
fmt.Printf("Screen on: %v\n", interactive)
` + "```" + `
`
}

func renderUsageExamples() string {
	return `### Get GPS Location

` + "```go" + `
import (
    "context"
    "fmt"
    "log"

    "github.com/AndroidGoLab/binder/android/location"
    "github.com/AndroidGoLab/binder/binder"
    "github.com/AndroidGoLab/binder/binder/versionaware"
    "github.com/AndroidGoLab/binder/kernelbinder"
    "github.com/AndroidGoLab/binder/servicemanager"
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

    lm, err := location.GetLocationManager(ctx, sm)
    if err != nil {
        log.Fatal(err)
    }

    loc, err := lm.GetLastLocation(ctx, location.FusedProvider, location.LastLocationRequest{}, binder.DefaultCallerIdentity().PackageName)
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Lat: %f, Lon: %f, Alt: %f\n",
        loc.LatitudeDegrees, loc.LongitudeDegrees, loc.AltitudeMeters)
    fmt.Printf("Speed: %f m/s, Bearing: %f°\n",
        loc.SpeedMetersPerSecond, loc.BearingDegrees)
` + "```" + `

### Check Power State

` + "```go" + `
import (
    "context"
    "fmt"
    "log"

    genOs "github.com/AndroidGoLab/binder/android/os"
    "github.com/AndroidGoLab/binder/binder"
    "github.com/AndroidGoLab/binder/binder/versionaware"
    "github.com/AndroidGoLab/binder/kernelbinder"
    "github.com/AndroidGoLab/binder/servicemanager"
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
    "github.com/AndroidGoLab/binder/android/app"
    "github.com/AndroidGoLab/binder/servicemanager"
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

func renderBinderMCP() string {
	return `### Device mode

` + "```bash" + `
# Build and push
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o build/binder-mcp ./cmd/binder-mcp/
adb push build/binder-mcp /data/local/tmp/

# Use with Claude Code (or any MCP client)
# In your MCP config, add:
# {
#   "mcpServers": {
#     "android": {
#       "command": "adb",
#       "args": ["shell", "/data/local/tmp/binder-mcp"]
#     }
#   }
# }
` + "```" + `

### Remote mode (runs on host)

` + "```bash" + `
go run ./cmd/binder-mcp/ --mode remote
# Auto-discovers device via ADB, pushes daemon, serves MCP on stdio
` + "```" + `

### Available tools

| Tool | Description |
|---|---|
| ` + "`list_services`" + ` | Enumerate all binder services |
| ` + "`get_service_info`" + ` | Descriptor, handle, liveness for a service |
| ` + "`call_method`" + ` | Invoke raw binder transactions |
| ` + "`get_device_info`" + ` | Power, display, thermal status |
| ` + "`get_location`" + ` | GPS/fused location |
| ` + "`check_permissions`" + ` | SELinux context and service accessibility |
`
}

func renderInteroperability() string {
	return `<details>
<summary><strong>gadb</strong> — Pure Go ADB for CI/CD</summary>

The ` + "`interop/gadb/runner/`" + ` package provides pure-Go ADB device control
without requiring the ` + "`adb`" + ` binary. Discover devices, push binaries,
and run commands programmatically:

` + "```go" + `
dr, _ := runner.NewDeviceRunner("SERIAL")
dr.PushBinary(ctx, "build/mybinary", "/data/local/tmp/mybinary")
output, _ := dr.Run(ctx, "/data/local/tmp/mybinary", "--flag")
` + "```" + `

For remote binder access from a host machine, ` + "`interop/gadb/proxy/`" + `
sets up a forwarded session:

` + "```go" + `
sess, _ := proxy.NewSession(ctx, "SERIAL")
defer sess.Close(ctx)
sm := servicemanager.New(sess.Transport())
// Use sm as if running on-device
` + "```" + `

</details>

<details>
<summary><strong>gomobile</strong> — Android AAR</summary>

` + "`interop/gomobile/client/`" + ` wraps binder calls in a Java-friendly API
via gomobile. Build the AAR:

` + "```bash" + `
gomobile bind -target android -o binder.aar ./interop/gomobile/client/
` + "```" + `

Available methods: ` + "`GetPowerStatus()`" + `, ` + "`GetDisplayInfo()`" + `,
` + "`GetLastLocation()`" + `, ` + "`GetDeviceInfo()`" + `.

See the example app at [` + "`examples/gomobile/`" + `](examples/gomobile/).

</details>
`
}

// updateExampleCount replaces "N runnable examples" in the directory tree.
func updateExampleCount(content string, count int) string {
	re := regexp.MustCompile(`\d+ runnable examples`)
	return re.ReplaceAllString(content, fmt.Sprintf("%d runnable examples", count))
}

// roundDown rounds n down to the nearest multiple of unit.
func roundDown(n, unit int) int {
	return (n / unit) * unit
}

// formatCount returns a human-readable count like "14,000" or "5,092".
func formatCount(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	thousands := n / 1000
	remainder := n % 1000
	if remainder == 0 {
		return fmt.Sprintf("%d,000", thousands)
	}
	return fmt.Sprintf("%d,%03d", thousands, remainder)
}

// updateInlineStats replaces hardcoded codebase statistics in the README with
// dynamically computed values from the generated code.
func updateInlineStats(content string, stats codebaseStats, packages []packageInfo) string {
	// Compute human-friendly rounded values.
	methodsRounded := roundDown(stats.methods, 1000) // e.g. 14454 -> 14000

	// Compute total spec-based interface count for the "N+ interfaces" pattern.
	totalInterfaces := 0
	for _, pkg := range packages {
		totalInterfaces += pkg.interfaceCount
	}
	interfacesRounded := roundDown(totalInterfaces, 100) // e.g. 1507 -> 1500

	replacements := []struct {
		pattern string
		replace string
	}{
		// Hero paragraph: "~14,000 type-safe Go methods"
		{`~[\d,]+ type-safe Go methods`, fmt.Sprintf("~%s type-safe Go methods", formatCount(methodsRounded))},
		// Hero paragraph: "across 1,500+ Android interfaces"
		{`across [\d,]+\+ Android (?:system services|interfaces)`, fmt.Sprintf("across %s+ Android interfaces", formatCount(interfacesRounded))},
		// Bullet point: "and more" (no misleading count)
		{`and [\d,]+\+ more`, "and more"},
		// bindercli section: "1,500+ interfaces, 14,000+ methods"
		{`[\d,]+\+ interfaces, [\d,]+\+ methods`, fmt.Sprintf("%s+ interfaces, %s+ methods", formatCount(interfacesRounded), formatCount(methodsRounded))},
		// AOSP bulk generation: "**5,490 Go files** across **666 packages**"
		{`\*\*[\d,]+ Go files\*\* across \*\*[\d,]+ packages\*\*`,
			fmt.Sprintf("**%s Go files** across **%s packages**", formatCount(stats.goFiles), formatCount(stats.packages))},
		// Directory tree: "(5,490 files)"
		{`Pre-generated AOSP service proxies \([\d,]+ files\)`,
			fmt.Sprintf("Pre-generated AOSP service proxies (%s files)", formatCount(stats.goFiles))},
		// Directory tree: "666 packages total"
		{`[\d,]+ packages total`, fmt.Sprintf("%s packages total", formatCount(stats.packages))},
		// AIDL file count: "~12,000 AIDL files" — compute from actual AIDL source count
		// (leave as-is since this refers to AIDL source files, not generated Go files)
	}

	for _, r := range replacements {
		re := regexp.MustCompile(r.pattern)
		content = re.ReplaceAllString(content, r.replace)
	}

	// Also update the log-scale approximation: "~12,000 AIDL files"
	// We keep this as a round number since it refers to the AIDL source files.
	aidlRounded := int(math.Round(float64(countAIDLFiles()) / 1000.0)) * 1000
	if aidlRounded > 0 {
		aidlRe := regexp.MustCompile(`~[\d,]+ AIDL files`)
		content = aidlRe.ReplaceAllString(content, fmt.Sprintf("~%s AIDL files", formatCount(aidlRounded)))
	}

	return content
}

// countAIDLFiles counts .aidl files in the 3rdparty directory.
func countAIDLFiles() int {
	count := 0
	_ = filepath.Walk("tools/pkg/3rdparty", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && strings.HasSuffix(path, ".aidl") {
			count++
		}
		return nil
	})
	return count
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
