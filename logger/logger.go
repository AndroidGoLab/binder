// Package logger provides logging utilities for the binder project.
// Trace-level functions are compiled as no-ops by default for zero
// overhead on hot paths. Build with -tags=debug_trace to enable them.
package logger

import (
	"context"

	"github.com/facebookincubator/go-belt/pkg/field"
	"github.com/facebookincubator/go-belt/tool/logger"
)

// Logger is a type alias for convenience.
type Logger = logger.Logger

// Level is a type alias for convenience.
type Level = logger.Level

const (
	LevelUndefined = logger.LevelUndefined
	LevelFatal     = logger.LevelFatal
	LevelPanic     = logger.LevelPanic
	LevelError     = logger.LevelError
	LevelWarning   = logger.LevelWarning
	LevelInfo      = logger.LevelInfo
	LevelDebug     = logger.LevelDebug
	LevelTrace     = logger.LevelTrace
)

func FromCtx(ctx context.Context) logger.Logger {
	return logger.FromCtx(ctx)
}

func CtxWithLogger(ctx context.Context, l logger.Logger) context.Context {
	return logger.CtxWithLogger(ctx, l)
}

func DebugFields(ctx context.Context, message string, fields field.AbstractFields) {
	logger.DebugFields(ctx, message, fields)
}

func InfoFields(ctx context.Context, message string, fields field.AbstractFields) {
	logger.InfoFields(ctx, message, fields)
}

func WarnFields(ctx context.Context, message string, fields field.AbstractFields) {
	logger.WarnFields(ctx, message, fields)
}

func ErrorFields(ctx context.Context, message string, fields field.AbstractFields) {
	logger.ErrorFields(ctx, message, fields)
}

func Debugf(ctx context.Context, format string, args ...any) {
	logger.Debugf(ctx, format, args...)
}

func Infof(ctx context.Context, format string, args ...any) {
	logger.Infof(ctx, format, args...)
}

func Warnf(ctx context.Context, format string, args ...any) {
	logger.Warnf(ctx, format, args...)
}

func Errorf(ctx context.Context, format string, args ...any) {
	logger.Errorf(ctx, format, args...)
}

func Logf(ctx context.Context, level logger.Level, format string, args ...any) {
	logger.Logf(ctx, level, format, args...)
}
