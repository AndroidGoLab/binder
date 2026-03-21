package codegen

import (
	"strings"

	"github.com/xaionaro-go/binder/tools/pkg/parser"
	"github.com/xaionaro-go/binder/tools/pkg/resolver"
)

// MarshalInfo contains the parcel read/write expressions for a type.
type MarshalInfo struct {
	// WriteExpr is a format string where %s is the variable name.
	// e.g., "_data.WriteInt32(%s)" where %s is substituted with the var name.
	WriteExpr string

	// ReadExpr is the expression to read the value from a parcel.
	// e.g., "_reply.ReadInt32()" which returns (T, error).
	ReadExpr string

	// NeedsCast is true if the read returns a different type that needs casting.
	NeedsCast bool

	// IsIBinder is true for IBinder types that require constructing a
	// ProxyBinder from the raw handle returned by ReadStrongBinder.
	IsIBinder bool

	// IsInterface is true for user-defined AIDL interface types (not IBinder).
	// These require constructing a typed proxy from the binder handle.
	IsInterface bool

	// IsMap is true for Map<K,V> types. Map serialization requires inline
	// code generation (writing count + key/value pairs) and cannot be
	// expressed as a single WriteExpr/ReadExpr format string.
	IsMap bool
}

// marshalPrimitiveMap maps AIDL type names to their marshal info.
var marshalPrimitiveMap = map[string]MarshalInfo{
	"int": {
		WriteExpr: "_data.WriteInt32(%s)",
		ReadExpr:  "_reply.ReadInt32()",
	},
	"long": {
		WriteExpr: "_data.WriteInt64(%s)",
		ReadExpr:  "_reply.ReadInt64()",
	},
	"boolean": {
		WriteExpr: "_data.WriteBool(%s)",
		ReadExpr:  "_reply.ReadBool()",
	},
	"byte": {
		WriteExpr: "_data.WritePaddedByte(%s)",
		ReadExpr:  "_reply.ReadPaddedByte()",
	},
	"float": {
		WriteExpr: "_data.WriteFloat32(%s)",
		ReadExpr:  "_reply.ReadFloat32()",
	},
	"double": {
		WriteExpr: "_data.WriteFloat64(%s)",
		ReadExpr:  "_reply.ReadFloat64()",
	},
	"char": {
		WriteExpr: "_data.WriteInt32(int32(%s))",
		ReadExpr:  "_reply.ReadInt32()",
		NeedsCast: true,
	},
	"String": {
		WriteExpr: "_data.WriteString16(%s)",
		ReadExpr:  "_reply.ReadString16()",
	},
	"CharSequence": {
		WriteExpr: "_data.WriteString16(%s)",
		ReadExpr:  "_reply.ReadString16()",
	},
	"IBinder": {
		WriteExpr: "_data.WriteStrongBinder(%s.Handle())",
		ReadExpr:  "_reply.ReadStrongBinder()",
		IsIBinder: true,
	},
	"ParcelFileDescriptor": {
		WriteExpr: "_data.WriteFileDescriptor(%s)",
		ReadExpr:  "_reply.ReadFileDescriptor()",
	},
	"FileDescriptor": {
		WriteExpr: "_data.WriteFileDescriptor(%s)",
		ReadExpr:  "_reply.ReadFileDescriptor()",
	},
}

// MarshalForType returns the MarshalInfo for an AIDL type.
func MarshalForType(ts *parser.TypeSpecifier) MarshalInfo {
	return MarshalForTypeWithRegistry(ts, nil)
}

// MarshalForTypeWithRegistry returns the MarshalInfo for an AIDL type,
// using the registry to look up whether user-defined types are enums
// (which use integer marshaling) or interfaces (which use IBinder).
// currentPkg, if non-empty, enables same-package lookups by trying
// currentPkg + "." + typeName as a fully qualified name.
func MarshalForTypeWithRegistry(
	ts *parser.TypeSpecifier,
	registry *resolver.TypeRegistry,
	currentPkg ...string,
) MarshalInfo {
	if ts == nil {
		return MarshalInfo{}
	}

	// The @utf8InCpp annotation only affects the in-memory representation
	// in C++ (std::string vs android::String16). The wire format is always
	// UTF-16 because the C++ binder backend uses writeUtf8AsUtf16 /
	// readUtf8FromUtf16 for these fields. We therefore do NOT special-case
	// @utf8InCpp here -- all Strings use WriteString16/ReadString16.

	if info, ok := marshalPrimitiveMap[ts.Name]; ok {
		return info
	}

	// Map types require inline code generation (count + key/value pairs).
	// The IsMap flag signals code generators to emit map-specific serialization.
	if ts.Name == "Map" {
		return MarshalInfo{IsMap: true}
	}

	// If we have a registry, look up the type to determine marshaling strategy.
	if registry != nil {
		// Try fully qualified name first.
		if def, ok := registry.Lookup(ts.Name); ok {
			return marshalForDefinition(def)
		}
		// Try current package + name for same-package references.
		if len(currentPkg) > 0 && currentPkg[0] != "" {
			candidate := currentPkg[0] + "." + ts.Name
			if def, ok := registry.Lookup(candidate); ok {
				return marshalForDefinition(def)
			}
		}
		// Try short name lookup.
		if def, ok := registry.LookupByShortName(ts.Name); ok {
			return marshalForDefinition(def)
		}
	}

	// Parcelable types use MarshalParcel/UnmarshalParcel.
	return MarshalInfo{
		WriteExpr: "%s.MarshalParcel(_data)",
		ReadExpr:  "%s.UnmarshalParcel(_reply)",
	}
}

// marshalForDefinition returns the MarshalInfo based on the definition kind.
func marshalForDefinition(def parser.Definition) MarshalInfo {
	switch d := def.(type) {
	case *parser.EnumDecl:
		return marshalForEnum(d)
	case *parser.InterfaceDecl:
		return MarshalInfo{
			WriteExpr:   "_data.WriteStrongBinder(%s.AsBinder().Handle())",
			ReadExpr:    "_reply.ReadStrongBinder()",
			IsInterface: true,
		}
	default:
		return MarshalInfo{
			WriteExpr: "%s.MarshalParcel(_data)",
			ReadExpr:  "%s.UnmarshalParcel(_reply)",
		}
	}
}

// marshalForEnum returns MarshalInfo for an enum type, using the
// appropriate read/write method for the enum's backing type (default int32).
func marshalForEnum(decl *parser.EnumDecl) MarshalInfo {
	backingType := "int"
	if decl.BackingType != nil {
		backingType = decl.BackingType.Name
	}

	goType := aidlPrimitiveToGo[backingType]
	if goType == "" {
		goType = "int32"
	}

	if info, ok := marshalPrimitiveMap[backingType]; ok {
		// Wrap the write expression to cast the enum to its backing type.
		// e.g., "_data.WriteInt32(%s)" -> "_data.WriteInt32(int32(%s))"
		writeExpr := castWriteExpr(info.WriteExpr, goType)
		return MarshalInfo{
			WriteExpr: writeExpr,
			ReadExpr:  info.ReadExpr,
			NeedsCast: true,
		}
	}

	// Fallback to int32 for unknown backing types.
	return MarshalInfo{
		WriteExpr: "_data.WriteInt32(int32(%s))",
		ReadExpr:  "_reply.ReadInt32()",
		NeedsCast: true,
	}
}

// castWriteExpr wraps the %s in a write expression with a type cast.
// "_data.WriteInt32(%s)" with goType "int32" -> "_data.WriteInt32(int32(%s))"
func castWriteExpr(
	writeExpr string,
	goType string,
) string {
	return strings.ReplaceAll(writeExpr, "%s", goType+"(%s)")
}
