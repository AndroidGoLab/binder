package main

import (
	"strings"
	"unicode"
)

// generatedRegistry is populated by registry_gen.go's init() function.
var generatedRegistry *Registry

// Registry holds the set of known binder services, indexed by descriptor.
type Registry struct {
	Services map[string]*ServiceInfo
}

// ByDescriptor returns the ServiceInfo for the given AIDL descriptor,
// or nil if not found.
func (r *Registry) ByDescriptor(descriptor string) *ServiceInfo {
	return r.Services[descriptor]
}

// ByAlias scans all registered services and returns the first whose
// Aliases list contains the given alias, or nil if none matches.
func (r *Registry) ByAlias(alias string) *ServiceInfo {
	for _, svc := range r.Services {
		for _, a := range svc.Aliases {
			if a == alias {
				return svc
			}
		}
	}
	return nil
}

// Lookup tries to find a service by alias first, then by descriptor.
func (r *Registry) Lookup(name string) *ServiceInfo {
	if svc := r.ByAlias(name); svc != nil {
		return svc
	}
	return r.ByDescriptor(name)
}

// camelToKebab converts a CamelCase identifier to kebab-case.
// Inserts a hyphen before an uppercase letter when the previous character
// is lowercase, or when the next character is lowercase (to handle acronyms
// like "USBSpeed" → "usb-speed").
func camelToKebab(s string) string {
	runes := []rune(s)
	var b strings.Builder
	for i, r := range runes {
		if unicode.IsUpper(r) && i > 0 {
			prevLower := unicode.IsLower(runes[i-1])
			nextLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])
			if prevLower || nextLower {
				b.WriteByte('-')
			}
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}
