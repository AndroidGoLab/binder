package servicemap

import (
	"regexp"

	antlr "github.com/antlr4-go/antlr/v4"

	"github.com/AndroidGoLab/binder/tools/pkg/javaparser"
)

var (
	// contextConstantRe matches "Context.XXX" and captures the constant name.
	contextConstantRe = regexp.MustCompile(`Context\.([A-Z_]+)`)

	// stubAsInterfaceRe matches "IFoo.Stub.asInterface" and captures the interface name.
	stubAsInterfaceRe = regexp.MustCompile(`([A-Za-z]\w*)\.Stub\.asInterface`)
)

// registrationListener walks a Java AST and collects service registrations
// from registerService(...) calls that contain a Stub.asInterface invocation.
type registrationListener struct {
	javaparser.BaseJavaParserListener

	Registrations []Registration
}

func newRegistrationListener() *registrationListener {
	return &registrationListener{}
}

// EnterMethodCallExpression is invoked for expressions of the form expr.methodCall(...).
// We look for top-level registerService(...) calls only (not nested ones).
func (l *registrationListener) EnterMethodCallExpression(ctx *javaparser.MethodCallExpressionContext) {
	mc := ctx.MethodCall()
	if mc == nil {
		return
	}

	id := mc.Identifier()
	if id == nil {
		return
	}

	if id.GetText() != "registerService" {
		return
	}

	// Skip nested registerService calls: only process if there is no
	// ancestor MethodCallExpression whose method is also registerService.
	if isNestedRegisterService(ctx) {
		return
	}

	text := ctx.GetText()

	contextConstant := extractContextConstant(text)
	if contextConstant == "" {
		return
	}

	aidlInterface := extractAIDLInterface(text)
	if aidlInterface == "" {
		return
	}

	l.Registrations = append(l.Registrations, Registration{
		ContextConstant: contextConstant,
		AIDLInterface:   aidlInterface,
	})
}

// EnterMethodCall handles bare registerService(...) calls that are not
// preceded by a receiver expression (i.e., not expr.registerService(...)).
func (l *registrationListener) EnterMethodCall(ctx *javaparser.MethodCallContext) {
	id := ctx.Identifier()
	if id == nil {
		return
	}

	if id.GetText() != "registerService" {
		return
	}

	// Only process if this MethodCall is NOT a child of a MethodCallExpression,
	// because those are handled by EnterMethodCallExpression.
	if _, ok := ctx.GetParent().(*javaparser.MethodCallExpressionContext); ok {
		return
	}

	text := ctx.GetText()

	contextConstant := extractContextConstant(text)
	if contextConstant == "" {
		return
	}

	aidlInterface := extractAIDLInterface(text)
	if aidlInterface == "" {
		return
	}

	l.Registrations = append(l.Registrations, Registration{
		ContextConstant: contextConstant,
		AIDLInterface:   aidlInterface,
	})
}

// isNestedRegisterService checks whether ctx has an ancestor MethodCallExpression
// that is also a registerService call.
func isNestedRegisterService(ctx antlr.Tree) bool {
	for p := ctx.(antlr.RuleContext).GetParent(); p != nil; {
		mce, ok := p.(*javaparser.MethodCallExpressionContext)
		if ok {
			mc := mce.MethodCall()
			if mc != nil {
				id := mc.Identifier()
				if id != nil && id.GetText() == "registerService" {
					return true
				}
			}
		}

		next, ok := p.(antlr.RuleContext)
		if !ok {
			break
		}
		p = next.GetParent()
	}

	return false
}

// extractContextConstant finds the first Context.XXX reference in text
// and returns the constant name (e.g. "ACCOUNT_SERVICE").
// Returns empty string if no Context.XXX pattern is found.
func extractContextConstant(text string) string {
	m := contextConstantRe.FindStringSubmatch(text)
	if m == nil {
		return ""
	}

	return m[1]
}

// extractAIDLInterface finds the first IFoo.Stub.asInterface(...) reference in text
// and returns the interface name (e.g. "IAccountManager").
// Returns empty string if no Stub.asInterface call is found.
func extractAIDLInterface(text string) string {
	m := stubAsInterfaceRe.FindStringSubmatch(text)
	if m == nil {
		return ""
	}

	return m[1]
}
