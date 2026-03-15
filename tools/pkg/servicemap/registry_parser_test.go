package servicemap

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractRegistrations(t *testing.T) {
	src := `class SystemServiceRegistry {
    static {
        registerService(Context.ACCOUNT_SERVICE, AccountManager.class,
                new CachedServiceFetcher<AccountManager>() {
            @Override
            public AccountManager createService(ContextImpl ctx) throws ServiceNotFoundException {
                IBinder b = ServiceManager.getServiceOrThrow(Context.ACCOUNT_SERVICE);
                IAccountManager service = IAccountManager.Stub.asInterface(b);
                return new AccountManager(ctx, service);
            }});

        registerService(Context.ACTIVITY_SERVICE, ActivityManager.class,
                new CachedServiceFetcher<ActivityManager>() {
            @Override
            public ActivityManager createService(ContextImpl ctx) {
                return new ActivityManager(ctx.getOuterContext(), ctx.mMainThread.getHandler());
            }});
    }
}`
	regs := ExtractRegistrations(src)
	// Should find ACCOUNT_SERVICE but skip ACTIVITY_SERVICE (no Stub.asInterface)
	require.Len(t, regs, 1)
	require.Equal(t, "ACCOUNT_SERVICE", regs[0].ContextConstant)
	require.Equal(t, "IAccountManager", regs[0].AIDLInterface)
}

func TestExtractRegistrationsReal(t *testing.T) {
	src, err := os.ReadFile("../3rdparty/frameworks-base/core/java/android/app/SystemServiceRegistry.java")
	if err != nil {
		t.Skip("3rdparty submodules not available")
	}

	regs := ExtractRegistrations(string(src))
	require.Greater(t, len(regs), 70, "should find at least 70 binder-backed service registrations")

	// Verify known mappings
	found := map[string]string{}
	for _, r := range regs {
		found[r.ContextConstant] = r.AIDLInterface
	}
	require.Equal(t, "IPowerManager", found["POWER_SERVICE"])
	require.Equal(t, "IAccountManager", found["ACCOUNT_SERVICE"])
}
