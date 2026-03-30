package codegen

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/AndroidGoLab/binder/tools/pkg/parser"
	"github.com/AndroidGoLab/binder/tools/pkg/resolver"
)

func TestAIDLTypeToDexDescriptor(t *testing.T) {
	// Set up a minimal registry with a few types.
	reg := resolver.NewTypeRegistry()
	reg.Register("android.content.AttributionSource", &parser.ParcelableDecl{
		ParcName: "AttributionSource",
	})
	reg.Register("android.os.ParcelUuid", &parser.ParcelableDecl{
		ParcName: "ParcelUuid",
	})
	reg.Register("android.bluetooth.IBluetoothGattCallback", &parser.InterfaceDecl{
		IntfName: "IBluetoothGattCallback",
	})

	tests := []struct {
		name    string
		ts      *parser.TypeSpecifier
		pkg     string
		want    string
	}{
		{
			name: "int",
			ts:   &parser.TypeSpecifier{Name: "int"},
			want: "I",
		},
		{
			name: "boolean",
			ts:   &parser.TypeSpecifier{Name: "boolean"},
			want: "Z",
		},
		{
			name: "long",
			ts:   &parser.TypeSpecifier{Name: "long"},
			want: "J",
		},
		{
			name: "String",
			ts:   &parser.TypeSpecifier{Name: "String"},
			want: "Ljava/lang/String;",
		},
		{
			name: "IBinder",
			ts:   &parser.TypeSpecifier{Name: "IBinder"},
			want: "Landroid/os/IBinder;",
		},
		{
			name: "qualified_parcelable",
			ts:   &parser.TypeSpecifier{Name: "android.content.AttributionSource"},
			want: "Landroid/content/AttributionSource;",
		},
		{
			name: "short_name_with_registry",
			ts:   &parser.TypeSpecifier{Name: "AttributionSource"},
			want: "Landroid/content/AttributionSource;",
		},
		{
			name: "short_name_interface",
			ts:   &parser.TypeSpecifier{Name: "IBluetoothGattCallback"},
			pkg:  "android.bluetooth",
			want: "Landroid/bluetooth/IBluetoothGattCallback;",
		},
		{
			name: "array_of_int",
			ts:   &parser.TypeSpecifier{Name: "int", IsArray: true},
			want: "[I",
		},
		{
			name: "List",
			ts:   &parser.TypeSpecifier{Name: "List"},
			want: "Ljava/util/List;",
		},
		{
			name: "unresolvable",
			ts:   &parser.TypeSpecifier{Name: "NoSuchType"},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AIDLTypeToDexDescriptor(tt.ts, reg, tt.pkg)
			assert.Equal(t, tt.want, got)
		})
	}
}
