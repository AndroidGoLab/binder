package versionaware

import (
	"context"
	"debug/elf"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/facebookincubator/go-belt/tool/logger"
	"github.com/xaionaro-go/binder/binder"
	"github.com/xaionaro-go/binder/binder/versionaware/dex"
	"github.com/xaionaro-go/binder/parcel"
)

// Transport wraps a binder.Transport and adds version-aware
// transaction code resolution via ResolveCode.
//
// All version detection (API level + revision probing) happens
// eagerly in NewTransport. If the device version cannot be
// determined or is unsupported, NewTransport returns an error.
type Transport struct {
	inner    binder.Transport
	apiLevel int
	table    VersionTable
	version  string
}

// DetectAPILevel returns the Android API level of the running device.
// Reads /etc/build_flags.json (no fork, no binder dependency).
// Returns 0 if detection fails.
func DetectAPILevel() int {
	return detectAPILevel()
}

// NewTransport creates a version-aware Transport wrapping inner.
//
// targetAPI is the Android API level (e.g., 36). If 0, auto-detects
// from /etc/build_flags.json.
//
// Returns an error if:
//   - the API level cannot be detected (targetAPI==0 and detection fails)
//   - the API level has no compiled version tables
//   - revision probing fails (when multiple revisions exist)
//
// If multiple AOSP revisions exist for the API level, NewTransport
// probes the device via binder transactions to determine which
// revision matches. This requires the binder driver to be open.
func NewTransport(
	ctx context.Context,
	inner binder.Transport,
	targetAPI int,
	opts ...Option,
) (*Transport, error) {
	cfg := Options(opts).config()

	if targetAPI <= 0 {
		targetAPI = detectAPILevel()
	}
	if targetAPI <= 0 {
		return nil, fmt.Errorf("versionaware: unable to detect Android API level; pass --target-api explicitly or ensure /etc/build_flags.json is readable; supported API levels: %v", supportedAPILevels())
	}

	// Try loading from cache if configured.
	if cfg.CachePath != "" {
		fingerprint := resolvedTableFingerprint(targetAPI)
		if table := loadCachedTable(cfg.CachePath, fingerprint); table != nil {
			logger.Debugf(ctx, "versionaware: loaded cached transaction codes from %s (%d interfaces)", cfg.CachePath, len(table))
			return &Transport{
				inner:    inner,
				apiLevel: targetAPI,
				table:    table,
				version:  fmt.Sprintf("%d.cached", targetAPI),
			}, nil
		}
	}

	table, version, err := resolveTable(ctx, inner, targetAPI)
	if err != nil {
		return nil, err
	}

	// Save to cache if configured.
	if cfg.CachePath != "" {
		fingerprint := resolvedTableFingerprint(targetAPI)
		saveCachedTable(cfg.CachePath, fingerprint, table)
		logger.Debugf(ctx, "versionaware: cached resolved transaction codes to %s", cfg.CachePath)
	}

	return &Transport{
		inner:    inner,
		apiLevel: targetAPI,
		table:    table,
		version:  version,
	}, nil
}

// resolveTable performs the full transaction code resolution:
// DEX extraction first, then compiled tables with revision detection.
func resolveTable(
	ctx context.Context,
	inner binder.Transport,
	targetAPI int,
) (VersionTable, string, error) {
	// Primary: extract transaction codes directly from the device's
	// framework JAR files. This is definitive — no version guessing.
	table := extractTransactionCodesFromDevice()
	if table != nil {
		version := fmt.Sprintf("%d.device", targetAPI)
		logger.Debugf(ctx, "versionaware: extracted transaction codes from device framework JARs (%d interfaces)", len(table))
		return table, version, nil
	}

	// Fallback: use compiled version tables with revision detection.
	// This is less reliable than DEX extraction — log at Error level
	// so users know the primary method failed.
	logger.Errorf(ctx, "versionaware: framework JARs not available or unreadable at %s; falling back to compiled version tables (transaction codes may be inaccurate)", frameworkJARDir)

	revisions := Revisions[targetAPI]
	if len(revisions) == 0 {
		return nil, "", fmt.Errorf("versionaware: API level %d is not supported and framework JARs not readable; supported API levels: %v", targetAPI, supportedAPILevels())
	}

	// Narrow revision candidates by checking which methods exist in
	// libbinder.so's BpServiceManager. This distinguishes old-style
	// (no getService2/checkService2) from new-style method ordering
	// without any binder transactions.
	revisions = filterRevisionsBySOMethodSet(revisions)

	var version string
	switch len(revisions) {
	case 1:
		version = revisions[0]
	default:
		var err error
		version, err = probeRevision(ctx, inner, targetAPI)
		if err != nil {
			return nil, "", fmt.Errorf("versionaware: probing revision for API %d: %w", targetAPI, err)
		}
	}

	table, ok := Tables[version]
	if !ok {
		return nil, "", fmt.Errorf("versionaware: no transaction code table for version %q", version)
	}

	logger.Debugf(ctx, "versionaware: using compiled version table %s (%d interfaces)", version, len(table))
	return table, version, nil
}

// resolvedTableFingerprint returns a fingerprint that changes when
// the device's firmware changes, invalidating cached transaction codes.
// Combines API level with framework JAR fingerprint (if available).
func resolvedTableFingerprint(apiLevel int) string {
	fp := frameworkFingerprint()
	return fmt.Sprintf("api=%d;jars=%s", apiLevel, fp)
}

// ResolveCode resolves an AIDL method name to the correct transaction code
// for the detected device version.
//
// Returns an error if the method is not found in the version table
// (e.g., calling a method that does not exist on the device's API level).
func (t *Transport) ResolveCode(
	descriptor string,
	method string,
) (binder.TransactionCode, error) {
	code := t.table.Resolve(descriptor, method)
	if code == 0 {
		return 0, fmt.Errorf("versionaware: method %s.%s not found in version %s", descriptor, method, t.version)
	}
	return code, nil
}

// supportedAPILevels returns the list of API levels that have version tables.
func supportedAPILevels() []int {
	levels := make([]int, 0, len(Revisions))
	for level := range Revisions {
		levels = append(levels, level)
	}
	sort.Ints(levels)
	return levels
}

const (
	serviceManagerHandle      = uint32(0)
	serviceManagerDescriptor  = "android.os.IServiceManager"
	activityManagerDescriptor = "android.app.IActivityManager"
)

// frameworkJARDir is the directory containing framework JARs
// with AIDL $Stub classes and TRANSACTION_* constants.
const frameworkJARDir = "/system/framework"

// extractTransactionCodesFromDevice scans all JARs in /system/framework/
// and extracts definitive transaction codes from DEX bytecode.
// Returns nil if the directory is not readable or no codes are found.
func extractTransactionCodesFromDevice() VersionTable {
	entries, err := os.ReadDir(frameworkJARDir)
	if err != nil {
		return nil
	}

	table := VersionTable{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jar") {
			continue
		}
		jarPath := frameworkJARDir + "/" + entry.Name()
		codes, err := dex.ExtractFromJAR(jarPath)
		if err != nil {
			continue
		}
		for descriptor, methods := range codes {
			if table[descriptor] == nil {
				table[descriptor] = make(map[string]binder.TransactionCode)
			}
			for method, code := range methods {
				table[descriptor][method] = binder.TransactionCode(code)
			}
		}
	}

	if len(table) == 0 {
		return nil
	}

	return table
}

// frameworkFingerprint returns a string identifying the current set of
// framework JARs by their names and sizes. Changes when the OS is updated.
func frameworkFingerprint() string {
	entries, err := os.ReadDir(frameworkJARDir)
	if err != nil {
		return ""
	}

	var b strings.Builder
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jar") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		fmt.Fprintf(&b, "%s:%d;", entry.Name(), info.Size())
	}
	return b.String()
}

// loadCachedTable reads a cached VersionTable from the given path.
// Returns nil if cache is missing, corrupted, or fingerprint doesn't match.
func loadCachedTable(
	cachePath string,
	fingerprint string,
) VersionTable {
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil
	}

	var cached struct {
		Fingerprint string                       `json:"fingerprint"`
		Table       map[string]map[string]uint32 `json:"table"`
	}
	if err := json.Unmarshal(data, &cached); err != nil {
		return nil
	}
	if cached.Fingerprint != fingerprint {
		return nil
	}

	table := VersionTable{}
	for desc, methods := range cached.Table {
		table[desc] = make(map[string]binder.TransactionCode)
		for method, code := range methods {
			table[desc][method] = binder.TransactionCode(code)
		}
	}
	return table
}

// saveCachedTable writes a VersionTable to the given path.
func saveCachedTable(
	cachePath string,
	fingerprint string,
	table VersionTable,
) {
	dir := filepath.Dir(cachePath)
	_ = os.MkdirAll(dir, 0o755)

	raw := make(map[string]map[string]uint32)
	for desc, methods := range table {
		raw[desc] = make(map[string]uint32)
		for method, code := range methods {
			raw[desc][method] = uint32(code)
		}
	}

	cached := struct {
		Fingerprint string                       `json:"fingerprint"`
		Table       map[string]map[string]uint32 `json:"table"`
	}{
		Fingerprint: fingerprint,
		Table:       raw,
	}

	data, err := json.Marshal(cached)
	if err != nil {
		return
	}
	_ = os.WriteFile(cachePath, data, 0o644)
}

// filterRevisionsBySOMethodSet reads BpServiceManager symbols from
// /system/lib64/libbinder.so to determine which methods exist on
// the device, then filters revision candidates to those whose method
// set matches.
func filterRevisionsBySOMethodSet(revisions []string) []string {
	deviceMethods := readBpServiceManagerMethods()
	if len(deviceMethods) == 0 {
		return revisions // can't read .so, don't filter
	}

	var filtered []string
	for _, rev := range revisions {
		table, ok := Tables[rev]
		if !ok {
			continue
		}
		smMethods := table[serviceManagerDescriptor]
		if smMethods == nil {
			continue
		}
		if methodSetMatches(smMethods, deviceMethods) {
			filtered = append(filtered, rev)
		}
	}

	if len(filtered) == 0 {
		return revisions // no match found, don't filter
	}
	return filtered
}

// methodSetMatches returns true if the version table's method set for
// an interface matches the methods found in the device's .so.
// A match means: every method in the table exists in the device methods,
// and no device methods are missing from the table.
func methodSetMatches(
	tableMethods map[string]binder.TransactionCode,
	deviceMethods map[string]bool,
) bool {
	// Check that every method in the table exists on the device.
	for method := range tableMethods {
		if !deviceMethods[method] {
			return false
		}
	}
	// Check that every device method exists in the table.
	for method := range deviceMethods {
		if _, ok := tableMethods[method]; !ok {
			return false
		}
	}
	return true
}

// readBpServiceManagerMethods reads libbinder.so and extracts the
// method names from BpServiceManager symbols.
func readBpServiceManagerMethods() map[string]bool {
	paths := []string{
		"/system/lib64/libbinder.so",
		"/system/lib/libbinder.so",
	}

	for _, path := range paths {
		methods := parseBpMethods(path, "BpServiceManager")
		if len(methods) > 0 {
			return methods
		}
	}
	return nil
}

// parseBpMethods reads an ELF shared library and extracts method names
// from the BpXxx class's exported symbols. Returns a set of method names.
func parseBpMethods(
	path string,
	className string,
) map[string]bool {
	f, err := elf.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	symbols, err := f.DynamicSymbols()
	if err != nil {
		return nil
	}

	// Match demangled C++ symbols like:
	// _ZN7android2os16BpServiceManager10getServiceE...
	// The method name is after the class name length+name prefix.
	prefix := className
	methods := map[string]bool{}

	for _, sym := range symbols {
		if sym.Info&0xf != uint8(elf.STT_FUNC) {
			continue
		}
		name := sym.Name
		// Look for mangled name containing the class name.
		idx := findMangledMethod(name, prefix)
		if idx == "" {
			continue
		}
		methods[idx] = true
	}

	return methods
}

// findMangledMethod extracts the method name from a C++ mangled symbol
// that belongs to the given class. Returns "" if not a match.
// Handles Itanium name mangling: _ZN<len><namespace><len><class><len><method>E...
func findMangledMethod(
	mangled string,
	className string,
) string {
	// Quick filter: must contain the class name.
	classLen := fmt.Sprintf("%d%s", len(className), className)
	idx := strings.Index(mangled, classLen)
	if idx < 0 {
		return ""
	}

	// Skip constructors/destructors (C1, C2, D0, D1, D2).
	rest := mangled[idx+len(classLen):]
	if len(rest) < 2 {
		return ""
	}
	if rest[0] == 'C' || rest[0] == 'D' {
		return ""
	}

	// Parse the method name length + name.
	nameLen := 0
	i := 0
	for i < len(rest) && rest[i] >= '0' && rest[i] <= '9' {
		nameLen = nameLen*10 + int(rest[i]-'0')
		i++
	}
	if nameLen == 0 || i+nameLen > len(rest) {
		return ""
	}

	methodName := rest[i : i+nameLen]
	// Convert to camelCase (first letter lowercase) to match AIDL method names.
	if len(methodName) > 0 && methodName[0] >= 'A' && methodName[0] <= 'Z' {
		methodName = string(methodName[0]+32) + methodName[1:]
	}

	return methodName
}

// probeRevision determines which revision of the given API level matches
// the running device by calling a distinguishing method on IActivityManager.
//
// Strategy: for each candidate revision, look up the transaction code for
// "isUserAMonkey" on IActivityManager. This method returns a bool (status
// int32 + bool int32 = 8 bytes). If we call the wrong transaction code,
// we either get an error or a different reply size. The first revision
// whose code produces a valid 8-byte reply wins.
func probeRevision(
	ctx context.Context,
	inner binder.Transport,
	apiLevel int,
) (string, error) {
	logger.Debugf(ctx, "probing revision for API %d", apiLevel)

	revisions := Revisions[apiLevel]
	if len(revisions) == 0 {
		return "", fmt.Errorf("no revisions for API %d", apiLevel)
	}
	if len(revisions) == 1 {
		return revisions[0], nil
	}

	// Get the activity service handle via raw ServiceManager CheckService.
	activityHandle, err := rawCheckService(ctx, inner, apiLevel, "activity")
	if err != nil {
		return "", fmt.Errorf("cannot look up activity service for probing: %w", err)
	}

	// Try each revision's code for isUserAMonkey and check reply size.
	for _, rev := range revisions {
		table := Tables[rev]
		if table == nil {
			continue
		}
		code := table.Resolve(activityManagerDescriptor, "isUserAMonkey")
		if code == 0 {
			continue
		}

		data := parcel.New()
		data.WriteInterfaceToken(activityManagerDescriptor)
		reply, err := inner.Transact(ctx, activityHandle, code, 0, data)
		if err != nil {
			logger.Debugf(ctx, "probeRevision: %s code %d: transact error: %v", rev, code, err)
			continue
		}

		replyLen := reply.Len()
		reply.Recycle()

		// isUserAMonkey returns: exception code (int32) + bool (int32) = 8 bytes.
		if replyLen == 8 {
			logger.Debugf(ctx, "probeRevision: matched %s (code %d, reply %d bytes)", rev, code, replyLen)
			return rev, nil
		}

		logger.Debugf(ctx, "probeRevision: %s code %d: unexpected reply size %d", rev, code, replyLen)
	}

	return "", fmt.Errorf("no revision matched for API %d across %d candidates; the device firmware may be newer than the compiled AIDL snapshots", apiLevel, len(revisions))
}

// rawCheckService performs a raw ServiceManager CheckService transaction
// to look up a service handle without going through the versionaware layer.
// Tries all distinct checkService codes from the version tables for the
// given API level, since we don't yet know which revision the device runs.
func rawCheckService(
	ctx context.Context,
	inner binder.Transport,
	apiLevel int,
	serviceName string,
) (uint32, error) {
	// Collect distinct checkService codes across revisions for this API level.
	seen := map[binder.TransactionCode]bool{}
	var codes []binder.TransactionCode
	for _, rev := range Revisions[apiLevel] {
		table, ok := Tables[rev]
		if !ok {
			continue
		}
		code := table.Resolve(serviceManagerDescriptor, "checkService")
		if code != 0 && !seen[code] {
			seen[code] = true
			codes = append(codes, code)
		}
	}
	if len(codes) == 0 {
		return 0, fmt.Errorf("cannot determine checkService transaction code for API %d", apiLevel)
	}

	// Try each candidate code. The correct one returns a parseable reply
	// with a binder handle; wrong codes return errors or empty replies.
	for _, code := range codes {
		handle, err := tryCheckService(ctx, inner, code, serviceName)
		if err != nil {
			logger.Debugf(ctx, "rawCheckService: code %d failed: %v", code, err)
			continue
		}
		return handle, nil
	}

	return 0, fmt.Errorf("CheckService(%q): all %d candidate codes failed for API %d", serviceName, len(codes), apiLevel)
}

// tryCheckService attempts a single CheckService transaction at the given code.
func tryCheckService(
	ctx context.Context,
	inner binder.Transport,
	code binder.TransactionCode,
	serviceName string,
) (uint32, error) {
	data := parcel.New()
	data.WriteInterfaceToken(serviceManagerDescriptor)
	data.WriteString16(serviceName)

	reply, err := inner.Transact(ctx, serviceManagerHandle, code, 0, data)
	if err != nil {
		return 0, fmt.Errorf("CheckService(%q): transact: %w", serviceName, err)
	}

	if err := binder.ReadStatus(reply); err != nil {
		return 0, fmt.Errorf("CheckService(%q): status: %w", serviceName, err)
	}

	handle, ok, err := reply.ReadNullableStrongBinder()
	if err != nil {
		return 0, fmt.Errorf("CheckService(%q): reading binder: %w", serviceName, err)
	}
	if !ok {
		return 0, fmt.Errorf("CheckService(%q): service not found", serviceName)
	}

	return handle, nil
}

// DefaultAPILevel is the API level that the compiled proxy code was
// generated against. Set by generated code (codes_gen.go).
var DefaultAPILevel int

// Tables holds multi-version transaction code tables.
// Populated by generated code (codes_gen.go).
var Tables = MultiVersionTable{}

// Revisions maps API level -> list of version IDs (latest first).
// Populated by generated code (codes_gen.go).
var Revisions = APIRevisions{}

// --- Delegate all Transport methods to inner ---

func (t *Transport) Transact(
	ctx context.Context,
	handle uint32,
	code binder.TransactionCode,
	flags binder.TransactionFlags,
	data *parcel.Parcel,
) (*parcel.Parcel, error) {
	return t.inner.Transact(ctx, handle, code, flags, data)
}

func (t *Transport) AcquireHandle(
	ctx context.Context,
	handle uint32,
) error {
	return t.inner.AcquireHandle(ctx, handle)
}

func (t *Transport) ReleaseHandle(
	ctx context.Context,
	handle uint32,
) error {
	return t.inner.ReleaseHandle(ctx, handle)
}

func (t *Transport) RequestDeathNotification(
	ctx context.Context,
	handle uint32,
	recipient binder.DeathRecipient,
) error {
	return t.inner.RequestDeathNotification(ctx, handle, recipient)
}

func (t *Transport) ClearDeathNotification(
	ctx context.Context,
	handle uint32,
	recipient binder.DeathRecipient,
) error {
	return t.inner.ClearDeathNotification(ctx, handle, recipient)
}

func (t *Transport) Close(ctx context.Context) error {
	return t.inner.Close(ctx)
}

// Verify Transport implements binder.VersionAwareTransport.
var _ binder.VersionAwareTransport = (*Transport)(nil)
