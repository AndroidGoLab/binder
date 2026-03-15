package servicemap

// Registration represents a service registration extracted from SystemServiceRegistry.java.
type Registration struct {
	ContextConstant string // e.g. "ACCOUNT_SERVICE"
	AIDLInterface   string // e.g. "IAccountManager"
}
