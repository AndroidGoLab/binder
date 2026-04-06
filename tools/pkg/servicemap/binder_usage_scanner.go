package servicemap

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// BinderUsage records a single occurrence of an AIDL interface being obtained
// via ServiceManager.getService for a specific service.
type BinderUsage struct {
	// AIDLInterface is the fully qualified interface name when resolvable
	// via imports, otherwise the simple name (e.g., "ITelephony").
	AIDLInterface string

	// ServiceConstant is the Context constant name when the call uses
	// Context.XXX_SERVICE (e.g., "TELEPHONY_SERVICE"). Empty if the
	// call uses a string literal instead.
	ServiceConstant string

	// ServiceLiteral is the service name string literal when the call
	// uses ServiceManager.getService("name") directly. Empty if the
	// call uses a Context constant.
	ServiceLiteral string
}

// asInterfaceGetServiceRe matches patterns like:
//
//	IFoo.Stub.asInterface(ServiceManager.getService(Context.BAR_SERVICE))
//	IFoo.Stub.asInterface(ServiceManager.getServiceOrThrow(Context.BAR_SERVICE))
//	IFoo.Stub.asInterface(\n  ServiceManager.getService("bar"))
//
// It captures the interface name, and optionally the Context constant or string literal.
var asInterfaceGetServiceRe = regexp.MustCompile(
	`(\w+)\.Stub\.asInterface\s*\(\s*` +
		`(?:\w+\s*\.\s*)*` + // optional qualifier chain before ServiceManager
		`ServiceManager\s*\.\s*getService(?:OrThrow)?\s*\(\s*` +
		`(?:` +
		`Context\s*\.\s*([A-Z_]+)` + // group 2: Context constant
		`|` +
		`"([^"]+)"` + // group 3: string literal
		`)`,
)

// systemServiceAnnotationRe matches @SystemService(Context.XXX_SERVICE).
var systemServiceAnnotationRe = regexp.MustCompile(
	`@SystemService\s*\(\s*Context\s*\.\s*([A-Z_]+)\s*\)`,
)

// scannerStubAsInterfaceRe matches IFoo.Stub.asInterface(...) and captures IFoo.
var scannerStubAsInterfaceRe = regexp.MustCompile(
	`(\w+)\.Stub\.asInterface\s*\(`,
)

// classNameRe extracts the public class name from a Java source file.
var classNameRe = regexp.MustCompile(
	`(?m)^public\s+(?:final\s+)?class\s+(\w+)`,
)

// ScanBinderUsages walks all Java files under root and extracts
// service-to-AIDL-interface associations using two strategies:
//
//  1. Direct: IFoo.Stub.asInterface(ServiceManager.getService(...)) in the same expression.
//  2. Annotated: @SystemService(Context.XXX) on a class that calls IFoo.Stub.asInterface(),
//     where the interface name relates to the class name (e.g., ActivityManager -> IActivityManager).
//
// Multiple usages of the same interface are deduplicated.
func ScanBinderUsages(
	root string,
) ([]BinderUsage, error) {
	var usages []BinderUsage
	seen := map[string]bool{}

	addUsage := func(
		fqn string,
		constName string,
		literalName string,
	) {
		key := fqn + ":" + constName + ":" + literalName
		if seen[key] {
			return
		}
		seen[key] = true

		usages = append(usages, BinderUsage{
			AIDLInterface:   fqn,
			ServiceConstant: constName,
			ServiceLiteral:  literalName,
		})
	}

	err := filepath.Walk(root, func(
		path string,
		info os.FileInfo,
		walkErr error,
	) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() || !strings.HasSuffix(path, ".java") {
			return nil
		}

		src, readErr := os.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("reading %s: %w", path, readErr)
		}

		content := string(src)
		if !strings.Contains(content, "Stub.asInterface") {
			return nil
		}

		imports := extractImports(content)

		// Strategy 1: direct ServiceManager.getService calls inside asInterface.
		if strings.Contains(content, "ServiceManager") {
			for _, m := range asInterfaceGetServiceRe.FindAllStringSubmatch(content, -1) {
				fqn := resolveInterfaceName(m[1], imports)
				addUsage(fqn, m[2], m[3])
			}
		}

		// Strategy 2: @SystemService(Context.XXX) annotation on a class
		// that calls IFoo.Stub.asInterface(). To avoid false positives
		// when a class uses multiple AIDL interfaces, only accept interfaces
		// whose name relates to the annotated class name.
		annotationMatch := systemServiceAnnotationRe.FindStringSubmatch(content)
		if annotationMatch == nil {
			return nil
		}
		constName := annotationMatch[1]

		className := extractPublicClassName(content)
		candidates := scannerStubAsInterfaceRe.FindAllStringSubmatch(content, -1)
		best := selectBestInterfaceForClass(className, candidates)
		if best != "" {
			fqn := resolveInterfaceName(best, imports)
			addUsage(fqn, constName, "")
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scanning binder usages: %w", err)
	}

	return usages, nil
}

// resolveInterfaceName resolves a simple Java class name to its fully
// qualified name using the file's import table. Returns the simple name
// unchanged if no import is found.
func resolveInterfaceName(
	simpleName string,
	imports map[string]string,
) string {
	if fqn, ok := imports[simpleName]; ok {
		return fqn
	}
	return simpleName
}

// extractPublicClassName extracts the public class name from a Java source.
func extractPublicClassName(content string) string {
	m := classNameRe.FindStringSubmatch(content)
	if m == nil {
		return ""
	}
	return m[1]
}

// selectBestInterfaceForClass picks the interface from candidates whose
// name best matches the given class name. For a class named "FooManager"
// or "FooService", the ideal interface is "IFoo", "IFooManager", or
// "IFooService". If only one candidate exists, it is returned directly.
// Returns empty string if no suitable match is found.
func selectBestInterfaceForClass(
	className string,
	candidates [][]string,
) string {
	// Deduplicate candidate interface names.
	unique := make(map[string]struct{}, len(candidates))
	for _, c := range candidates {
		unique[c[1]] = struct{}{}
	}

	if len(unique) == 0 {
		return ""
	}

	// Single candidate — no ambiguity.
	if len(unique) == 1 {
		for name := range unique {
			return name
		}
	}

	if className == "" {
		return ""
	}

	// Strip common suffixes to derive the core concept.
	// "ActivityManager" -> "Activity", "TelephonyManager" -> "Telephony".
	core := className
	for _, suffix := range []string{"Manager", "Service"} {
		core = strings.TrimSuffix(core, suffix)
	}
	coreLower := strings.ToLower(core)

	// Score each candidate: prefer interfaces whose name (minus the "I"
	// prefix) matches or contains the class's core concept.
	type scored struct {
		name  string
		score int
	}
	var best scored
	for name := range unique {
		ifaceCore := strings.TrimPrefix(name, "I")
		ifaceCoreLower := strings.ToLower(ifaceCore)

		var s int
		switch {
		case ifaceCoreLower == coreLower:
			// IActivity matches ActivityManager.
			s = 4
		case ifaceCoreLower == coreLower+"manager":
			// IActivityManager matches ActivityManager.
			s = 3
		case ifaceCoreLower == coreLower+"service":
			// IActivityService matches ActivityManager.
			s = 3
		case strings.Contains(ifaceCoreLower, coreLower):
			// Partial match.
			s = 2
		case strings.Contains(coreLower, ifaceCoreLower):
			// Reverse partial match.
			s = 1
		default:
			continue
		}

		if s > best.score {
			best = scored{name: name, score: s}
		}
	}

	return best.name
}
