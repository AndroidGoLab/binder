//go:build linux

package main

import (
	"fmt"

	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/binder/versionaware"
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
