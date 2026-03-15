package servicemap

// ServiceMapEntry represents a fully resolved service mapping from
// service name to its AIDL interface descriptor.
type ServiceMapEntry struct {
	ServiceName    string `json:"service_name"`    // e.g. "activity"
	ConstantName   string `json:"constant_name"`   // e.g. "ACTIVITY_SERVICE"
	AIDLInterface  string `json:"aidl_interface"`  // e.g. "IActivityManager" (simple name)
	AIDLDescriptor string `json:"aidl_descriptor"` // e.g. "android.app.IActivityManager" (fully qualified)
}
