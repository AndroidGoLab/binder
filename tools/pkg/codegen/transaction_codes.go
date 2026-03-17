package codegen

import (
	"github.com/xaionaro-go/binder/binder"
	"github.com/xaionaro-go/binder/tools/pkg/parser"
)

// ComputeTransactionCodes computes the binder transaction code for each
// method in an AIDL interface, following the Android AIDL transaction
// code assignment rules: sequential from FirstCallTransaction, with
// explicit TransactionID overrides resetting the counter.
func ComputeTransactionCodes(
	methods []*parser.MethodDecl,
) map[string]binder.TransactionCode {
	codes := make(map[string]binder.TransactionCode, len(methods))

	counter := 0
	for _, m := range methods {
		if m.TransactionID != 0 {
			counter = m.TransactionID - 1
		}
		codes[m.MethodName] = binder.FirstCallTransaction + binder.TransactionCode(counter)
		counter++
	}

	return codes
}
