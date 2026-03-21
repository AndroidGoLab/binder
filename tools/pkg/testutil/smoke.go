package testutil

import (
	"context"
	"reflect"
	"testing"

	"github.com/xaionaro-go/binder/binder"
)

// skippedMethods lists infrastructure methods that are not AIDL
// service methods and should be excluded from smoke testing.
var skippedMethods = map[string]bool{
	"AsBinder":            true,
	"InterfaceDescriptor": true,
}

// SmokeOption configures SmokeTestAllMethods behavior.
type SmokeOption func(*smokeConfig)

type smokeConfig struct {
	methodFilter func(string) bool
}

// WithMethodFilter sets a predicate that decides whether a method should
// be tested. Methods for which the filter returns false are skipped.
func WithMethodFilter(filter func(string) bool) SmokeOption {
	return func(cfg *smokeConfig) {
		cfg.methodFilter = filter
	}
}

// SmokeTestAllMethods calls every exported method on proxy with
// zero-value arguments and classifies the results. Each method
// is run as a t.Run sub-test. Panics (e.g. nil interface dereference)
// are caught and counted, not propagated.
func SmokeTestAllMethods(
	t *testing.T,
	proxy any,
	opts ...SmokeOption,
) SmokeResult {
	t.Helper()

	var cfg smokeConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	v := reflect.ValueOf(proxy)
	typ := v.Type()
	var result SmokeResult

	for i := 0; i < typ.NumMethod(); i++ {
		method := typ.Method(i)
		if skippedMethods[method.Name] {
			continue
		}

		if cfg.methodFilter != nil && !cfg.methodFilter(method.Name) {
			t.Run(method.Name, func(t *testing.T) {
				t.Skipf("method %s skipped by method filter (dangerous method name)", method.Name)
			})
			continue
		}

		result.Total++
		methodName := method.Name
		methodValue := v.Method(i)
		methodType := methodValue.Type()

		t.Run(methodName, func(t *testing.T) {
			args := buildZeroArgs(methodType)

			outcome := callWithRecover(methodValue, args)
			switch outcome {
			case outcomePanicked:
				result.Panicked++
				t.Logf("method %s panicked (nil interface arg)", methodName)
			case outcomeFailed:
				result.Failed++
				t.Logf("method %s returned an error", methodName)
			case outcomePassed:
				result.Passed++
			}
		})
	}

	return result
}

type outcome int

const (
	outcomePassed  outcome = iota
	outcomePanicked
	outcomeFailed
)

// callWithRecover invokes the method and recovers from panics.
func callWithRecover(
	method reflect.Value,
	args []reflect.Value,
) (result outcome) {
	defer func() {
		if r := recover(); r != nil {
			result = outcomePanicked
		}
	}()

	results := method.Call(args)
	if len(results) == 0 {
		return outcomePassed
	}

	// Check the last return value as the error.
	lastResult := results[len(results)-1]
	if !lastResult.Type().Implements(reflect.TypeOf((*error)(nil)).Elem()) {
		return outcomePassed
	}

	if lastResult.IsNil() {
		return outcomePassed
	}

	err := lastResult.Interface().(error)
	return classifyError(err)
}

// classifyError decides whether an error means "passed" or "failed".
// A nil error means the method completed successfully (passed).
// A non-nil error means the method returned an error (failed).
func classifyError(err error) outcome {
	if err == nil {
		return outcomePassed
	}
	return outcomeFailed
}

var (
	contextType = reflect.TypeOf((*context.Context)(nil)).Elem()
	iBinderType = reflect.TypeOf((*binder.IBinder)(nil)).Elem()
)

// buildZeroArgs builds zero-value arguments for a method.
// context.Context parameters get context.Background().
// binder.IBinder parameters get a MockBinder.
// Pointer-to-struct parameters get a freshly allocated zero struct.
// Everything else gets reflect.Zero.
func buildZeroArgs(methodType reflect.Type) []reflect.Value {
	numIn := methodType.NumIn()
	args := make([]reflect.Value, numIn)

	for i := 0; i < numIn; i++ {
		argType := methodType.In(i)

		switch {
		case argType.Implements(contextType):
			args[i] = reflect.ValueOf(context.Background())
		case argType.Implements(iBinderType) || argType == iBinderType:
			args[i] = reflect.ValueOf(NewMockBinder())
		case argType.Kind() == reflect.Ptr && argType.Elem().Kind() == reflect.Struct:
			args[i] = reflect.New(argType.Elem())
		default:
			args[i] = reflect.Zero(argType)
		}
	}

	return args
}
