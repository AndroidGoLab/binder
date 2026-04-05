//go:build linux

package main

import (
	"context"
	"fmt"

	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/binder/versionaware"
	"github.com/AndroidGoLab/binder/cmd/bindercli/discovery"
)

// resolveCodeToMethod performs reverse lookup: given a descriptor and
// transaction code, returns the method name. Returns ("", false) if not found.
func resolveCodeToMethod(
	table versionaware.VersionTable,
	descriptor string,
	code binder.TransactionCode,
) (string, bool) {
	methods, ok := table[descriptor]
	if !ok {
		return "", false
	}
	for name, c := range methods {
		if c == code {
			return name, true
		}
	}
	return "", false
}

// resolveMethodToCode performs forward lookup: given a descriptor and
// method name, returns the transaction code. Returns (0, false) if not found.
func resolveMethodToCode(
	table versionaware.VersionTable,
	descriptor string,
	method string,
) (binder.TransactionCode, bool) {
	code := table.Resolve(descriptor, method)
	if code == 0 {
		return 0, false
	}
	return code, true
}

// getActiveTable extracts the active VersionTable from a Conn's transport.
// Returns an error if the transport is not version-aware.
func getActiveTable(c *Conn) (versionaware.VersionTable, error) {
	if c == nil {
		return nil, fmt.Errorf("connection is nil")
	}
	vat, ok := c.Transport.(*versionaware.Transport)
	if !ok {
		return nil, fmt.Errorf("transport is not version-aware")
	}
	return vat.ActiveTable(), nil
}

// resolveDescriptor determines the AIDL interface descriptor for a named
// service, fetching it from the binder connection.
func resolveDescriptor(
	ctx context.Context,
	conn *Conn,
	name string,
) (string, error) {
	svc, err := conn.GetService(ctx, name)
	if err != nil {
		return "", err
	}
	return descriptorForBinder(ctx, svc, name)
}

// descriptorForBinder determines the AIDL interface descriptor for an
// already-obtained binder handle, falling back to the static
// KnownServiceNames map when InterfaceTransaction returns empty.
func descriptorForBinder(
	ctx context.Context,
	svc binder.IBinder,
	name string,
) (string, error) {
	descriptor := discovery.QueryDescriptor(ctx, svc)
	if descriptor == "" || descriptor == "(unknown)" {
		for desc, svcName := range discovery.KnownServiceNames {
			if svcName == name {
				descriptor = desc
				break
			}
		}
	}
	if descriptor == "" || descriptor == "(unknown)" {
		return "", fmt.Errorf("cannot determine interface descriptor for service %q", name)
	}
	return descriptor, nil
}

// kebabToMethod converts a kebab-case CLI name back to the camelCase
// method name using the generated registry. Returns the input unchanged
// if no match is found.
func kebabToMethod(
	query string,
	descriptor string,
) string {
	if generatedRegistry == nil {
		return query
	}
	info := generatedRegistry.ByDescriptor(descriptor)
	if info == nil {
		return query
	}
	for _, m := range info.Methods {
		if camelToKebab(m.Name) == query {
			return m.Name
		}
	}
	return query
}
