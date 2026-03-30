package binder

import "context"

// ResolveMethodSignature returns the device's parameter type descriptor
// list (DEX format) for the given method. Returns nil if the backing
// transport does not support signature resolution or the signature is
// unknown.
//
// This is a convenience wrapper that extracts the VersionAwareTransport
// from the IBinder and delegates to its ResolveMethodSignature method.
func ResolveMethodSignature(
	b IBinder,
	ctx context.Context,
	descriptor string,
	method string,
) []string {
	if b == nil {
		return nil
	}
	t := b.Transport()
	if t == nil {
		return nil
	}
	return t.ResolveMethodSignature(ctx, descriptor, method)
}

// MatchParamsToSignature determines which compiled parameters to write
// and in what order, given the device's expected method signature.
//
// compiledDescs is the DEX type descriptor for each compiled parameter
// (in the order the proxy method declares them). deviceSig is the
// device's expected parameter type descriptors.
//
// Returns a slice of compiled parameter indices in device order.
// Each element is an index into compiledDescs. If a device-expected
// type has no match among compiled params, -1 is used (the caller
// should write a zero value or skip).
//
// The matching is greedy left-to-right: for each device param type,
// the first unused compiled param with that type is selected. This
// handles the common case where params are added or removed between
// API levels but the relative order of shared types is preserved.
func MatchParamsToSignature(
	compiledDescs []string,
	deviceSig []string,
) []int {
	result := make([]int, len(deviceSig))
	used := make([]bool, len(compiledDescs))

	for di, devType := range deviceSig {
		found := false
		for ci, compType := range compiledDescs {
			if !used[ci] && compType == devType {
				result[di] = ci
				used[ci] = true
				found = true
				break
			}
		}
		if !found {
			result[di] = -1
		}
	}

	return result
}

// SignatureMatches returns true if the compiled parameter descriptors
// match the device signature exactly (same length, same types in order).
// When true, the proxy can write all params normally without adaptation.
func SignatureMatches(
	compiledDescs []string,
	deviceSig []string,
) bool {
	if len(compiledDescs) != len(deviceSig) {
		return false
	}
	for i := range compiledDescs {
		if compiledDescs[i] != deviceSig[i] {
			return false
		}
	}
	return true
}
