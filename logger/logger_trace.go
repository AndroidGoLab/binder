//go:build debug_trace

package logger

import (
	"context"

	"github.com/facebookincubator/go-belt/pkg/field"
	"github.com/facebookincubator/go-belt/tool/logger"
)

// TraceFields delegates to belt logger when debug_trace build tag is set.
func TraceFields(ctx context.Context, message string, fields field.AbstractFields) {
	logger.TraceFields(ctx, message, fields)
}

// Trace delegates to belt logger when debug_trace build tag is set.
func Trace(ctx context.Context, values ...any) {
	logger.Trace(ctx, values...)
}

// Tracef delegates to belt logger when debug_trace build tag is set.
func Tracef(ctx context.Context, format string, args ...any) {
	logger.Tracef(ctx, format, args...)
}
