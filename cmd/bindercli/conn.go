//go:build linux

package main

import (
	"github.com/AndroidGoLab/binder/cmd/bindercli/conn"
)

// Conn is an alias for conn.Conn for use within package main.
type Conn = conn.Conn

// OpenConn is a convenience alias for conn.Open.
var OpenConn = conn.Open
