package validate

// ParcelableType represents a reference to an AIDL parcelable.
type ParcelableType struct {
	QualifiedName string
}

func (t *ParcelableType) resolvedTypeNode() {}

func (t *ParcelableType) String() string {
	return t.QualifiedName
}
