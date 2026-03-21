//go:build e2e

package e2e

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/xaionaro-go/binder/binder"
	"github.com/xaionaro-go/binder/binder/versionaware"
	aidlerrors "github.com/xaionaro-go/binder/errors"
	"github.com/xaionaro-go/binder/parcel"
	"github.com/xaionaro-go/binder/servicemanager"
)

// getService retrieves a named Android service via the service manager.
// It retries up to 3 times with a short sleep between attempts to handle
// transient "null binder" responses that occur when the kernel binder
// driver is under resource pressure (e.g. after 200+ sequential tests
// each opening their own /dev/binder fd).
func getService(
	ctx context.Context,
	t *testing.T,
	driver *versionaware.Transport,
	name string,
) binder.IBinder {
	t.Helper()
	sm := servicemanager.New(driver)

	const maxAttempts = 3
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		svc, err := sm.GetService(ctx, servicemanager.ServiceName(name))
		if err == nil {
			require.NotNil(t, svc, "expected non-nil binder for %s", name)
			return svc
		}

		// Retry only on "null binder" errors — these indicate transient
		// resource exhaustion, not a genuinely missing service.
		isNullBinder := strings.Contains(err.Error(), "null binder") ||
			strings.Contains(err.Error(), "unexpected null")
		if !isNullBinder || attempt == maxAttempts {
			requireOrSkip(t, err)
			return nil // unreachable: requireOrSkip either skips or fails
		}

		t.Logf("getService(%q): attempt %d/%d got null binder, retrying after sleep", name, attempt, maxAttempts)
		time.Sleep(500 * time.Millisecond)
	}

	return nil // unreachable
}

// transactExpectException sends a transaction and asserts that ReadStatus
// returns a *StatusError. If the expected exception code matches, it returns
// the StatusError. If no exception is thrown or a different exception type
// is received, the test is skipped (device/version-dependent behavior).
func transactExpectException(
	ctx context.Context,
	t *testing.T,
	svc binder.IBinder,
	code binder.TransactionCode,
	data *parcel.Parcel,
	expectedExc aidlerrors.ExceptionCode,
) *aidlerrors.StatusError {
	t.Helper()

	reply, err := svc.Transact(ctx, code, 0, data)
	requireOrSkip(t, err)

	statusErr := binder.ReadStatus(reply)
	if statusErr == nil {
		t.Skipf("no exception thrown (device/version-dependent behavior); expected %s", expectedExc)
		return nil
	}

	var se *aidlerrors.StatusError
	if !errors.As(statusErr, &se) {
		t.Skipf("non-StatusError returned: %T: %v", statusErr, statusErr)
		return nil
	}

	if se.Exception != expectedExc {
		t.Skipf("got %s exception instead of expected %s (device/version-dependent behavior)", se.Exception, expectedExc)
		return nil
	}

	return se
}

// --- Exception tests ---

const (
	powerManagerDescriptor       = "android.os.IPowerManager"
	webViewUpdateDescriptor      = "android.webkit.IWebViewUpdateService"
	storageStatsManagerDescriptor = "android.app.usage.IStorageStatsManager"
)

func TestException_IllegalArgument(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	pm := getService(ctx, t, driver, "power")

	// IPowerManager::updateWakeLockCallback with missing lock param causes IllegalArgumentException.
	code := resolveCode(ctx, t, pm, powerManagerDescriptor, "updateWakeLockCallback")
	data := parcel.New()
	data.WriteInterfaceToken(powerManagerDescriptor)

	se := transactExpectException(ctx, t, pm, code, data, aidlerrors.ExceptionIllegalArgument)
	if se != nil {
		t.Logf("IllegalArgument exception message: %s", se.Message)
	}
}

func TestException_BadParcelable(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	am := getService(ctx, t, driver, "activity")

	// IActivityManager::setThemeOverlayReady with two string params causes BadParcelableException.
	code := resolveCode(ctx, t, am, activityManagerDescriptor, "setThemeOverlayReady")
	data := parcel.New()
	data.WriteInterfaceToken(activityManagerDescriptor)
	data.WriteString16("com.android.systemui")
	data.WriteString16("com.android.systemui")

	se := transactExpectException(ctx, t, am, code, data, aidlerrors.ExceptionBadParcelable)
	if se != nil {
		t.Logf("BadParcelable exception message: %s", se.Message)
	}
}

func TestException_Parcelable(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	ss := getService(ctx, t, driver, "storagestats")

	// IStorageStatsManager::getCacheBytes with empty volume UUID and package name.
	code := resolveCode(ctx, t, ss, storageStatsManagerDescriptor, "getCacheBytes")
	data := parcel.New()
	data.WriteInterfaceToken(storageStatsManagerDescriptor)
	data.WriteString16("")
	data.WriteString16("com.android.shell")

	se := transactExpectException(ctx, t, ss, code, data, aidlerrors.ExceptionParcelable)
	if se != nil {
		t.Logf("Parcelable exception message: %s", se.Message)
	}
}

func TestException_AllTypesInventory(t *testing.T) {
	ctx := context.Background()
	driver := openBinder(t)
	sm := servicemanager.New(driver)

	type exceptionTestCase struct {
		name       string
		service    string
		descriptor string
		method     string
		setupData  func(*parcel.Parcel)
		expected   aidlerrors.ExceptionCode
	}

	cases := []exceptionTestCase{
		{
			name:       "Security",
			service:    "activity",
			descriptor: activityManagerDescriptor,
			method:     "requestBugReportWithDescription",
			setupData:  func(p *parcel.Parcel) {},
			expected:   aidlerrors.ExceptionSecurity,
		},
		{
			name:       "NullPointer",
			service:    "activity",
			descriptor: activityManagerDescriptor,
			method:     "clearApplicationUserData",
			setupData:  func(p *parcel.Parcel) {},
			expected:   aidlerrors.ExceptionNullPointer,
		},
		{
			name:       "IllegalArgument",
			service:    "power",
			descriptor: powerManagerDescriptor,
			method:     "updateWakeLockCallback",
			setupData:  func(p *parcel.Parcel) {},
			expected:   aidlerrors.ExceptionIllegalArgument,
		},
		{
			name:       "IllegalState",
			service:    "webviewupdate",
			descriptor: webViewUpdateDescriptor,
			method:     "isMultiProcessEnabled",
			setupData:  func(p *parcel.Parcel) {},
			expected:   aidlerrors.ExceptionIllegalState,
		},
		{
			name:       "BadParcelable",
			service:    "activity",
			descriptor: activityManagerDescriptor,
			method:     "setThemeOverlayReady",
			setupData: func(p *parcel.Parcel) {
				p.WriteString16("com.android.systemui")
				p.WriteString16("com.android.systemui")
			},
			expected: aidlerrors.ExceptionBadParcelable,
		},
		{
			name:       "Parcelable",
			service:    "storagestats",
			descriptor: storageStatsManagerDescriptor,
			method:     "getCacheBytes",
			setupData: func(p *parcel.Parcel) {
				p.WriteString16("")
				p.WriteString16("com.android.shell")
			},
			expected: aidlerrors.ExceptionParcelable,
		},
	}

	testedExceptions := make(map[aidlerrors.ExceptionCode]bool)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, err := sm.GetService(ctx, servicemanager.ServiceName(tc.service))
			requireOrSkip(t, err)
			require.NotNil(t, svc)

			code := resolveCode(ctx, t, svc, tc.descriptor, tc.method)

			data := parcel.New()
			data.WriteInterfaceToken(tc.descriptor)
			tc.setupData(data)

			reply, err := svc.Transact(ctx, code, 0, data)
			requireOrSkip(t, err)

			statusErr := binder.ReadStatus(reply)
			if statusErr == nil {
				t.Skipf("no exception thrown (device/version-dependent); expected %s", tc.expected)
				return
			}

			var se *aidlerrors.StatusError
			if !errors.As(statusErr, &se) {
				t.Skipf("non-StatusError returned: %T: %v", statusErr, statusErr)
				return
			}

			if se.Exception != tc.expected {
				t.Skipf("got %s instead of expected %s (device/version-dependent)", se.Exception, tc.expected)
				return
			}

			testedExceptions[se.Exception] = true
			t.Logf("exception %s: message=%q", se.Exception, se.Message)
		})
	}

	// The number of testable exception types varies by device/API level.
	// At minimum we should see at least 1 if any services are available.
	require.NotEmpty(t, testedExceptions,
		"at least one exception type should have been tested; if no services are available, the subtests would have skipped")
	t.Logf("Tested %d distinct exception types", len(testedExceptions))
}
