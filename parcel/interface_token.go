package parcel

const (
	// strictModePenaltyGather is the strict-mode policy flag with bit 31 set.
	strictModePenaltyGather = -1 << 31

	// workSourceUIDNone indicates no work-source UID.
	workSourceUIDNone = int32(-1)

	// headerSYST is the vendor header for system (non-VNDK, non-recovery) builds.
	// B_PACK_CHARS('S','Y','S','T') = 0x53595354
	headerSYST = int32(0x53595354)

	// vendorHeaderOffset is the byte offset of the vendor header in a token.
	//
	// Example: remove this word for an Android 10 request.
	vendorHeaderOffset = 8

	// vendorHeaderEnd is the byte offset immediately after the vendor header.
	//
	// Example: copy the descriptor following the removed header.
	vendorHeaderEnd = 12

	// vendorHeaderSize is the size of the vendor header in bytes.
	//
	// Example: adjust Binder object offsets after removing the header.
	vendorHeaderSize = vendorHeaderEnd - vendorHeaderOffset
)

// WriteInterfaceToken writes a Binder interface token (descriptor) with
// the strict-mode policy header, work-source UID, vendor header, and descriptor.
// The format matches Android's Parcel::writeInterfaceToken:
//
//	int32: strict-mode policy | STRICT_MODE_PENALTY_GATHER
//	int32: work-source UID (or -1 for none)
//	int32: vendor header ('SYST' for system builds)
//	String16: interface descriptor
//
// Example: address a request to android.os.IServiceManager.
func (p *Parcel) WriteInterfaceToken(
	descriptor string,
) {
	atStart := len(p.data) == 0
	p.WriteInt32(strictModePenaltyGather)
	p.WriteInt32(workSourceUIDNone)
	p.WriteInt32(headerSYST)
	p.WriteString16(descriptor)
	if atStart {
		p.interfaceTokenEnd = len(p.data)
	}
}

// Android10Request returns a copy with the marked interface token encoded for
// Android 10, or p itself if no token was written at the start. Object offsets
// are adjusted in the copy; the original parcel remains unchanged.
//
// Example: adapt a generated AIDL request before sending it to API 29.
func (p *Parcel) Android10Request() *Parcel {
	if p.interfaceTokenEnd < vendorHeaderEnd || p.interfaceTokenEnd > len(p.data) {
		return p
	}
	for _, offset := range p.objects {
		if offset < uint64(p.interfaceTokenEnd) || offset > uint64(len(p.data)) {
			return p
		}
	}
	data := make([]byte, 0, len(p.data)-vendorHeaderSize)
	data = append(data, p.data[:vendorHeaderOffset]...)
	data = append(data, p.data[vendorHeaderEnd:]...)
	var objects []uint64
	if p.objects != nil {
		objects = make([]uint64, len(p.objects))
		for i, offset := range p.objects {
			objects[i] = offset - vendorHeaderSize
		}
	}
	clone := FromBytesWithObjects(data, objects)
	clone.SetTargetAPILevel(p.TargetAPILevel())
	clone.SetBinderSerializationContext(p.BinderSerializationContext())
	for _, lease := range p.binderLeases {
		lease.retain()
		clone.binderLeases = append(clone.binderLeases, lease)
	}
	return clone
}

// ReadInterfaceToken reads a Binder interface token, consuming the
// strict-mode policy header, work-source UID, vendor header, and returns the descriptor.
func (p *Parcel) ReadInterfaceToken() (string, error) {
	// Read and discard strict-mode policy.
	_, err := p.ReadInt32()
	if err != nil {
		return "", err
	}

	// Read and discard work-source UID.
	_, err = p.ReadInt32()
	if err != nil {
		return "", err
	}

	// Read and discard vendor header.
	_, err = p.ReadInt32()
	if err != nil {
		return "", err
	}

	return p.ReadString16()
}
