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

// ServiceInfo describes a single binder service: its AIDL descriptor,
// short aliases for CLI usage, and the methods it exposes.
type ServiceInfo struct {
	Descriptor string
	Aliases    []string
	Methods    []MethodInfo
}

// MethodInfo describes one method on a binder service interface.
type MethodInfo struct {
	Name       string
	Params     []ParamInfo
	ReturnType string
}

// ParamInfo describes one parameter of a binder method.
type ParamInfo struct {
	Name string
	Type string
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
// Each uppercase letter starts a new segment separated by '-'.
func camelToKebab(s string) string {
	var b strings.Builder
	for i, r := range s {
		if unicode.IsUpper(r) && i > 0 {
			b.WriteByte('-')
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}
