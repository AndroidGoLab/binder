package versionaware

import (
	"debug/elf"
	"encoding/hex"
	"errors"
	"fmt"
)

const (
	// surfaceComposerVerifiedAPI is the API level of the verified library.
	//
	// Example: reject a matching library on a different Android API level.
	surfaceComposerVerifiedAPI = 34
	// elfNoteWordSize is the size of each ELF note header field.
	//
	// Example: locate the note type after two fields.
	elfNoteWordSize = 4
	// elfNoteHeaderSize is the size of an ELF note header.
	//
	// Example: locate the owner after three header fields.
	elfNoteHeaderSize = 3 * elfNoteWordSize
	// gnuBuildIDNoteType identifies GNU build ID notes.
	//
	// Example: distinguish a build ID from another GNU note.
	gnuBuildIDNoteType = 3
	// gnuBuildIDOwner is the null-terminated GNU note owner.
	//
	// Example: verify the owner of a build ID note.
	gnuBuildIDOwner = "GNU\x00"
	// surfaceComposerBuildIDSize is the verified build ID length in bytes.
	//
	// Example: reject a build ID of an unsupported length.
	surfaceComposerBuildIDSize = 16
	// gnuBuildIDDescriptorOffset locates the descriptor after the aligned owner.
	//
	// Example: read a build ID following its note header and owner.
	gnuBuildIDDescriptorOffset = elfNoteHeaderSize + len(gnuBuildIDOwner)
	// surfaceComposerLibraryPath identifies the device's 64-bit native client.
	//
	// Example: inspect this file to recognize the capture contract.
	surfaceComposerLibraryPath = "/system/lib64/libgui.so"
	// surfaceComposerVerifiedBuildID identifies the verified Pixel 6a library.
	//
	// Example: reject a different build with matching exported signatures.
	surfaceComposerVerifiedBuildID = "e1a469cf6fa0276c7bb104f53fc1f463"
	// surfaceComposerCaptureSymbol identifies the capture method signature.
	//
	// Example: require the CaptureArgs parameter in the native client.
	surfaceComposerCaptureSymbol = "_ZN7android3gui17BpSurfaceComposer18captureDisplayByIdElRKNS0_11CaptureArgsERKNS_2spINS0_22IScreenCaptureListenerEEE"
	// surfaceComposerIDsSymbol identifies the physical display enumeration method.
	//
	// Example: require the native display ID vector method.
	surfaceComposerIDsSymbol = "_ZN7android3gui17BpSurfaceComposer21getPhysicalDisplayIdsEPNSt3__16vectorIlNS2_9allocatorIlEEEE"
	// surfaceComposerSerializerSymbol identifies the CaptureArgs serializer.
	//
	// Example: require the serializer verified for this contract.
	surfaceComposerSerializerSymbol = "_ZNK7android3gui11CaptureArgs13writeToParcelEPNS_6ParcelE"
)

// surfaceComposerLibrary describes evidence from a native client library.
//
// Example: compare the device libgui against the verified capture contract.
type surfaceComposerLibrary struct {
	API     int
	Class   elf.Class
	Machine elf.Machine
	BuildID string
	Symbols map[string]bool
}

// supported reports whether all evidence matches the verified library.
//
// Example: reject a different build even when it exports the same signatures.
func (l surfaceComposerLibrary) supported() bool {
	return l.API == surfaceComposerVerifiedAPI && l.Class == elf.ELFCLASS64 && l.Machine == elf.EM_AARCH64 &&
		l.BuildID == surfaceComposerVerifiedBuildID && l.Symbols[surfaceComposerCaptureSymbol] &&
		l.Symbols[surfaceComposerIDsSymbol] && l.Symbols[surfaceComposerSerializerSymbol]
}

// readSurfaceComposerLibrary extracts the metadata used to identify libgui.
//
// Example: inspect /system/lib64/libgui.so before enabling native capture.
func readSurfaceComposerLibrary(
	path string,
	api int,
) (_ surfaceComposerLibrary, _err error) {
	f, err := elf.Open(path)
	if err != nil {
		return surfaceComposerLibrary{}, err
	}
	// Report a close failure alongside any inspection failure.
	//
	// Example: a successful inspection still reports a failed close.
	defer func() { _err = errors.Join(_err, f.Close()) }()
	section := f.Section(".note.gnu.build-id")
	if section == nil {
		return surfaceComposerLibrary{}, fmt.Errorf("missing GNU build ID")
	}
	note, err := section.Data()
	if err != nil {
		return surfaceComposerLibrary{}, err
	}
	if len(note) < gnuBuildIDDescriptorOffset {
		return surfaceComposerLibrary{}, fmt.Errorf("invalid GNU build ID note")
	}
	namesz := f.ByteOrder.Uint32(note[:elfNoteWordSize])
	descsz := f.ByteOrder.Uint32(note[elfNoteWordSize : 2*elfNoteWordSize])
	kind := f.ByteOrder.Uint32(note[2*elfNoteWordSize : elfNoteHeaderSize])
	if namesz != uint32(len(gnuBuildIDOwner)) || kind != gnuBuildIDNoteType || string(note[elfNoteHeaderSize:gnuBuildIDDescriptorOffset]) != gnuBuildIDOwner || descsz != surfaceComposerBuildIDSize || len(note) < gnuBuildIDDescriptorOffset+int(descsz) {
		return surfaceComposerLibrary{}, fmt.Errorf("invalid GNU build ID note")
	}
	syms, err := f.DynamicSymbols()
	if err != nil {
		return surfaceComposerLibrary{}, err
	}
	result := surfaceComposerLibrary{API: api, Class: f.Class, Machine: f.Machine, BuildID: hex.EncodeToString(note[gnuBuildIDDescriptorOffset : gnuBuildIDDescriptorOffset+int(descsz)]), Symbols: surfaceComposerDefinedSymbols(syms)}
	return result, nil
}

// surfaceComposerDefinedSymbols records only definitions exported by libgui.
//
// Example: an undefined import cannot establish a native implementation.
func surfaceComposerDefinedSymbols(symbols []elf.Symbol) map[string]bool {
	result := make(map[string]bool, len(symbols))
	for _, symbol := range symbols {
		if symbol.Section != elf.SHN_UNDEF {
			result[symbol.Name] = true
		}
	}
	return result
}
