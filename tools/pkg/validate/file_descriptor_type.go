package validate

// FileDescriptorType represents ParcelFileDescriptor.
type FileDescriptorType struct{}

func (t *FileDescriptorType) resolvedTypeNode() {}

func (t *FileDescriptorType) String() string {
	return "ParcelFileDescriptor"
}
