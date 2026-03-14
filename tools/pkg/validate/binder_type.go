package validate

// BinderType represents IBinder.
type BinderType struct{}

func (t *BinderType) resolvedTypeNode() {}

func (t *BinderType) String() string {
	return "IBinder"
}
