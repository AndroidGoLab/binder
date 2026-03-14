package codegen

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/xaionaro-go/aidl/tools/pkg/parser"
)

func TestMarshalForType_Primitives(t *testing.T) {
	tests := []struct {
		name      string
		aidlType  string
		writeExpr string
		readExpr  string
		needsCast bool
	}{
		{
			name:      "int",
			aidlType:  "int",
			writeExpr: "_data.WriteInt32(%s)",
			readExpr:  "_reply.ReadInt32()",
		},
		{
			name:      "long",
			aidlType:  "long",
			writeExpr: "_data.WriteInt64(%s)",
			readExpr:  "_reply.ReadInt64()",
		},
		{
			name:      "boolean",
			aidlType:  "boolean",
			writeExpr: "_data.WriteBool(%s)",
			readExpr:  "_reply.ReadBool()",
		},
		{
			name:      "byte",
			aidlType:  "byte",
			writeExpr: "_data.WritePaddedByte(%s)",
			readExpr:  "_reply.ReadPaddedByte()",
		},
		{
			name:      "float",
			aidlType:  "float",
			writeExpr: "_data.WriteFloat32(%s)",
			readExpr:  "_reply.ReadFloat32()",
		},
		{
			name:      "double",
			aidlType:  "double",
			writeExpr: "_data.WriteFloat64(%s)",
			readExpr:  "_reply.ReadFloat64()",
		},
		{
			name:      "char",
			aidlType:  "char",
			writeExpr: "_data.WriteInt32(int32(%s))",
			readExpr:  "_reply.ReadInt32()",
			needsCast: true,
		},
		{
			name:      "String",
			aidlType:  "String",
			writeExpr: "_data.WriteString16(%s)",
			readExpr:  "_reply.ReadString16()",
		},
		{
			name:      "IBinder",
			aidlType:  "IBinder",
			writeExpr: "_data.WriteStrongBinder(%s.Handle())",
			readExpr:  "_reply.ReadStrongBinder()",
		},
		{
			name:      "ParcelFileDescriptor",
			aidlType:  "ParcelFileDescriptor",
			writeExpr: "_data.WriteFileDescriptor(%s)",
			readExpr:  "_reply.ReadFileDescriptor()",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := &parser.TypeSpecifier{Name: tt.aidlType}
			info := MarshalForType(ts)
			assert.Equal(t, tt.writeExpr, info.WriteExpr)
			assert.Equal(t, tt.readExpr, info.ReadExpr)
			assert.Equal(t, tt.needsCast, info.NeedsCast)
		})
	}
}

func TestMarshalForType_Utf8String(t *testing.T) {
	ts := &parser.TypeSpecifier{
		Name:   "String",
		Annots: []*parser.Annotation{{Name: "utf8InCpp"}},
	}
	info := MarshalForType(ts)
	assert.Equal(t, "_data.WriteString(%s)", info.WriteExpr)
	assert.Equal(t, "_reply.ReadString()", info.ReadExpr)
	assert.False(t, info.NeedsCast)
}

func TestMarshalForType_Parcelable(t *testing.T) {
	ts := &parser.TypeSpecifier{Name: "MyParcelable"}
	info := MarshalForType(ts)
	assert.Equal(t, "%s.MarshalParcel(_data)", info.WriteExpr)
	assert.Equal(t, "%s.UnmarshalParcel(_reply)", info.ReadExpr)
	assert.False(t, info.NeedsCast)
}

func TestMarshalForType_Map(t *testing.T) {
	t.Run("Map_returns_IsMap_true", func(t *testing.T) {
		ts := &parser.TypeSpecifier{Name: "Map"}
		info := MarshalForType(ts)
		assert.True(t, info.IsMap, "Map type must set IsMap flag")
	})

	t.Run("Map_with_type_args", func(t *testing.T) {
		ts := &parser.TypeSpecifier{
			Name: "Map",
			TypeArgs: []*parser.TypeSpecifier{
				{Name: "String"},
				{Name: "int"},
			},
		}
		info := MarshalForType(ts)
		assert.True(t, info.IsMap, "Map<String,int> must set IsMap flag")
	})
}

func TestMarshalForType_Nil(t *testing.T) {
	info := MarshalForType(nil)
	assert.Equal(t, "", info.WriteExpr)
	assert.Equal(t, "", info.ReadExpr)
	assert.False(t, info.NeedsCast)
}
