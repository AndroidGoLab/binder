package versionaware

import (
	"context"
	"fmt"
	"sort"

	"github.com/facebookincubator/go-belt/tool/logger"
	"github.com/xaionaro-go/aidl/binder"
	"github.com/xaionaro-go/aidl/parcel"
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
) (*Transport, error) {
	if targetAPI <= 0 {
		targetAPI = detectAPILevel()
	}
	if targetAPI <= 0 {
		return nil, fmt.Errorf("versionaware: unable to detect Android API level; pass --target-api explicitly or ensure /etc/build_flags.json is readable; supported API levels: %v", supportedAPILevels())
	}

	revisions := Revisions[targetAPI]
	if len(revisions) == 0 {
		return nil, fmt.Errorf("versionaware: API level %d is not supported; supported API levels: %v", targetAPI, supportedAPILevels())
	}

	var version string
	switch len(revisions) {
	case 1:
		version = revisions[0]
	default:
		var err error
		version, err = probeRevision(ctx, inner, targetAPI)
		if err != nil {
			return nil, fmt.Errorf("versionaware: probing revision for API %d: %w", targetAPI, err)
		}
	}

	table, ok := Tables[version]
	if !ok {
		return nil, fmt.Errorf("versionaware: no transaction code table for version %q", version)
	}

	logger.Debugf(ctx, "versionaware: detected version %s (%d interfaces)", version, len(table))

	return &Transport{
		inner:    inner,
		apiLevel: targetAPI,
		table:    table,
		version:  version,
	}, nil
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
	serviceManagerHandle     = uint32(0)
	serviceManagerDescriptor = "android.os.IServiceManager"
	activityManagerDescriptor = "android.app.IActivityManager"
)

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
// Uses the version tables to determine the correct checkService transaction
// code for the given API level.
func rawCheckService(
	ctx context.Context,
	inner binder.Transport,
	apiLevel int,
	serviceName string,
) (uint32, error) {
	// Look up the checkService transaction code from the version tables.
	// The ServiceManager's method ordering varies across API levels.
	checkServiceCode := binder.TransactionCode(0)
	for _, rev := range Revisions[apiLevel] {
		if table, ok := Tables[rev]; ok {
			checkServiceCode = table.Resolve(serviceManagerDescriptor, "checkService")
			if checkServiceCode != 0 {
				break
			}
		}
	}
	if checkServiceCode == 0 {
		return 0, fmt.Errorf("cannot determine checkService transaction code for API %d", apiLevel)
	}

	data := parcel.New()
	data.WriteInterfaceToken(serviceManagerDescriptor)
	data.WriteString16(serviceName)

	reply, err := inner.Transact(ctx, serviceManagerHandle, checkServiceCode, 0, data)
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
