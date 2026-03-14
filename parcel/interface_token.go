package parcel

const (
	// strictModePenaltyGather is the strict-mode policy flag with bit 31 set.
	strictModePenaltyGather = -1 << 31

	// workSourceUIDNone indicates no work-source UID.
	workSourceUIDNone = int32(-1)

	// headerSYST is the vendor header for system (non-VNDK, non-recovery) builds.
	// B_PACK_CHARS('S','Y','S','T') = 0x53595354
	headerSYST = int32(0x53595354)
)

// WriteInterfaceToken writes a Binder interface token (descriptor) with
// the strict-mode policy header, work-source UID, vendor header, and descriptor.
// The format matches Android's Parcel::writeInterfaceToken:
//
//	int32: strict-mode policy | STRICT_MODE_PENALTY_GATHER
//	int32: work-source UID (or -1 for none)
//	int32: vendor header ('SYST' for system builds)
//	String16: interface descriptor
func (p *Parcel) WriteInterfaceToken(
	descriptor string,
) {
	p.WriteInt32(strictModePenaltyGather)
	p.WriteInt32(workSourceUIDNone)
	p.WriteInt32(headerSYST)
	p.WriteString16(descriptor)
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
