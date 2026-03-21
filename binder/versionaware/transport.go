package versionaware

import (
	"bytes"
	"context"
	"debug/elf"
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/facebookincubator/go-belt/tool/logger"
	"github.com/xaionaro-go/binder/binder"
	"github.com/xaionaro-go/binder/binder/versionaware/dex"
	"github.com/xaionaro-go/binder/parcel"
)

const (
	serviceManagerHandle      = uint32(0)
	serviceManagerDescriptor  = "android.os.IServiceManager"
	activityManagerDescriptor = "android.app.IActivityManager"
)

// frameworkJARDir is the directory containing framework JARs
// with AIDL $Stub classes and TRANSACTION_* constants.
const frameworkJARDir = "/system/framework"

// Transport wraps a binder.Transport and adds version-aware
// transaction code resolution via ResolveCode.
//
// All version detection (API level + revision probing) happens
// eagerly in NewTransport. If the device version cannot be
// determined or is unsupported, NewTransport returns an error.
type Transport struct {
	inner     binder.Transport
	apiLevel  int
	table     VersionTable
	version   string
	cachePath string
	// Revision is the detected AOSP firmware revision (e.g. "36.r4").
	// Set during NewTransport when libbinder.so symbol inspection
	// narrows the candidates to exactly one. Empty if ambiguous.
	Revision Revision
	// ScannedJARs tracks which framework JARs have been fully scanned
	// for $Stub classes. Keyed by JAR filename (e.g. "framework.jar").
	// Persisted in the cache to avoid re-scanning across runs.
	ScannedJARs map[string]bool
	// mu protects table and ScannedJARs from concurrent read+write
	// during lazy descriptor extraction. RLock for reads (ResolveCode
	// fast path), Lock for writes (lazyExtractDescriptor). Required
	// because Go maps are not safe for concurrent access.
	mu sync.RWMutex
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

	// Detect revision from libbinder.so symbols (cheap: single file
	// read, no binder transactions). Done before cache loading so
	// the revision is available for compiled-table lookups.
	var revision Revision
	revisions := filterRevisionsBySOMethodSet(Revisions[targetAPI])
	if len(revisions) == 1 {
		revision = revisions[0]
		logger.Debugf(ctx, "versionaware: detected revision %s from libbinder.so symbols", revision)
	}

	// Try loading from cache if configured.
	if cfg.CachePath != "" {
		fingerprint := resolvedTableFingerprint(targetAPI, revision)
		cached := loadCachedTable(cfg.CachePath, fingerprint)
		if cached != nil {
			logger.Debugf(ctx, "versionaware: loaded cached transaction codes from %s (%d interfaces, %d scanned JARs)", cfg.CachePath, len(cached.ResolvedTable), len(cached.ScannedJARs))
			scannedJARs := make(map[string]bool, len(cached.ScannedJARs))
			for _, jar := range cached.ScannedJARs {
				scannedJARs[jar] = true
			}
			return &Transport{
				inner:       inner,
				apiLevel:    targetAPI,
				table:       cached.ResolvedTable,
				version:     fmt.Sprintf("%d.cached", targetAPI),
				cachePath:   cfg.CachePath,
				Revision:    revision,
				ScannedJARs: scannedJARs,
			}, nil
		}
	}

	// If framework JARs are readable, skip upfront extraction entirely.
	// Individual interfaces are extracted on demand by ResolveCode's lazy
	// path: framework interfaces from DEX, HAL interfaces from compiled
	// tables. This avoids the 90ms cold-run cost of scanning all 38 JARs.
	if frameworkJARsAvailable() {
		version := fmt.Sprintf("%d.device", targetAPI)
		logger.Debugf(ctx, "versionaware: framework JARs available; interfaces will be extracted on demand")
		return &Transport{
			inner:       inner,
			apiLevel:    targetAPI,
			table:       VersionTable{},
			version:     version,
			cachePath:   cfg.CachePath,
			Revision:    revision,
			ScannedJARs: map[string]bool{},
		}, nil
	}

	// Framework JARs not available — resolve a full table from compiled
	// version tables with revision detection.
	table, version, err := resolveTable(ctx, inner, targetAPI)
	if err != nil {
		return nil, err
	}

	// Save to cache if configured.
	if cfg.CachePath != "" {
		fingerprint := resolvedTableFingerprint(targetAPI, revision)
		saveCachedTable(ctx, cfg.CachePath, fingerprint, table, nil)
		logger.Debugf(ctx, "versionaware: cached resolved transaction codes to %s", cfg.CachePath)
	}

	return &Transport{
		inner:       inner,
		apiLevel:    targetAPI,
		table:       table,
		version:     version,
		cachePath:   cfg.CachePath,
		Revision:    revision,
		ScannedJARs: map[string]bool{},
	}, nil
}

// resolveTable builds a version table from compiled AIDL snapshots.
// Used when framework JARs are not available (e.g., non-Android host).
// Determines the correct revision via libbinder.so symbol inspection
// and/or binder transaction probing.
func resolveTable(
	ctx context.Context,
	inner binder.Transport,
	targetAPI int,
) (VersionTable, string, error) {
	logger.Warnf(ctx, "versionaware: framework JARs not available or unreadable at %s; falling back to compiled version tables (transaction codes may be inaccurate)", frameworkJARDir)

	revisions := Revisions[targetAPI]
	if len(revisions) == 0 {
		return nil, "", fmt.Errorf("versionaware: API level %d is not supported and framework JARs not readable; supported API levels: %v", targetAPI, supportedAPILevels())
	}

	// Narrow revision candidates by checking which methods exist in
	// libbinder.so's BpServiceManager. This distinguishes old-style
	// (no getService2/checkService2) from new-style method ordering
	// without any binder transactions.
	revisions = filterRevisionsBySOMethodSet(revisions)

	var version Revision
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
	return table, string(version), nil
}

// resolvedTableFingerprint returns a fingerprint that changes when
// the device's firmware changes, invalidating cached transaction codes.
// Combines API level, detected revision, and framework JAR fingerprint.
func resolvedTableFingerprint(apiLevel int, revision Revision) string {
	fp := frameworkFingerprint()
	return fmt.Sprintf("api=%d;rev=%s;jars=%s", apiLevel, revision, fp)
}

// MergeTable adds entries from extra into the transport's version table.
// Existing entries are not overwritten. This is useful for registering
// stable AIDL HAL interfaces whose transaction codes are not extracted
// from framework JARs.
func (t *Transport) MergeTable(
	extra VersionTable,
) {
	t.mu.Lock()
	defer t.mu.Unlock()

	for descriptor, methods := range extra {
		if t.table[descriptor] == nil {
			t.table[descriptor] = make(map[string]binder.TransactionCode)
		}
		for method, code := range methods {
			if _, exists := t.table[descriptor][method]; !exists {
				t.table[descriptor][method] = code
			}
		}
	}
}

// ResolveCode resolves an AIDL method name to the correct transaction code
// for the detected device version.
//
// If the descriptor is not in the pre-loaded table, ResolveCode attempts
// on-demand extraction from the device's framework JARs. This handles
// cases where the cache is stale or a new interface is needed that wasn't
// in the initial extraction.
//
// Returns an error if the method is not found in the version table
// (e.g., calling a method that does not exist on the device's API level).
func (t *Transport) ResolveCode(
	ctx context.Context,
	descriptor string,
	method string,
) (binder.TransactionCode, error) {
	// Fast path: descriptor already in table.
	t.mu.RLock()
	code := t.table.Resolve(descriptor, method)
	needsLazy := code == 0 && t.table[descriptor] == nil
	t.mu.RUnlock()

	if code != 0 {
		return code, nil
	}

	// If the entire descriptor is missing (not just the method),
	// try lazy extraction from device JARs.
	if needsLazy {
		code = t.lazyExtractDescriptor(ctx, descriptor, method)
		if code != 0 {
			return code, nil
		}
	}

	return 0, fmt.Errorf("versionaware: method %s.%s not found in version %s", descriptor, method, t.version)
}

// lazyExtractDescriptor attempts on-demand extraction of a single
// interface descriptor. Uses a two-phase procedure:
//
// Phase 1 scans ALL unscanned JARs in /system/framework/, merging
// every discovered $Stub class into the table. This is done in full
// (no early stop) so that subsequent lookups for other descriptors
// hit the fast path. After scanning, checks if the descriptor is now
// in the table.
//
// Phase 2 falls back to compiled version tables, using t.Revision
// if available for an exact match, otherwise iterating all revisions.
func (t *Transport) lazyExtractDescriptor(
	ctx context.Context,
	descriptor string,
	method string,
) binder.TransactionCode {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Double-check after acquiring write lock: another goroutine may
	// have already extracted this descriptor.
	if code := t.table.Resolve(descriptor, method); code != 0 {
		return code
	}
	if t.table[descriptor] != nil {
		return 0 // descriptor present but method missing — no retry
	}

	modified := false

	// Phase 1: scan all unscanned JARs in /system/framework/.
	entries, readErr := os.ReadDir(frameworkJARDir)
	if readErr == nil {
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jar") {
				continue
			}
			if t.ScannedJARs[entry.Name()] {
				continue
			}

			jarPath := frameworkJARDir + "/" + entry.Name()
			allCodes, err := dex.ExtractFromJAR(jarPath)
			if err != nil {
				logger.Debugf(ctx, "versionaware: extracting from %s: %v", entry.Name(), err)
				t.ScannedJARs[entry.Name()] = true
				modified = true
				continue
			}

			for iface, codes := range allCodes {
				if t.table[iface] == nil {
					t.table[iface] = make(map[string]binder.TransactionCode, len(codes))
				}
				for m, c := range codes {
					t.table[iface][m] = binder.TransactionCode(c)
				}
			}
			t.ScannedJARs[entry.Name()] = true
			modified = true
		}
	}

	// Check if Phase 1 found the descriptor.
	if code := t.table.Resolve(descriptor, method); code != 0 {
		if modified {
			t.saveCache(ctx)
		}
		return code
	}
	if t.table[descriptor] != nil {
		// Descriptor found but method missing — save and return.
		if modified {
			t.saveCache(ctx)
		}
		return 0
	}

	// Phase 2: fall back to compiled version tables.
	logger.Debugf(ctx, "versionaware: %s not in JARs, trying compiled tables", descriptor)
	codes := t.lookupCompiledDescriptor(descriptor)
	if codes != nil {
		logger.Debugf(ctx, "versionaware: lazy extracted %s from compiled tables (%d methods)", descriptor, len(codes))
		t.table[descriptor] = make(map[string]binder.TransactionCode, len(codes))
		for m, c := range codes {
			t.table[descriptor][m] = binder.TransactionCode(c)
		}
		t.saveCache(ctx)
		return t.table.Resolve(descriptor, method)
	}

	// Not found in any source. Cache negative result so subsequent
	// calls skip the expensive lookup and fall back to the hardcoded
	// transaction code in the generated proxy.
	logger.Debugf(ctx, "versionaware: %s not found in any source", descriptor)
	t.table[descriptor] = make(map[string]binder.TransactionCode)
	if modified {
		t.saveCache(ctx)
	}
	return 0
}

// saveCache persists the current table and scanned JAR list to disk.
// Must be called with t.mu held.
func (t *Transport) saveCache(ctx context.Context) {
	if t.cachePath == "" {
		return
	}
	fingerprint := resolvedTableFingerprint(t.apiLevel, t.Revision)
	scannedJARs := make([]string, 0, len(t.ScannedJARs))
	for jar := range t.ScannedJARs {
		scannedJARs = append(scannedJARs, jar)
	}
	saveCachedTable(ctx, t.cachePath, fingerprint, t.table, scannedJARs)
	logger.Debugf(ctx, "versionaware: cache updated (%d interfaces, %d scanned JARs)", len(t.table), len(t.ScannedJARs))
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

// lookupCompiledDescriptor searches compiled version tables for a single
// descriptor. If t.Revision is set, only that revision is checked;
// otherwise all revisions for the API level are tried (first match wins).
// Used as a fallback for interfaces not found in framework JARs
// (e.g., HAL interfaces).
func (t *Transport) lookupCompiledDescriptor(
	descriptor string,
) dex.TransactionCodes {
	// Fast path: exact revision known.
	if t.Revision != "" {
		compiled, ok := Tables[t.Revision]
		if !ok {
			return nil
		}
		methods := compiled[descriptor]
		if methods == nil {
			return nil
		}
		codes := make(dex.TransactionCodes, len(methods))
		for m, c := range methods {
			codes[m] = uint32(c)
		}
		return codes
	}

	// Slow path: iterate all revisions for the API level.
	// If the detected API level has no tables, fall back to
	// DefaultAPILevel's tables (the codes were generated from
	// the same AIDL sources as the proxy code).
	for _, level := range []int{t.apiLevel, DefaultAPILevel} {
		for _, rev := range Revisions[level] {
			compiled, ok := Tables[rev]
			if !ok {
				continue
			}
			methods := compiled[descriptor]
			if methods == nil {
				continue
			}
			codes := make(dex.TransactionCodes, len(methods))
			for m, c := range methods {
				codes[m] = uint32(c)
			}
			return codes
		}
	}
	return nil
}

// frameworkJARsAvailable returns true if /system/framework/ is readable
// and contains at least one .jar file. This is a cheap check (readdir only,
// no ZIP parsing) to decide whether lazy DEX extraction is possible.
func frameworkJARsAvailable() bool {
	entries, err := os.ReadDir(frameworkJARDir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".jar") {
			return true
		}
	}
	return false
}

// frameworkFingerprint returns a string identifying the current set of
// framework JARs by their names and sizes. Changes when the OS is updated.
func frameworkFingerprint() string {
	entries, err := os.ReadDir(frameworkJARDir)
	if err != nil {
		// Return a distinct marker so off-device caches don't
		// collide with on-device caches that have real JAR data.
		return "no-jars"
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
	if b.Len() == 0 {
		return "no-jars"
	}
	return b.String()
}

// loadCachedTable reads a cached table from the given path.
// Returns nil if cache is missing, corrupted, or fingerprint doesn't match.
// The returned cachedTable contains both the VersionTable and the list
// of previously scanned JARs.
func loadCachedTable(
	cachePath string,
	fingerprint string,
) *cachedTable {
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil
	}

	var cached cachedTable
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(&cached); err != nil {
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
	return &cachedTable{
		Fingerprint:   cached.Fingerprint,
		ResolvedTable: table,
		ScannedJARs:   cached.ScannedJARs,
	}
}

// saveCachedTable writes a VersionTable and scanned JAR list to the given path.
// Uses atomic write (temp file + rename) to avoid corrupted reads.
func saveCachedTable(
	ctx context.Context,
	cachePath string,
	fingerprint string,
	table VersionTable,
	scannedJARs []string,
) {
	dir := filepath.Dir(cachePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		logger.Warnf(ctx, "versionaware: saveCachedTable: MkdirAll(%s): %v", dir, err)
		return
	}

	raw := make(map[string]map[string]uint32)
	for desc, methods := range table {
		raw[desc] = make(map[string]uint32)
		for method, code := range methods {
			raw[desc][method] = uint32(code)
		}
	}

	cached := cachedTable{
		Fingerprint: fingerprint,
		Table:       raw,
		ScannedJARs: scannedJARs,
	}

	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(cached); err != nil {
		logger.Warnf(ctx, "versionaware: saveCachedTable: gob encode: %v", err)
		return
	}

	tmpPath := cachePath + ".tmp"
	if err := os.WriteFile(tmpPath, buf.Bytes(), 0o644); err != nil {
		logger.Warnf(ctx, "versionaware: saveCachedTable: WriteFile(%s): %v", tmpPath, err)
		return
	}
	if err := os.Rename(tmpPath, cachePath); err != nil {
		logger.Warnf(ctx, "versionaware: saveCachedTable: Rename(%s -> %s): %v", tmpPath, cachePath, err)
	}
}

// filterRevisionsBySOMethodSet reads BpServiceManager symbols from
// /system/lib64/libbinder.so to determine which methods exist on
// the device, then filters revision candidates to those whose method
// set matches.
func filterRevisionsBySOMethodSet(revisions []Revision) []Revision {
	deviceMethods := readBpServiceManagerMethods()
	if len(deviceMethods) == 0 {
		return revisions // can't read .so, don't filter
	}

	var filtered []Revision
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

	// Return the method name as-is from the mangled symbol.
	// AIDL method names may start with uppercase (e.g., "GetApInterfaces",
	// "SendMgmtFrame", "Continue") so we must not force lowercase.
	return rest[i : i+nameLen]
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
) (Revision, error) {
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
	defer func() {
		if releaseErr := inner.ReleaseHandle(ctx, activityHandle); releaseErr != nil {
			logger.Debugf(ctx, "probeRevision: failed to release activity handle %d: %v", activityHandle, releaseErr)
		}
	}()

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
		data.Recycle()
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
	data.Recycle()
	if err != nil {
		return 0, fmt.Errorf("CheckService(%q): transact: %w", serviceName, err)
	}
	defer reply.Recycle()

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

func (t *Transport) RegisterReceiver(
	ctx context.Context,
	receiver binder.TransactionReceiver,
) uintptr {
	return t.inner.RegisterReceiver(ctx, receiver)
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
