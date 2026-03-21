package servicemanager

import (
	"context"
	"errors"
	"fmt"

	"github.com/xaionaro-go/binder/logger"
	"github.com/xaionaro-go/binder/binder"
	"github.com/xaionaro-go/binder/parcel"
)

const (
	serviceManagerHandle     = uint32(0)
	serviceManagerDescriptor = "android.os.IServiceManager"

	// dumpFlagPriorityAll combines all priority flags per IServiceManager.aidl:
	// DUMP_FLAG_PRIORITY_CRITICAL (1) | DUMP_FLAG_PRIORITY_HIGH (2)
	// | DUMP_FLAG_PRIORITY_NORMAL (4) | DUMP_FLAG_PRIORITY_DEFAULT (8).
	dumpFlagPriorityAll = int32(1 | 2 | 4 | 8)

	// maxServiceCount is the maximum number of services we accept from
	// ListServices replies. Guards against corrupted or malicious parcels
	// causing excessive allocations.
	maxServiceCount = int32(10000)
)

// ServiceManager provides access to Android's ServiceManager via Binder IPC.
type ServiceManager struct {
	remote   binder.IBinder
	identity binder.CallerIdentity
}

// New creates a ServiceManager client using the given transport.
func New(
	transport binder.VersionAwareTransport,
	opts ...Option,
) *ServiceManager {
	cfg := Options(opts).config()
	return &ServiceManager{
		remote:   binder.NewProxyBinder(transport, cfg.Identity, serviceManagerHandle),
		identity: cfg.Identity,
	}
}

// GetService retrieves a service by name. Blocks until the service is available.
func (sm *ServiceManager) GetService(
	ctx context.Context,
	name ServiceName,
) (_binder binder.IBinder, _err error) {
	logger.Tracef(ctx, "GetService(%q)", name)
	defer func() { logger.Tracef(ctx, "/GetService(%q): %v", name, _err) }()

	code, err := sm.remote.ResolveCode(ctx, serviceManagerDescriptor, "getService")
	if err != nil {
		return nil, fmt.Errorf("servicemanager: GetService(%q): %w", name, err)
	}

	data := parcel.New()
	defer data.Recycle()
	data.WriteInterfaceToken(serviceManagerDescriptor)
	data.WriteString16(string(name))

	reply, err := sm.remote.Transact(ctx, code, 0, data)
	if err != nil {
		return nil, fmt.Errorf("servicemanager: GetService(%q): %w", name, err)
	}
	defer reply.Recycle()

	if err := binder.ReadStatus(reply); err != nil {
		return nil, fmt.Errorf("servicemanager: GetService(%q): %w", name, err)
	}

	handle, ok, err := reply.ReadNullableStrongBinder()
	if err != nil {
		return nil, fmt.Errorf("servicemanager: GetService(%q): reading binder: %w", name, err)
	}

	if !ok {
		return nil, fmt.Errorf("servicemanager: GetService(%q): service not found", name)
	}

	transport, err := sm.transport()
	if err != nil {
		return nil, fmt.Errorf("servicemanager: GetService(%q): %w", name, err)
	}

	return binder.NewProxyBinder(transport, sm.identity, handle), nil
}

// CheckService checks if a service is registered without blocking.
// Returns nil, nil if the service is not found.
func (sm *ServiceManager) CheckService(
	ctx context.Context,
	name ServiceName,
) (_binder binder.IBinder, _err error) {
	logger.Tracef(ctx, "CheckService(%q)", name)
	defer func() { logger.Tracef(ctx, "/CheckService(%q): %v", name, _err) }()

	code, err := sm.remote.ResolveCode(ctx, serviceManagerDescriptor, "checkService")
	if err != nil {
		return nil, fmt.Errorf("servicemanager: CheckService(%q): %w", name, err)
	}

	data := parcel.New()
	defer data.Recycle()
	data.WriteInterfaceToken(serviceManagerDescriptor)
	data.WriteString16(string(name))

	reply, err := sm.remote.Transact(ctx, code, 0, data)
	if err != nil {
		return nil, fmt.Errorf("servicemanager: CheckService(%q): %w", name, err)
	}
	defer reply.Recycle()

	if err := binder.ReadStatus(reply); err != nil {
		return nil, fmt.Errorf("servicemanager: CheckService(%q): %w", name, err)
	}

	handle, ok, err := reply.ReadNullableStrongBinder()
	if err != nil {
		return nil, fmt.Errorf("servicemanager: CheckService(%q): reading binder: %w", name, err)
	}

	if !ok {
		return nil, nil
	}

	transport, err := sm.transport()
	if err != nil {
		return nil, fmt.Errorf("servicemanager: CheckService(%q): %w", name, err)
	}

	return binder.NewProxyBinder(transport, sm.identity, handle), nil
}

// ListServices returns the names of all registered services.
func (sm *ServiceManager) ListServices(
	ctx context.Context,
) (_services []ServiceName, _err error) {
	logger.Tracef(ctx, "ListServices")
	defer func() { logger.Tracef(ctx, "/ListServices: %d services, err=%v", len(_services), _err) }()

	code, err := sm.remote.ResolveCode(ctx, serviceManagerDescriptor, "listServices")
	if err != nil {
		return nil, fmt.Errorf("servicemanager: ListServices: %w", err)
	}

	data := parcel.New()
	defer data.Recycle()
	data.WriteInterfaceToken(serviceManagerDescriptor)
	data.WriteInt32(dumpFlagPriorityAll)

	reply, err := sm.remote.Transact(ctx, code, 0, data)
	if err != nil {
		return nil, fmt.Errorf("servicemanager: ListServices: %w", err)
	}
	defer reply.Recycle()

	if err := binder.ReadStatus(reply); err != nil {
		return nil, fmt.Errorf("servicemanager: ListServices: %w", err)
	}

	count, err := reply.ReadInt32()
	if err != nil {
		return nil, fmt.Errorf("servicemanager: ListServices: reading count: %w", err)
	}

	if count < 0 {
		return nil, fmt.Errorf("servicemanager: ListServices: invalid service count: %d", count)
	}

	if count > maxServiceCount {
		return nil, fmt.Errorf("servicemanager: ListServices: service count %d exceeds maximum (%d)", count, maxServiceCount)
	}

	services := make([]ServiceName, 0, count)
	for i := int32(0); i < count; i++ {
		name, err := reply.ReadString16()
		if err != nil {
			return nil, fmt.Errorf("servicemanager: ListServices: reading service %d: %w", i, err)
		}
		services = append(services, ServiceName(name))
	}

	return services, nil
}

// IsDeclared checks whether a service is declared in the VINTF manifest.
func (sm *ServiceManager) IsDeclared(
	ctx context.Context,
	name ServiceName,
) (_declared bool, _err error) {
	logger.Tracef(ctx, "IsDeclared(%q)", name)
	defer func() { logger.Tracef(ctx, "/IsDeclared(%q): %v, err=%v", name, _declared, _err) }()

	code, err := sm.remote.ResolveCode(ctx, serviceManagerDescriptor, "isDeclared")
	if err != nil {
		return false, fmt.Errorf("servicemanager: IsDeclared(%q): %w", name, err)
	}

	data := parcel.New()
	defer data.Recycle()
	data.WriteInterfaceToken(serviceManagerDescriptor)
	data.WriteString16(string(name))

	reply, err := sm.remote.Transact(ctx, code, 0, data)
	if err != nil {
		return false, fmt.Errorf("servicemanager: IsDeclared(%q): %w", name, err)
	}
	defer reply.Recycle()

	if err := binder.ReadStatus(reply); err != nil {
		return false, fmt.Errorf("servicemanager: IsDeclared(%q): %w", name, err)
	}

	val, err := reply.ReadBool()
	if err != nil {
		return false, fmt.Errorf("servicemanager: IsDeclared(%q): reading result: %w", name, err)
	}

	return val, nil
}

// AddService publishes a Go service under the given name so that other
// processes can discover it via GetService/CheckService.
func (sm *ServiceManager) AddService(
	ctx context.Context,
	name ServiceName,
	service binder.TransactionReceiver,
	allowIsolated bool,
	dumpPriority int32,
) (_err error) {
	logger.Tracef(ctx, "AddService(%q)", name)
	defer func() { logger.Tracef(ctx, "/AddService(%q): %v", name, _err) }()

	transport, err := sm.transport()
	if err != nil {
		return fmt.Errorf("servicemanager: AddService(%q): %w", name, err)
	}

	stub := binder.NewStubBinder(service)
	stub.RegisterWithTransport(ctx, transport)

	code, err := sm.remote.ResolveCode(ctx, serviceManagerDescriptor, "addService")
	if err != nil {
		return fmt.Errorf("servicemanager: AddService(%q): %w", name, err)
	}

	data := parcel.New()
	defer data.Recycle()
	data.WriteInterfaceToken(serviceManagerDescriptor)
	data.WriteString16(string(name))
	binder.WriteBinderToParcel(ctx, data, stub, transport)

	var allowIsolatedInt int32
	if allowIsolated {
		allowIsolatedInt = 1
	}
	data.WriteInt32(allowIsolatedInt)
	data.WriteInt32(dumpPriority)

	reply, err := sm.remote.Transact(ctx, code, 0, data)
	if err != nil {
		return fmt.Errorf("servicemanager: AddService(%q): %w", name, err)
	}
	defer reply.Recycle()

	if err := binder.ReadStatus(reply); err != nil {
		return fmt.Errorf("servicemanager: AddService(%q): %w", name, err)
	}

	return nil
}

// ErrNotProxyBinder is returned when the ServiceManager's remote IBinder
// is not a *binder.ProxyBinder, which prevents extracting the transport.
var ErrNotProxyBinder = errors.New("servicemanager: remote is not a *binder.ProxyBinder")

// transport extracts the VersionAwareTransport from the ProxyBinder.
func (sm *ServiceManager) transport() (binder.VersionAwareTransport, error) {
	pb, ok := sm.remote.(*binder.ProxyBinder)
	if !ok {
		return nil, ErrNotProxyBinder
	}
	return pb.Transport(), nil
}
