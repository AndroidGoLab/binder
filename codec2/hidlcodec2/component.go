package hidlcodec2

import (
	"context"
	"encoding/binary"
	"fmt"

	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/hwparcel"
	"github.com/AndroidGoLab/binder/hwservicemanager"
	"github.com/AndroidGoLab/binder/logger"
)

const (
	componentFQName = "android.hardware.media.c2@1.0::IComponent"
)

// Component is a HIDL client for IComponent on hwbinder.
type Component struct {
	transport binder.Transport
	handle    uint32
}

// Handle returns the binder handle for this component.
func (c *Component) Handle() uint32 {
	return c.handle
}

// Start calls IComponent::start().
func (c *Component) Start(
	ctx context.Context,
) (_err error) {
	logger.Tracef(ctx, "hidlcodec2.Component.Start")
	defer func() { logger.Tracef(ctx, "/hidlcodec2.Component.Start: %v", _err) }()

	hp := hwparcel.New()
	hp.WriteInterfaceToken(componentFQName)

	reply, err := hwservicemanager.TransactHidl(
		ctx, c.transport, c.handle, transactionStart, hp,
	)
	if err != nil {
		return fmt.Errorf("start transaction: %w", err)
	}
	defer reply.Recycle()

	return parseStatusOnlyResponse(reply.Data(), "start")
}

// Stop calls IComponent::stop().
func (c *Component) Stop(
	ctx context.Context,
) (_err error) {
	logger.Tracef(ctx, "hidlcodec2.Component.Stop")
	defer func() { logger.Tracef(ctx, "/hidlcodec2.Component.Stop: %v", _err) }()

	hp := hwparcel.New()
	hp.WriteInterfaceToken(componentFQName)

	reply, err := hwservicemanager.TransactHidl(
		ctx, c.transport, c.handle, transactionStop, hp,
	)
	if err != nil {
		return fmt.Errorf("stop transaction: %w", err)
	}
	defer reply.Recycle()

	return parseStatusOnlyResponse(reply.Data(), "stop")
}

// Reset calls IComponent::reset().
func (c *Component) Reset(
	ctx context.Context,
) (_err error) {
	logger.Tracef(ctx, "hidlcodec2.Component.Reset")
	defer func() { logger.Tracef(ctx, "/hidlcodec2.Component.Reset: %v", _err) }()

	hp := hwparcel.New()
	hp.WriteInterfaceToken(componentFQName)

	reply, err := hwservicemanager.TransactHidl(
		ctx, c.transport, c.handle, transactionReset, hp,
	)
	if err != nil {
		return fmt.Errorf("reset transaction: %w", err)
	}
	defer reply.Recycle()

	return parseStatusOnlyResponse(reply.Data(), "reset")
}

// Release calls IComponent::release().
func (c *Component) Release(
	ctx context.Context,
) (_err error) {
	logger.Tracef(ctx, "hidlcodec2.Component.Release")
	defer func() { logger.Tracef(ctx, "/hidlcodec2.Component.Release: %v", _err) }()

	hp := hwparcel.New()
	hp.WriteInterfaceToken(componentFQName)

	reply, err := hwservicemanager.TransactHidl(
		ctx, c.transport, c.handle, transactionRelease, hp,
	)
	if err != nil {
		return fmt.Errorf("release transaction: %w", err)
	}
	defer reply.Recycle()

	return parseStatusOnlyResponse(reply.Data(), "release")
}

// GetInterface calls IComponent::getInterface() and returns the
// IComponentInterface handle.
func (c *Component) GetInterface(
	ctx context.Context,
) (_ *ComponentInterface, _err error) {
	logger.Tracef(ctx, "hidlcodec2.Component.GetInterface")
	defer func() { logger.Tracef(ctx, "/hidlcodec2.Component.GetInterface: %v", _err) }()

	hp := hwparcel.New()
	hp.WriteInterfaceToken(componentFQName)

	reply, err := hwservicemanager.TransactHidl(
		ctx, c.transport, c.handle, transactionGetInterface, hp,
	)
	if err != nil {
		return nil, fmt.Errorf("getInterface transaction: %w", err)
	}
	defer reply.Recycle()

	data := reply.Data()
	if len(data) < 28 {
		return nil, fmt.Errorf("getInterface response too short: %d bytes", len(data))
	}

	hidlStatus := int32(binary.LittleEndian.Uint32(data[0:]))
	if hidlStatus != 0 {
		return nil, fmt.Errorf("HIDL status error: %d", hidlStatus)
	}

	ifaceHandle, err := readBinderHandle(data[4:])
	if err != nil {
		return nil, fmt.Errorf("reading interface binder: %w", err)
	}

	return &ComponentInterface{
		transport: c.transport,
		handle:    ifaceHandle,
	}, nil
}

// Queue calls IComponent::queue() with the given WorkBundle.
// The workBundle is serialized as a HIDL WorkBundle struct.
func (c *Component) Queue(
	ctx context.Context,
	wb *WorkBundle,
) (_err error) {
	logger.Tracef(ctx, "hidlcodec2.Component.Queue")
	defer func() { logger.Tracef(ctx, "/hidlcodec2.Component.Queue: %v", _err) }()

	hp := hwparcel.New()
	hp.WriteInterfaceToken(componentFQName)

	wb.WriteTo(hp)

	reply, err := hwservicemanager.TransactHidl(
		ctx, c.transport, c.handle, transactionQueue, hp,
	)
	if err != nil {
		return fmt.Errorf("queue transaction: %w", err)
	}
	defer reply.Recycle()

	return parseStatusOnlyResponse(reply.Data(), "queue")
}

// Flush calls IComponent::flush() and returns the flushed WorkBundle.
// For simplicity, the returned WorkBundle is not fully parsed; we only
// check the status.
func (c *Component) Flush(
	ctx context.Context,
) (_err error) {
	logger.Tracef(ctx, "hidlcodec2.Component.Flush")
	defer func() { logger.Tracef(ctx, "/hidlcodec2.Component.Flush: %v", _err) }()

	hp := hwparcel.New()
	hp.WriteInterfaceToken(componentFQName)

	reply, err := hwservicemanager.TransactHidl(
		ctx, c.transport, c.handle, transactionFlush, hp,
	)
	if err != nil {
		return fmt.Errorf("flush transaction: %w", err)
	}
	defer reply.Recycle()

	data := reply.Data()
	if len(data) < 8 {
		return fmt.Errorf("flush response too short: %d bytes", len(data))
	}

	hidlStatus := int32(binary.LittleEndian.Uint32(data[0:]))
	if hidlStatus != 0 {
		return fmt.Errorf("flush HIDL status error: %d", hidlStatus)
	}

	c2Status := Status(int32(binary.LittleEndian.Uint32(data[4:])))
	return c2Status.Err()
}

// Drain calls IComponent::drain(withEos).
func (c *Component) Drain(
	ctx context.Context,
	withEos bool,
) (_err error) {
	logger.Tracef(ctx, "hidlcodec2.Component.Drain(%v)", withEos)
	defer func() { logger.Tracef(ctx, "/hidlcodec2.Component.Drain: %v", _err) }()

	hp := hwparcel.New()
	hp.WriteInterfaceToken(componentFQName)
	hp.WriteBool(withEos)

	reply, err := hwservicemanager.TransactHidl(
		ctx, c.transport, c.handle, transactionDrain, hp,
	)
	if err != nil {
		return fmt.Errorf("drain transaction: %w", err)
	}
	defer reply.Recycle()

	return parseStatusOnlyResponse(reply.Data(), "drain")
}

// parseStatusOnlyResponse parses a reply that contains only
// HIDL status + C2 Status.
func parseStatusOnlyResponse(data []byte, method string) error {
	if len(data) < 8 {
		return fmt.Errorf("%s response too short: %d bytes", method, len(data))
	}

	hidlStatus := int32(binary.LittleEndian.Uint32(data[0:]))
	if hidlStatus != 0 {
		return fmt.Errorf("%s HIDL status error: %d", method, hidlStatus)
	}

	c2Status := Status(int32(binary.LittleEndian.Uint32(data[4:])))
	if err := c2Status.Err(); err != nil {
		return fmt.Errorf("%s: %w", method, err)
	}

	return nil
}
