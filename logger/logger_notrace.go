//go:build !debug_trace

package logger

import (
	"context"

	"github.com/facebookincubator/go-belt/pkg/field"
)

// TraceFields is a no-op when debug_trace build tag is not set.
func TraceFields(_ context.Context, _ string, _ field.AbstractFields) {}

// Trace is a no-op when debug_trace build tag is not set.
func Trace(_ context.Context, _ ...any) {}

// Tracef is a no-op when debug_trace build tag is not set.
func Tracef(_ context.Context, _ string, _ ...any) {}
