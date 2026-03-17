package main

// ServiceInfo describes a single binder service: its AIDL descriptor,
// short aliases for CLI usage, and the methods it exposes.
type ServiceInfo struct {
	Descriptor string
	Aliases    []string
	Methods    []MethodInfo
}
